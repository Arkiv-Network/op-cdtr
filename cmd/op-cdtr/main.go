package main

import (
	"context"
	"os"

	"github.com/ethereum/go-ethereum/log"
	"github.com/urfave/cli/v2"

	"github.com/arkiv-network/op-cdtr/cmd/op-cdtr/bootstrap"
	"github.com/arkiv-network/op-cdtr/cmd/op-cdtr/raft"
	opservice "github.com/ethereum-optimism/optimism/op-service"
	"github.com/ethereum-optimism/optimism/op-service/ctxinterrupt"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

var (
	Version   = "v0.0.1"
	GitCommit = ""
	GitDate   = ""
)

func main() {
	oplog.SetupDefaults()

	app := cli.NewApp()
	app.Version = opservice.FormatVersion(Version, GitCommit, GitDate, "")
	app.Name = "op-cdtr"
	app.Usage = "Initialize Raft state for op-conductor clusters"
	app.Description = "Tool to initialize Raft state files for op-conductor high-availability clusters"
	app.Commands = []*cli.Command{
		raft.Command(),
		bootstrap.Command(),
	}

	ctx := ctxinterrupt.WithSignalWaiterMain(context.Background())
	err := app.RunContext(ctx, os.Args)
	if err != nil {
		log.Crit("Application failed", "message", err)
	}
}
