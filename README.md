# op-cdtr

A tool to initialize Raft state for op-conductor clusters, eliminating the need for a dedicated bootstrap node.

## Overview

`op-cdtr` provides two approaches for managing op-conductor clusters:

1. **Raft State Generation**: Pre-generate Raft state files (BoltDB databases) that allow all op-conductor nodes to start as equals without requiring a special bootstrap sequencer
2. **Bootstrap Cluster**: Bootstrap a live op-conductor cluster using the standard op-conductor configuration

## Commands

The tool is organized into two main command groups:

### `raft` - Raft State Management

Commands for initializing and managing Raft state files for op-conductor high-availability clusters.

```bash
# Generate Raft state
op-cdtr raft generate \
  --nodes sequencer-1.namespace.svc.cluster.local:50050,sequencer-2.namespace.svc.cluster.local:50050,sequencer-3.namespace.svc.cluster.local:50050 \
  --server-ids sequencer-1,sequencer-2,sequencer-3 \
  --initial-leader sequencer-1 \
  --output-dir ./raft-state

# Show detailed state information
op-cdtr raft info --state-dir ./raft-state/sequencer-1

# Edit unsafe head in Raft state
op-cdtr raft edit unsafehead \
  --state-dir ./raft-state/sequencer-1 \
  --execution-rpc http://op-geth:8545 \
  --block-number 12345
```

### `bootstrap` - Bootstrap Cluster

Bootstrap a new op-conductor cluster with the specified configuration.

```bash
# Bootstrap cluster using op-conductor configuration
op-cdtr bootstrap cluster [op-conductor flags]
```

### Environment Variables

For the `raft` commands, all flags can be set via environment variables with the `OP_CDTR_` prefix:

```bash
export OP_CDTR_NODES="sequencer-1:50050,sequencer-2:50050,sequencer-3:50050"
export OP_CDTR_SERVER_IDS="sequencer-1,sequencer-2,sequencer-3"
export OP_CDTR_INITIAL_LEADER="sequencer-1"
export OP_CDTR_OUTPUT_DIR="./raft-state"
op-cdtr raft generate
```

## Raft Command Reference

#### `raft generate` - Generate Raft state

Flags:

- `--nodes` (required): Comma-separated list of node addresses with Raft consensus ports
- `--server-ids` (required): Comma-separated list of server IDs (must match the order of nodes)
- `--initial-leader` (required): Server ID of the initial Raft leader
- `--output-dir`: Output directory for generated state files (default: `./raft-state`)
- `--initial-term`: Initial Raft term (default: 1)
- `--network`: Network name for configuration
- `--force`: Force overwrite existing state files (default: false)

**Safety Note**: The tool will refuse to overwrite existing state files unless you use the `--force` flag.

#### `raft recover` - Recover cluster with new configuration

Uses HashiCorp Raft's `RecoverCluster` to force a new cluster configuration while setting a specific unsafe head. This is designed for disaster recovery scenarios when quorum is lost or cluster state is corrupted.

Flags:

- `--state-dir` (required): Directory containing raft state files to recover
- `--nodes` (required): Comma-separated list of node addresses for the new cluster
- `--server-ids` (required): Comma-separated list of server IDs for the new cluster
- `--initial-leader` (required): Server ID of the initial leader in the new cluster
- `--execution-rpc` (required): Execution layer RPC endpoint to get unsafe head
- `--block-number` (optional): Specific block number to use as unsafe head
- `--block-hash` (optional): Specific block hash to use as unsafe head
- `--force`: Required flag to confirm this dangerous operation

**How it works:**
1. Fetches the specified unsafe head from execution layer (or latest if not specified)
2. Creates an FSM snapshot with that unsafe head
3. Calls `RecoverCluster` with the new cluster configuration
4. Truncates existing Raft log and commits all pending entries
5. Creates a fresh cluster state with the new configuration and unsafe head

**Use cases:**
- Recover from lost quorum (multiple nodes failed)
- Fix corrupted Raft state across the cluster
- Reset cluster with fresh configuration and current chain state
- Disaster recovery when normal recovery procedures fail

**Safety Note**: This is an EXTREMELY DANGEROUS operation. It implicitly commits all Raft log entries and forces a new configuration. Only use when you've lost quorum and cannot recover through normal means.

**Typical recovery workflow:**
```bash
# 1. Stop all conductors
kubectl scale statefulset -n namespace sequencer-1 sequencer-2 sequencer-3 --replicas=0

# 2. Run recover on EACH node's state directory
# (Use kubectl exec with a debug pod that mounts the PVC)
op-cdtr raft recover \
  --state-dir /state/conductor \
  --nodes 10.43.101.2:50050,10.43.101.3:50050,10.43.101.4:50050 \
  --server-ids sequencer-1,sequencer-2,sequencer-3 \
  --initial-leader sequencer-2 \
  --execution-rpc http://execution-layer:8545 \
  --force

# 3. Restart all conductors
kubectl scale statefulset -n namespace sequencer-1 sequencer-2 sequencer-3 --replicas=1

# 4. Cluster will form automatically with the new configuration
```

#### `raft search` - Search for specific log entries

Search for log entries by index range, term, type, or block number.

Flags:

- `--state-dir` (required): Directory containing raft state files
- `--index`: Search for a specific log entry by index
- `--term`: Search for log entries in a specific term
- `--type`: Search for log entries of a specific type (command, noop, configuration, barrier)
- `--block-number`: Search for log entry containing a specific block number (command entries only)
- `--start-index`: Start of index range to search
- `--end-index`: End of index range to search
- `--latest`: Show only the latest unsafe head (most recent command entry)
- `--no-pager`: Disable pager for output (print directly to stdout)

#### `raft info` - Show detailed state information

Displays comprehensive information about Raft state including:

- Current term and voting information
- Node role (Leader/Follower)
- Log entries and cluster configuration
- All cluster members and their addresses

Flags:

- `--state-dir` (required): Directory containing raft state files
- `--no-pager`: Disable pager for output (print directly to stdout)

#### `raft edit unsafehead` - Edit unsafe head in Raft state

Updates the Raft consensus state with a specific unsafe head from the execution layer. Use this when Raft state is stale or needs correction. If no block number or hash is specified, defaults to the latest unsafe block from the execution layer.

Flags:

- `--state-dir` (required): Directory containing raft state files to edit
- `--execution-rpc` (required): Execution layer RPC endpoint (e.g., http://op-geth:8545)
- `--block-number` (optional): Specific block number to set as unsafe head
- `--block-hash` (optional): Specific block hash to set as unsafe head

**Use cases:**
- Initialize Raft FSM with first unsafe head after generating state
- Recover from stale or corrupted unsafe head in Raft consensus
- Manually sync Raft state with execution layer after network interruption

**Safety Note**: This command directly modifies Raft state. Ensure the conductor is stopped before running this command.

## Bootstrap Command Reference

#### `bootstrap cluster` - Bootstrap op-conductor cluster

The bootstrap cluster command accepts all standard op-conductor flags. Refer to the op-conductor documentation for the complete list of available flags.

## Output Structure

When using `raft generate`, the tool creates the following directory structure:

```
raft-state/
├── sequencer-1/
│   ├── raft-log.db      # Contains initial configuration entry
│   └── raft-stable.db   # Contains term and vote information
├── sequencer-2/
│   ├── raft-log.db
│   └── raft-stable.db
└── sequencer-3/
    ├── raft-log.db
    └── raft-stable.db
```

## Deployment Approaches

### Option 1: Pre-generated Raft State

1. **Generate pre-configured state** using the `raft generate` command

2. **Update sequencer configurations**:
   - Remove the bootstrap sequencer deployment
   - Ensure all sequencers have `--raft.bootstrap=false`
   - Point conductor state directories to the pre-configured state

3. **Deploy all sequencers simultaneously**:
   - All nodes will start with the existing Raft configuration
   - The initial leader will begin sequencing immediately
   - No manual bootstrap or peer addition required

### Option 2: Bootstrap Cluster

1. **Prepare op-conductor configuration** following the standard op-conductor setup

2. **Run the bootstrap command** with appropriate flags:
   ```bash
   op-cdtr bootstrap cluster [op-conductor configuration flags]
   ```

3. **Monitor cluster formation** through op-conductor logs

## Building

Using [just](https://github.com/casey/just):

```bash
cd op-cdtr

# Build the binary
just build

# Run tests
just test
```

Manual build:

```bash
cd op-cdtr
go build -o ./bin/op-cdtr ./cmd/main
```

## Important Notes

1. **State Compatibility**: The generated state is compatible with HashiCorp Raft v1 as used by op-conductor
2. **Initial Leader**: When using `raft generate`, the specified initial leader will have its vote recorded in the stable store
3. **All Nodes Equal**: After initial startup, any node can become leader through normal Raft elections
4. **Disaster Recovery**: The `raft` commands can be re-run to regenerate state if needed
5. **Bootstrap Alternative**: The `bootstrap cluster` command provides a way to initialize clusters using standard op-conductor configuration
