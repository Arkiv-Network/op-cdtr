package flags

import (
	"github.com/urfave/cli/v2"

	opservice "github.com/ethereum-optimism/optimism/op-service"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

const EnvVarPrefix = "OP_CDTR"

var (
	NodesFlag = &cli.StringFlag{
		Name:     "nodes",
		Usage:    "Comma-separated list of node addresses (e.g., sequencer-1:50050,sequencer-2:50050,sequencer-3:50050)",
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "NODES"),
		Required: true,
	}
	ServerIDsFlag = &cli.StringFlag{
		Name:     "server-ids",
		Usage:    "Comma-separated list of server IDs (e.g., sequencer-1,sequencer-2,sequencer-3)",
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "SERVER_IDS"),
		Required: true,
	}
	OutputDirFlag = &cli.StringFlag{
		Name:    "output-dir",
		Usage:   "Output directory for generated Raft state",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "OUTPUT_DIR"),
		Value:   "./raft-state",
	}
	InitialLeaderFlag = &cli.StringFlag{
		Name:     "initial-leader",
		Usage:    "Server ID of the initial leader",
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "INITIAL_LEADER"),
		Required: true,
	}
	InitialTermFlag = &cli.Uint64Flag{
		Name:    "initial-term",
		Usage:   "Initial Raft term",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "INITIAL_TERM"),
		Value:   1,
	}
	NetworkFlag = &cli.StringFlag{
		Name:    "network",
		Usage:   "Network name for configuration (e.g., base-mainnet, op-mainnet)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "NETWORK"),
	}
	ForceFlag = &cli.BoolFlag{
		Name:    "force",
		Usage:   "Force overwrite existing state files",
		Value:   false,
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "FORCE"),
	}
	// Flags for raft subcommands
	StateDirFlag = &cli.StringFlag{
		Name:     "state-dir",
		Usage:    "Directory containing raft state files",
		Required: true,
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "STATE_DIR"),
	}
	BackupDirFlag = &cli.StringFlag{
		Name:     "backup-dir",
		Usage:    "Directory for backup operations",
		Required: true,
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "BACKUP_DIR"),
	}
	RestoreForceFlag = &cli.BoolFlag{
		Name:    "force",
		Usage:   "Force restore without confirmation prompts",
		Value:   false,
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "RESTORE_FORCE"),
	}
	ExecutionRPCFlag = &cli.StringFlag{
		Name:     "execution-rpc",
		Usage:    "Execution layer RPC endpoint (e.g., http://localhost:8545)",
		Required: true,
		EnvVars:  opservice.PrefixEnvVar(EnvVarPrefix, "EXECUTION_RPC"),
	}
	BlockNumberFlag = &cli.Uint64Flag{
		Name:    "block-number",
		Usage:   "Specific block number to use (optional, uses latest if not specified)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "BLOCK_NUMBER"),
	}
	BlockHashFlag = &cli.StringFlag{
		Name:    "block-hash",
		Usage:   "Specific block hash to use (optional, uses latest if not specified)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "BLOCK_HASH"),
	}
	MaxEntriesFlag = &cli.UintFlag{
		Name:    "max-entries",
		Usage:   "Maximum number of log entries to display (0 for all)",
		Value:   10,
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "MAX_ENTRIES"),
	}
	// Search flags
	SearchIndexFlag = &cli.Uint64Flag{
		Name:    "index",
		Usage:   "Search for a specific log entry by index",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_INDEX"),
	}
	SearchTermFlag = &cli.Uint64Flag{
		Name:    "term",
		Usage:   "Search for log entries in a specific term",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_TERM"),
	}
	SearchTypeFlag = &cli.StringFlag{
		Name:    "type",
		Usage:   "Search for log entries of a specific type (command, noop, configuration, barrier)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_TYPE"),
	}
	SearchBlockNumberFlag = &cli.Uint64Flag{
		Name:    "block-number",
		Usage:   "Search for log entry containing a specific block number (command entries only)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_BLOCK_NUMBER"),
	}
	SearchStartIndexFlag = &cli.Uint64Flag{
		Name:    "start-index",
		Usage:   "Start of index range to search",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_START_INDEX"),
	}
	SearchEndIndexFlag = &cli.Uint64Flag{
		Name:    "end-index",
		Usage:   "End of index range to search",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_END_INDEX"),
	}
	SearchLatestFlag = &cli.BoolFlag{
		Name:    "latest",
		Usage:   "Show only the latest unsafe head (most recent command entry)",
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "SEARCH_LATEST"),
	}
	NoPagerFlag = &cli.BoolFlag{
		Name:    "no-pager",
		Usage:   "Disable pager for output (print directly to stdout)",
		Value:   false,
		EnvVars: opservice.PrefixEnvVar(EnvVarPrefix, "NO_PAGER"),
	}
)

var Flags = append([]cli.Flag{
	StateDirFlag,
	MaxEntriesFlag,
	SearchIndexFlag,
	SearchTermFlag,
	SearchTypeFlag,
	SearchBlockNumberFlag,
	SearchStartIndexFlag,
	SearchEndIndexFlag,
	SearchLatestFlag,
	NoPagerFlag,
}, oplog.CLIFlags(EnvVarPrefix)...)
