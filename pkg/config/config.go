package config

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"

	"github.com/arkiv-network/op-cdtr/pkg/flags"
)

// NodeConfig represents a single node in the Raft cluster
type NodeConfig struct {
	ServerID string
	Address  string
}

// Config holds the configuration for the raft-preconfig tool
type Config struct {
	OutputDir     string
	InitialLeader string
	Network       string
	Nodes         []NodeConfig
	InitialTerm   uint64
	Force         bool
}

// NewConfig creates a new Config from CLI context
func NewConfig(ctx *cli.Context, log log.Logger) (*Config, error) {
	cfg := &Config{
		OutputDir:     ctx.String(flags.OutputDirFlag.Name),
		InitialLeader: ctx.String(flags.InitialLeaderFlag.Name),
		InitialTerm:   ctx.Uint64(flags.InitialTermFlag.Name),
		Network:       ctx.String(flags.NetworkFlag.Name),
		Force:         ctx.Bool(flags.ForceFlag.Name),
	}

	// Parse nodes and server IDs
	nodes, err := parseNodes(ctx.String(flags.NodesFlag.Name), ctx.String(flags.ServerIDsFlag.Name))
	if err != nil {
		return nil, err
	}
	cfg.Nodes = nodes

	// Validate initial leader exists
	leaderFound := false
	for _, node := range cfg.Nodes {
		if node.ServerID == cfg.InitialLeader {
			leaderFound = true
			break
		}
	}
	if !leaderFound {
		return nil, fmt.Errorf("initial leader %s not found in server IDs", cfg.InitialLeader)
	}

	// Log configuration
	log.Info("Loaded configuration",
		"nodes", len(cfg.Nodes),
		"initial_leader", cfg.InitialLeader,
		"initial_term", cfg.InitialTerm,
		"output_dir", cfg.OutputDir,
	)

	return cfg, nil
}

// parseNodes parses the nodes and server IDs flags into NodeConfig slice
func parseNodes(nodesFlag, serverIDsFlag string) ([]NodeConfig, error) {
	nodeAddrs := strings.Split(nodesFlag, ",")
	serverIDs := strings.Split(serverIDsFlag, ",")

	if len(nodeAddrs) != len(serverIDs) {
		return nil, fmt.Errorf("number of nodes (%d) must match number of server IDs (%d)",
			len(nodeAddrs), len(serverIDs))
	}

	nodes := make([]NodeConfig, len(nodeAddrs))
	for i := range nodeAddrs {
		nodes[i] = NodeConfig{
			ServerID: strings.TrimSpace(serverIDs[i]),
			Address:  strings.TrimSpace(nodeAddrs[i]),
		}
	}

	return nodes, nil
}
