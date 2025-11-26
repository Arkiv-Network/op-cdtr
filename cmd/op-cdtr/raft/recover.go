package raft

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/urfave/cli/v2"

	"github.com/arkiv-network/op-cdtr/pkg/flags"
	"github.com/arkiv-network/op-cdtr/pkg/store"
	consensus "github.com/ethereum-optimism/optimism/op-conductor/consensus"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum-optimism/optimism/op-service/sources"
)

func RecoverCommand() *cli.Command {
	recoverFlags := append([]cli.Flag{
		flags.StateDirFlag,
		flags.NodesFlag,
		flags.ServerIDsFlag,
		flags.InitialLeaderFlag,
		flags.ExecutionRPCFlag,
		flags.BlockNumberFlag,
		flags.BlockHashFlag,
		flags.ForceFlag,
	}, oplog.CLIFlags(flags.EnvVarPrefix)...)

	return &cli.Command{
		Name:        "recover",
		Usage:       "Recover Raft cluster with new configuration and unsafe head",
		Description: "Use RecoverCluster to force a new cluster configuration while preserving or updating unsafe head. This is for disaster recovery when quorum is lost.",
		Action:      RecoverAction,
		Flags:       cliapp.ProtectFlags(recoverFlags),
	}
}

func RecoverAction(ctx *cli.Context) error {
	logCfg := oplog.ReadCLIConfig(ctx)
	logger := oplog.NewLogger(oplog.AppOut(ctx), logCfg)
	oplog.SetGlobalLogHandler(logger.Handler())
	opservice.ValidateEnvVars(flags.EnvVarPrefix, flags.Flags, logger)

	stateDir := ctx.String(flags.StateDirFlag.Name)
	nodesStr := ctx.String(flags.NodesFlag.Name)
	serverIDsStr := ctx.String(flags.ServerIDsFlag.Name)
	initialLeader := ctx.String(flags.InitialLeaderFlag.Name)
	executionRPC := ctx.String(flags.ExecutionRPCFlag.Name)
	blockNumberFlag := ctx.Uint64("block-number")
	blockHashFlag := ctx.String("block-hash")
	force := ctx.Bool("force")

	if stateDir == "" || nodesStr == "" || serverIDsStr == "" || initialLeader == "" || executionRPC == "" {
		return fmt.Errorf("state-dir, nodes, server-ids, initial-leader, and execution-rpc are required")
	}

	logger.Info("Starting Raft cluster recovery",
		"stateDir", stateDir,
		"executionRPC", executionRPC,
		"initialLeader", initialLeader)

	nodes, serverIDs, err := parseNodesAndIDs(nodesStr, serverIDsStr)
	if err != nil {
		return fmt.Errorf("failed to parse nodes and server IDs: %w", err)
	}

	var thisServerID string
	var thisServerAddr string
	for i, node := range nodes {
		if filepath.Base(stateDir) == serverIDs[i] || filepath.Base(filepath.Dir(stateDir)) == serverIDs[i] {
			thisServerID = serverIDs[i]
			thisServerAddr = node.Address
			break
		}
	}

	if thisServerID == "" {
		logger.Warn("Could not determine server ID from state-dir path, using first server in list")
		thisServerID = serverIDs[0]
		thisServerAddr = nodes[0].Address
	}

	logger.Info("Recovering cluster for server",
		"serverID", thisServerID,
		"address", thisServerAddr)

	if !force {
		logger.Warn("This operation will:")
		logger.Warn("  - Create a snapshot with new cluster configuration")
		logger.Warn("  - Truncate existing Raft log")
		logger.Warn("  - Implicitly commit all pending log entries")
		logger.Warn("  - Force a new unsafe head from execution layer")
		logger.Warn("")
		logger.Warn("This is a DANGEROUS operation. Use --force to proceed.")
		return fmt.Errorf("recovery requires --force flag")
	}

	logger.Info("Connecting to execution layer")
	ctxBg := context.Background()

	rpcClient, err := client.NewRPC(ctxBg, logger, executionRPC)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %w", err)
	}
	defer rpcClient.Close()

	l2Config := sources.L2ClientDefaultConfig(&rollup.Config{}, true)
	l2Client, err := sources.NewL2Client(rpcClient, logger, nil, l2Config)
	if err != nil {
		return fmt.Errorf("failed to create L2 client: %w", err)
	}

	var envelope *eth.ExecutionPayloadEnvelope
	switch {
	case blockNumberFlag > 0:
		logger.Info("Fetching block by number", "number", blockNumberFlag)
		envelope, err = l2Client.PayloadByNumber(ctxBg, blockNumberFlag)
	case blockHashFlag != "":
		logger.Info("Fetching block by hash", "hash", blockHashFlag)
		envelope, err = l2Client.PayloadByHash(ctxBg, common.HexToHash(blockHashFlag))
	default:
		logger.Info("Fetching latest unsafe block")
		envelope, err = l2Client.PayloadByLabel(ctxBg, eth.Unsafe)
	}
	if err != nil {
		return fmt.Errorf("failed to get payload: %w", err)
	}

	logger.Info("Retrieved unsafe head",
		"blockNumber", uint64(envelope.ExecutionPayload.BlockNumber),
		"blockHash", envelope.ExecutionPayload.BlockHash.Hex())

	logger.Info("Creating FSM with unsafe head")

	fsm := consensus.NewUnsafeHeadTracker(logger)

	var buf bytes.Buffer
	if _, err := envelope.MarshalSSZ(&buf); err != nil {
		return fmt.Errorf("failed to marshal unsafe head: %w", err)
	}

	logEntry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  buf.Bytes(),
	}

	result := fsm.Apply(logEntry)
	if err, ok := result.(error); ok && err != nil {
		return fmt.Errorf("failed to apply unsafe head to FSM: %w", err)
	}

	logger.Info("FSM initialized with unsafe head")
	logger.Info("Opening Raft stores", "dir", stateDir)

	logStorePath := filepath.Join(stateDir, store.RaftLogDB)
	logStoreDB, err := boltdb.NewBoltStore(logStorePath)
	if err != nil {
		return fmt.Errorf("failed to open log store: %w", err)
	}
	defer logStoreDB.Close()

	stableStorePath := filepath.Join(stateDir, store.RaftStableDB)
	stableStoreDB, err := boltdb.NewBoltStore(stableStorePath)
	if err != nil {
		return fmt.Errorf("failed to open stable store: %w", err)
	}
	defer stableStoreDB.Close()

	snapshotStore, err := raft.NewFileSnapshotStore(stateDir, 1, nil)
	if err != nil {
		return fmt.Errorf("failed to create snapshot store: %w", err)
	}

	logger.Info("Raft stores opened successfully")
	logger.Info("Creating new cluster configuration")

	servers := make([]raft.Server, len(nodes))
	for i, node := range nodes {
		servers[i] = raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(serverIDs[i]),
			Address:  raft.ServerAddress(node.Address),
		}
		logger.Info("  Adding server to config",
			"id", serverIDs[i],
			"address", node.Address,
			"isLeader", serverIDs[i] == initialLeader)
	}

	configuration := raft.Configuration{
		Servers: servers,
	}

	logger.Info("Creating network transport")

	bindAddr := "127.0.0.1:0"
	advertiseAddr, err := net.ResolveTCPAddr("tcp", thisServerAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve advertise address: %w", err)
	}

	transport, err := raft.NewTCPTransport(bindAddr, advertiseAddr, 1, 5*time.Second, nil)
	if err != nil {
		return fmt.Errorf("failed to create transport: %w", err)
	}
	defer transport.Close()

	logger.Info("Running RecoverCluster")
	logger.Warn("This will truncate the Raft log and commit all pending entries!")

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(thisServerID)

	err = raft.RecoverCluster(
		raftConfig,
		fsm,
		logStoreDB,
		stableStoreDB,
		snapshotStore,
		transport,
		configuration,
	)
	if err != nil {
		return fmt.Errorf("RecoverCluster failed: %w", err)
	}

	if thisServerID == initialLeader {
		logger.Info("Setting vote for initial leader", "serverID", thisServerID)
		if err := stableStoreDB.SetUint64([]byte(store.KeyLastVoteTerm), 1); err != nil {
			return fmt.Errorf("failed to set vote term: %w", err)
		}
		if err := stableStoreDB.Set([]byte(store.KeyLastVoteCand), []byte(thisServerID)); err != nil {
			return fmt.Errorf("failed to set vote candidate: %w", err)
		}
		logger.Info("✓ Vote set for initial leader")
	}

	logger.Info("✓ Cluster recovery successful!")
	logger.Info("Recovery summary:")
	logger.Info("  - New cluster configuration applied")
	logger.Info("  - Unsafe head set", "block", uint64(envelope.ExecutionPayload.BlockNumber))
	logger.Info("  - Snapshot created with current FSM state")
	logger.Info("  - Raft log truncated")
	logger.Info("")
	logger.Info("Next steps:")
	logger.Info("  1. Run this command on ALL other nodes with the SAME configuration")
	logger.Info("  2. Start all conductors simultaneously")
	logger.Info("  3. Raft will elect a leader and resume normal operation")

	return nil
}

// parseNodesAndIDs parses the comma-separated nodes and server IDs
func parseNodesAndIDs(nodesStr, serverIDsStr string) ([]NodeConfig, []string, error) {
	nodeAddrs := parseCommaSeparated(nodesStr)
	serverIDs := parseCommaSeparated(serverIDsStr)

	if len(nodeAddrs) != len(serverIDs) {
		return nil, nil, fmt.Errorf("number of nodes (%d) must match number of server IDs (%d)", len(nodeAddrs), len(serverIDs))
	}

	nodes := make([]NodeConfig, len(nodeAddrs))
	for i, addr := range nodeAddrs {
		nodes[i] = NodeConfig{
			ServerID: serverIDs[i],
			Address:  addr,
		}
	}

	return nodes, serverIDs, nil
}

func parseCommaSeparated(s string) []string {
	var result []string
	for part := range bytes.SplitSeq([]byte(s), []byte(",")) {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) > 0 {
			result = append(result, string(trimmed))
		}
	}
	return result
}

type NodeConfig struct {
	ServerID string
	Address  string
}
