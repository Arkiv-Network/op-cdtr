package store

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/hashicorp/raft"
	boltdb "github.com/hashicorp/raft-boltdb/v2"
)

const (
	// Raft database filenames (must match op-conductor's expectations)
	RaftLogDB    = "raft-log.db"
	RaftStableDB = "raft-stable.db"

	// Raft stable store keys (used by raft-boltdb library)
	KeyCurrentTerm  = "CurrentTerm"
	KeyLastVoteTerm = "LastVoteTerm"
	KeyLastVoteCand = "LastVoteCand"
)

// CreateStableStore creates the stable store for a node using HashiCorp's raft-boltdb
func CreateStableStore(nodeDir string, serverID string, initialTerm uint64, isLeader bool) error {
	dbPath := filepath.Join(nodeDir, RaftStableDB)

	// Create stable store using HashiCorp's raft-boltdb
	store, err := boltdb.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create stable store: %w", err)
	}
	defer store.Close() //nolint:errcheck

	// Set current term
	if err := store.SetUint64([]byte(KeyCurrentTerm), initialTerm); err != nil {
		return fmt.Errorf("failed to set current term: %w", err)
	}

	// Set last vote (only for leader)
	if isLeader {
		if err := store.SetUint64([]byte(KeyLastVoteTerm), initialTerm); err != nil {
			return fmt.Errorf("failed to set last vote term: %w", err)
		}
		if err := store.Set([]byte(KeyLastVoteCand), []byte(serverID)); err != nil {
			return fmt.Errorf("failed to set last vote candidate: %w", err)
		}
	}

	return nil
}

// CreateLogStore creates the log store for a node with initial configuration
func CreateLogStore(nodeDir string, configEntry *raft.Log) error {
	dbPath := filepath.Join(nodeDir, RaftLogDB)

	// Create log store using HashiCorp's raft-boltdb
	store, err := boltdb.NewBoltStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to create log store: %w", err)
	}
	defer store.Close() //nolint:errcheck

	// Store the configuration entry
	if err := store.StoreLog(configEntry); err != nil {
		return fmt.Errorf("failed to store configuration entry: %w", err)
	}

	// The raft-boltdb library automatically maintains FirstIndex and LastIndex
	// No need to set them manually

	return nil
}

// StableStoreReader provides high-level read access to stable store
type StableStoreReader struct {
	store *boltdb.BoltStore
}

// OpenStableStore opens a stable store for reading
func OpenStableStore(path string) (*StableStoreReader, error) {
	store, err := boltdb.NewBoltStore(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open stable store: %w", err)
	}
	return &StableStoreReader{store: store}, nil
}

// Close closes the stable store
func (r *StableStoreReader) Close() error {
	return r.store.Close()
}

// CurrentTerm returns the current term
func (r *StableStoreReader) CurrentTerm() (uint64, error) {
	return r.store.GetUint64([]byte(KeyCurrentTerm))
}

// LastVoteTerm returns the last vote term
func (r *StableStoreReader) LastVoteTerm() (uint64, error) {
	return r.store.GetUint64([]byte(KeyLastVoteTerm))
}

// LastVoteCandidate returns the last vote candidate
func (r *StableStoreReader) LastVoteCandidate() (string, error) {
	data, err := r.store.Get([]byte(KeyLastVoteCand))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// LogStoreReader provides high-level read access to log store
type LogStoreReader struct {
	store *boltdb.BoltStore
}

// OpenLogStore opens a log store for reading
func OpenLogStore(path string) (*LogStoreReader, error) {
	store, err := boltdb.NewBoltStore(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open log store: %w", err)
	}
	return &LogStoreReader{store: store}, nil
}

// Close closes the log store
func (r *LogStoreReader) Close() error {
	return r.store.Close()
}

// FirstIndex returns the first index
func (r *LogStoreReader) FirstIndex() (uint64, error) {
	return r.store.FirstIndex()
}

// LastIndex returns the last index
func (r *LogStoreReader) LastIndex() (uint64, error) {
	return r.store.LastIndex()
}

// GetLog retrieves a log entry at the given index
func (r *LogStoreReader) GetLog(idx uint64, log *raft.Log) error {
	return r.store.GetLog(idx, log)
}

// StoreLog stores a log entry (for write operations)
func (r *LogStoreReader) StoreLog(log *raft.Log) error {
	return r.store.StoreLog(log)
}

// ReadLatestSnapshot reads the latest snapshot and returns the unsafe head
func ReadLatestSnapshot(stateDir string) (*eth.ExecutionPayloadEnvelope, error) {
	snapshotStore, err := raft.NewFileSnapshotStore(stateDir, 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Get list of snapshots
	snapshots, err := snapshotStore.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, fmt.Errorf("no snapshots found")
	}

	// Get the latest snapshot (first in list)
	latestSnapshot := snapshots[0]

	// Open the snapshot
	_, reader, err := snapshotStore.Open(latestSnapshot.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer reader.Close() //nolint:errcheck

	// Read the snapshot data
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(reader); err != nil {
		return nil, fmt.Errorf("failed to read snapshot data: %w", err)
	}

	// Decode the unsafe head
	envelope := &eth.ExecutionPayloadEnvelope{}

	// Try BlockV4 first, then V3
	if err := envelope.UnmarshalSSZ(eth.BlockV4, uint32(buf.Len()), bytes.NewReader(buf.Bytes())); err == nil { //nolint:gosec
		return envelope, nil
	}

	if err := envelope.UnmarshalSSZ(eth.BlockV3, uint32(buf.Len()), bytes.NewReader(buf.Bytes())); err == nil { //nolint:gosec
		return envelope, nil
	}

	return nil, fmt.Errorf("failed to decode snapshot data")
}
