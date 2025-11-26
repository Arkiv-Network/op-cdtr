package generator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/raft"

	"github.com/arkiv-network/op-cdtr/pkg/config"
	"github.com/arkiv-network/op-cdtr/pkg/store"
)

// Generator handles the generation of pre-configured Raft state
type Generator struct {
	cfg *config.Config
	log log.Logger
}

// New creates a new Generator instance
func New(cfg *config.Config, log log.Logger) *Generator {
	return &Generator{
		cfg: cfg,
		log: log,
	}
}

// Generate creates the pre-configured Raft state for all nodes
func (g *Generator) Generate(ctx context.Context) error {
	g.log.Info("Starting Raft state generation",
		"nodes", len(g.cfg.Nodes),
		"initial_leader", g.cfg.InitialLeader,
		"output_dir", g.cfg.OutputDir,
	)

	// Check for existing files if not forcing
	if !g.cfg.Force {
		existingFiles := []string{}
		for _, node := range g.cfg.Nodes {
			nodeDir := filepath.Join(g.cfg.OutputDir, node.ServerID)
			stableDbPath := filepath.Join(nodeDir, "raft-stable.db")
			logDbPath := filepath.Join(nodeDir, "raft-log.db")

			if _, err := os.Stat(stableDbPath); err == nil {
				existingFiles = append(existingFiles, stableDbPath)
			}
			if _, err := os.Stat(logDbPath); err == nil {
				existingFiles = append(existingFiles, logDbPath)
			}
		}

		if len(existingFiles) > 0 {
			g.log.Error("Existing state files found. Use --force to overwrite", "files", len(existingFiles))
			for _, file := range existingFiles {
				g.log.Error("  Existing file", "path", file)
			}
			return fmt.Errorf("refusing to overwrite %d existing state files without --force flag", len(existingFiles))
		}
	}

	// Create output directory
	if err := os.MkdirAll(g.cfg.OutputDir, 0o755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create configuration entry using official raft types
	configEntry := g.createConfigurationEntry()

	// Generate state for each node
	for _, node := range g.cfg.Nodes {
		isLeader := node.ServerID == g.cfg.InitialLeader

		g.log.Info("Generating state for node",
			"server_id", node.ServerID,
			"address", node.Address,
			"is_leader", isLeader,
		)

		if err := g.createNodeState(node, configEntry, isLeader); err != nil {
			return fmt.Errorf("failed to create state for node %s: %w", node.ServerID, err)
		}
	}

	g.log.Info("Successfully generated Raft state for all nodes")
	g.printSummary()

	return nil
}

// createConfigurationEntry creates the initial Raft configuration log entry
func (g *Generator) createConfigurationEntry() *raft.Log {
	servers := make([]raft.Server, len(g.cfg.Nodes))

	for i, node := range g.cfg.Nodes {
		servers[i] = raft.Server{
			Suffrage: raft.Voter,
			ID:       raft.ServerID(node.ServerID),
			Address:  raft.ServerAddress(node.Address),
		}
	}

	config := raft.Configuration{
		Servers: servers,
	}

	// Encode configuration using official raft encoding
	data := raft.EncodeConfiguration(config)

	return &raft.Log{
		Index: 1,
		Term:  g.cfg.InitialTerm,
		Type:  raft.LogConfiguration,
		Data:  data,
	}
}

// createNodeState creates the Raft state files for a single node
func (g *Generator) createNodeState(node config.NodeConfig, configEntry *raft.Log, isLeader bool) error {
	nodeDir := filepath.Join(g.cfg.OutputDir, node.ServerID)

	// Create node directory
	if err := os.MkdirAll(nodeDir, 0o755); err != nil { //nolint:gosec
		return fmt.Errorf("failed to create node directory: %w", err)
	}

	// Create stable store
	if err := store.CreateStableStore(nodeDir, node.ServerID, g.cfg.InitialTerm, isLeader); err != nil {
		return fmt.Errorf("failed to create stable store: %w", err)
	}

	// Create log store
	if err := store.CreateLogStore(nodeDir, configEntry); err != nil {
		return fmt.Errorf("failed to create log store: %w", err)
	}

	return nil
}

// printSummary prints a summary of the generated state
func (g *Generator) printSummary() {
	g.log.Info("Generation summary:")
	g.log.Info("Generated files:")

	for _, node := range g.cfg.Nodes {
		nodeDir := filepath.Join(g.cfg.OutputDir, node.ServerID)
		g.log.Info("  Node directory", "path", nodeDir)
		g.log.Info("    - raft-log.db")
		g.log.Info("    - raft-stable.db")
	}

	g.log.Info("\nNext steps:")
	g.log.Info("1. Copy the generated state to your persistent volumes")
	g.log.Info("2. Ensure all sequencers have --raft.bootstrap=false")
	g.log.Info("3. Start all sequencers simultaneously")
}
