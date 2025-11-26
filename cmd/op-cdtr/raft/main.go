package raft

import (
	"github.com/urfave/cli/v2"
)

func Command() *cli.Command {
	return &cli.Command{
		Name:        "raft",
		Usage:       "Raft state management commands",
		Description: "Commands for initializing and managing Raft state files for op-conductor high-availability clusters",
		Subcommands: []*cli.Command{
			GenerateCommand(),
			RecoverCommand(),
			InfoCommand(),
			SearchCommand(),
			EditCommand(),
		},
	}
}
