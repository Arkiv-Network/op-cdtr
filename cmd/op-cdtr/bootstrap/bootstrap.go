package bootstrap

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v2"

	bootstrapconductor "github.com/arkiv-network/op-cdtr/pkg/conductor"
	"github.com/ethereum-optimism/optimism/op-conductor/conductor"
	opcflags "github.com/ethereum-optimism/optimism/op-conductor/flags"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:        "bootstrap",
		Usage:       "Bootstrap commands for op-conductor initialization",
		Description: "Commands for bootstrapping op-conductor clusters and related operations",
		Subcommands: []*cli.Command{
			{
				Name:        "cluster",
				Usage:       "Bootstrap a new op-conductor cluster",
				Description: "Initialize and bootstrap a new op-conductor cluster with the specified configuration",
				Action:      cliapp.LifecycleCmd(BootstrapClusterMain),
				Flags:       cliapp.ProtectFlags(opcflags.Flags),
			},
		},
	}
}

// BootstrapClusterMain handles the bootstrap cluster subcommand with lifecycle management
func BootstrapClusterMain(ctx *cli.Context, closeApp context.CancelCauseFunc) (cliapp.Lifecycle, error) {
	logCfg := oplog.ReadCLIConfig(ctx)
	log := oplog.NewLogger(oplog.AppOut(ctx), logCfg)
	oplog.SetGlobalLogHandler(log.Handler())
	opservice.ValidateEnvVars(opcflags.EnvVarPrefix, opcflags.Flags, log)

	cfg, err := conductor.NewConfig(ctx, log)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	c, err := bootstrapconductor.New(
		ctx.Context,
		cfg,
		log,
		"1",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create conductor: %w", err)
	}

	return c, nil
}
