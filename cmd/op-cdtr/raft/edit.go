package raft

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/hashicorp/raft"
	"github.com/urfave/cli/v2"

	"github.com/arkiv-network/op-cdtr/pkg/flags"
	"github.com/arkiv-network/op-cdtr/pkg/store"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum-optimism/optimism/op-service/sources"
)

func EditCommand() *cli.Command {
	return &cli.Command{
		Name:        "edit",
		Usage:       "Edit Raft state",
		Description: "Commands for editing Raft state values",
		Subcommands: []*cli.Command{
			{
				Name:        "unsafehead",
				Usage:       "Edit unsafe head to a specific block",
				Description: "Updates Raft consensus state with a specific unsafe head from execution layer. Use when Raft state is stale or needs correction. Defaults to latest unsafe block if no block number or hash is specified.",
				Action:      EditUnsafeHeadAction,
				Flags: cliapp.ProtectFlags([]cli.Flag{
					flags.StateDirFlag,
					flags.ExecutionRPCFlag,
					flags.BlockNumberFlag,
					flags.BlockHashFlag,
				}),
			},
		},
	}
}

func EditUnsafeHeadAction(ctx *cli.Context) error {
	logCfg := oplog.ReadCLIConfig(ctx)
	logger := oplog.NewLogger(oplog.AppOut(ctx), logCfg)
	oplog.SetGlobalLogHandler(logger.Handler())
	opservice.ValidateEnvVars(flags.EnvVarPrefix, flags.Flags, logger)

	stateDir := ctx.String(flags.StateDirFlag.Name)
	executionRPC := ctx.String(flags.ExecutionRPCFlag.Name)
	blockNumberFlag := ctx.Uint64("block-number")
	blockHashFlag := ctx.String("block-hash")

	logger.Info("Editing Raft unsafe head",
		"stateDir", stateDir,
		"executionRPC", executionRPC)

	ctxBg := context.Background()

	// Create RPC client
	logger.Info("Connecting to execution layer")
	rpcClient, err := client.NewRPC(ctxBg, logger, executionRPC)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %w", err)
	}
	defer rpcClient.Close()

	// Create L2 client to get execution payload envelope
	l2Config := sources.L2ClientDefaultConfig(&rollup.Config{}, true)
	l2Client, err := sources.NewL2Client(rpcClient, logger, nil, l2Config)
	if err != nil {
		return fmt.Errorf("failed to create L2 client: %w", err)
	}

	// Get the payload envelope - either by block number, hash, or latest
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

	blockNum := uint64(envelope.ExecutionPayload.BlockNumber)
	blockHash := envelope.ExecutionPayload.BlockHash

	logger.Info("Retrieved unsafe block",
		"number", blockNum,
		"hash", blockHash.Hex())

	// Marshal payload to SSZ format
	logger.Info("Marshaling payload to SSZ format")
	var buf bytes.Buffer
	if _, err := envelope.MarshalSSZ(&buf); err != nil {
		return fmt.Errorf("failed to marshal payload to SSZ: %w", err)
	}
	logger.Info("Payload marshaled", "size", buf.Len())

	// Open stable store
	logger.Info("Opening Raft stores")
	stablePath := filepath.Join(stateDir, store.RaftStableDB)
	stableReader, err := store.OpenStableStore(stablePath)
	if err != nil {
		return err
	}
	defer stableReader.Close() //nolint:errcheck

	// Get current term
	currentTerm, err := stableReader.CurrentTerm()
	if err != nil {
		logger.Warn("Failed to read current term, using default", "term", 1, "err", err)
		currentTerm = 1
	}

	// Open log store
	logPath := filepath.Join(stateDir, store.RaftLogDB)
	logReader, err := store.OpenLogStore(logPath)
	if err != nil {
		return err
	}
	defer logReader.Close() //nolint:errcheck

	// Get last index
	lastIdx, err := logReader.LastIndex()
	if err != nil {
		return fmt.Errorf("failed to get last index: %w", err)
	}

	// Create new log entry
	newIdx := lastIdx + 1
	logEntry := &raft.Log{
		Index: newIdx,
		Term:  currentTerm,
		Type:  raft.LogCommand,
		Data:  buf.Bytes(),
	}

	logger.Info("Storing unsafe head to Raft log", "index", newIdx, "term", currentTerm)
	if err := logReader.StoreLog(logEntry); err != nil {
		return fmt.Errorf("failed to store log entry: %w", err)
	}

	logger.Info("Successfully edited unsafe head in Raft state",
		"blockNumber", blockNum,
		"blockHash", blockHash.Hex(),
		"logIndex", newIdx,
		"term", currentTerm,
		"payloadSize", buf.Len())

	return nil
}
