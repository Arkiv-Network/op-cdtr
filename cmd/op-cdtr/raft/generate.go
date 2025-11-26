package raft

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/arkiv-network/op-cdtr/pkg/config"
	"github.com/arkiv-network/op-cdtr/pkg/flags"
	"github.com/arkiv-network/op-cdtr/pkg/generator"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

func GenerateCommand() *cli.Command {
	generateFlags := append([]cli.Flag{
		flags.NodesFlag,
		flags.ServerIDsFlag,
		flags.InitialLeaderFlag,
		flags.OutputDirFlag,
		flags.InitialTermFlag,
		flags.NetworkFlag,
		flags.ForceFlag,
	}, oplog.CLIFlags(flags.EnvVarPrefix)...)

	return &cli.Command{
		Name:        "generate",
		Usage:       "Generate pre-configured Raft state",
		Description: "Generate Raft state files for all nodes in the cluster",
		Action:      GenerateAction,
		Flags:       cliapp.ProtectFlags(generateFlags),
	}
}

// GenerateAction handles the generate subcommand
func GenerateAction(ctx *cli.Context) error {
	logCfg := oplog.ReadCLIConfig(ctx)
	log := oplog.NewLogger(oplog.AppOut(ctx), logCfg)
	oplog.SetGlobalLogHandler(log.Handler())
	opservice.ValidateEnvVars(flags.EnvVarPrefix, flags.Flags, log)

	cfg, err := config.NewConfig(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	gen := generator.New(cfg, log)
	if err := gen.Generate(context.Background()); err != nil {
		return fmt.Errorf("failed to generate raft state: %w", err)
	}

	log.Info("Successfully generated Raft state", "output", cfg.OutputDir)
	return nil
}
