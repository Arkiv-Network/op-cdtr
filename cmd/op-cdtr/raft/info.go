package raft

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/arkiv-network/op-cdtr/pkg/flags"
	"github.com/arkiv-network/op-cdtr/pkg/output"
	"github.com/arkiv-network/op-cdtr/pkg/store"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/hashicorp/raft"
	"github.com/urfave/cli/v2"
)

// StableStoreData holds stable store information
type StableStoreData struct {
	LastVoteCand string
	CurrentTerm  uint64
	LastVoteTerm uint64
	HasVoted     bool
}

// LogStoreData holds log store information
type LogStoreData struct {
	Entries    []LogEntryData
	Members    []string
	FirstIndex uint64
	LastIndex  uint64
}

// LogEntryData holds information about a single log entry
type LogEntryData struct {
	ReadError  error
	UnsafeHead *UnsafeHeadData
	Members    []string
	Index      uint64
	Term       uint64
	DataSize   int
	Type       raft.LogType
}

// UnsafeHeadData holds unsafe head information
type UnsafeHeadData struct {
	BlockHash   string
	BlockNumber uint64
}

func InfoCommand() *cli.Command {
	return &cli.Command{
		Name:        "info",
		Usage:       "Show detailed information about Raft state",
		Description: "Display comprehensive information about the Raft state including term, leader, and configuration",
		Action:      InfoAction,
		Flags: []cli.Flag{
			flags.StateDirFlag,
			flags.MaxEntriesFlag,
			flags.NoPagerFlag,
		},
	}
}

// InfoAction handles the info subcommand
func InfoAction(ctx *cli.Context) error {
	stateDir := ctx.String("state-dir")
	maxEntries := ctx.Uint("max-entries")
	noPager := ctx.Bool("no-pager")

	// Collect all data first
	stablePath := filepath.Join(stateDir, store.RaftStableDB)
	stableData, err := readStableStore(stablePath)
	if err != nil {
		return fmt.Errorf("error reading stable store: %w", err)
	}

	logPath := filepath.Join(stateDir, store.RaftLogDB)
	logData, err := readLogStore(logPath, maxEntries)
	if err != nil {
		return fmt.Errorf("error reading log store: %w", err)
	}

	// Build markdown from collected data
	md := buildInfoMarkdown(stateDir, stableData, logData, maxEntries)

	// Render with standardized output and pager support
	if err := output.RenderMarkdown(md, noPager); err != nil {
		return err
	}
	return nil
}

func readStableStore(path string) (*StableStoreData, error) {
	storeReader, err := store.OpenStableStore(path)
	if err != nil {
		return nil, err
	}
	defer storeReader.Close() //nolint:errcheck

	data := &StableStoreData{}

	// Read current term
	data.CurrentTerm, err = storeReader.CurrentTerm()
	if err != nil {
		data.CurrentTerm = 0
	}

	// Read vote information
	data.LastVoteTerm, err = storeReader.LastVoteTerm()
	data.HasVoted = (err == nil)

	data.LastVoteCand, _ = storeReader.LastVoteCandidate() //nolint:errcheck

	return data, nil
}

func readLogStore(path string, maxEntries uint) (*LogStoreData, error) {
	storeReader, err := store.OpenLogStore(path)
	if err != nil {
		return nil, err
	}
	defer storeReader.Close() //nolint:errcheck

	data := &LogStoreData{}

	// Get index information
	data.FirstIndex, err = storeReader.FirstIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to get first index: %w", err)
	}

	data.LastIndex, err = storeReader.LastIndex()
	if err != nil {
		return nil, fmt.Errorf("failed to get last index: %w", err)
	}

	// Read log entries (with limit)
	if data.LastIndex >= data.FirstIndex {
		totalEntries := data.LastIndex - data.FirstIndex + 1
		var entriesToRead uint64

		// If maxEntries is 0, read all entries
		if maxEntries == 0 {
			entriesToRead = totalEntries
		} else {
			entriesToRead = min(totalEntries, uint64(maxEntries))
		}

		// Read entries from FirstIndex
		for i := uint64(0); i < entriesToRead; i++ {
			idx := data.FirstIndex + i
			entryData := readLogEntry(storeReader, idx)
			data.Entries = append(data.Entries, entryData)
			if len(entryData.Members) > 0 {
				data.Members = entryData.Members
			}
		}
	}

	return data, nil
}

func readLogEntry(storeReader *store.LogStoreReader, idx uint64) LogEntryData {
	entryData := LogEntryData{
		Index: idx,
	}

	var logEntry raft.Log
	if err := storeReader.GetLog(idx, &logEntry); err != nil {
		entryData.ReadError = err
		return entryData
	}

	entryData.Term = logEntry.Term
	entryData.Type = logEntry.Type
	entryData.DataSize = len(logEntry.Data)

	// Decode configuration entry
	if logEntry.Type == raft.LogConfiguration {
		entryData.Members = decodeConfiguration(logEntry.Data)
	}

	// Decode unsafe head from command entry
	if logEntry.Type == raft.LogCommand && len(logEntry.Data) > 0 {
		entryData.UnsafeHead = decodeUnsafeHead(logEntry.Data)
	}

	return entryData
}

func decodeUnsafeHead(data []byte) *UnsafeHeadData {
	envelope := &eth.ExecutionPayloadEnvelope{}

	// Try BlockV4 first, then V3
	if err := envelope.UnmarshalSSZ(eth.BlockV4, uint32(len(data)), bytes.NewReader(data)); err == nil { //nolint:gosec
		return &UnsafeHeadData{
			BlockNumber: uint64(envelope.ExecutionPayload.BlockNumber),
			BlockHash:   envelope.ExecutionPayload.BlockHash.Hex(),
		}
	}

	if err := envelope.UnmarshalSSZ(eth.BlockV3, uint32(len(data)), bytes.NewReader(data)); err == nil { //nolint:gosec
		return &UnsafeHeadData{
			BlockNumber: uint64(envelope.ExecutionPayload.BlockNumber),
			BlockHash:   envelope.ExecutionPayload.BlockHash.Hex(),
		}
	}

	return nil
}

func buildInfoMarkdown(stateDir string, stable *StableStoreData, logs *LogStoreData, maxEntries uint) string {
	var md strings.Builder

	md.WriteString("# Raft State Information\n\n")
	md.WriteString(fmt.Sprintf("**Directory:** `%s`\n\n", stateDir))

	// Stable Store Section
	md.WriteString("## Stable Store (`raft-stable.db`)\n\n")
	md.WriteString("| Property | Value |\n")
	md.WriteString("|----------|-------|\n")
	md.WriteString(fmt.Sprintf("| Current Term | `%d` |\n", stable.CurrentTerm))

	if stable.HasVoted {
		md.WriteString(fmt.Sprintf("| Last Vote Term | `%d` |\n", stable.LastVoteTerm))
		md.WriteString(fmt.Sprintf("| Last Vote Candidate | `%s` |\n", stable.LastVoteCand))
		role := determineRoleMarkdown(stable.LastVoteCand)
		md.WriteString(fmt.Sprintf("| Node Role | %s |\n", role))
	} else {
		md.WriteString("| Node Role | 🔵 **Follower** (no vote recorded) |\n")
	}

	// Log Store Section
	md.WriteString("\n## Log Store (`raft-log.db`)\n\n")
	md.WriteString(fmt.Sprintf("- **First Index:** %d\n", logs.FirstIndex))
	md.WriteString(fmt.Sprintf("- **Last Index:** %d\n", logs.LastIndex))

	totalEntries := uint64(0)
	if logs.LastIndex >= logs.FirstIndex {
		totalEntries = logs.LastIndex - logs.FirstIndex + 1
		md.WriteString(fmt.Sprintf("- **Total Entries:** %d\n", totalEntries))
		if maxEntries > 0 && totalEntries > uint64(maxEntries) {
			md.WriteString(fmt.Sprintf("- **Showing:** First %d entries (use `--max-entries=0` to show all)\n\n", maxEntries))
		} else {
			md.WriteString("\n")
		}
	} else {
		md.WriteString("- **Total Entries:** 0\n\n")
	}

	// Log Entries
	md.WriteString("### Log Entries\n\n")

	if len(logs.Entries) > 0 {
		// Summary table for quick overview
		md.WriteString("| Index | Term | Type |\n")
		md.WriteString("|-------|------|------|\n")

		for _, entry := range logs.Entries {
			if entry.ReadError != nil {
				md.WriteString(fmt.Sprintf("| %d | - | ❌ Error: %v |\n", entry.Index, entry.ReadError))
				continue
			}

			typeEmoji := getLogTypeEmoji(uint8(entry.Type))
			typeName := entry.Type.String()

			md.WriteString(fmt.Sprintf("| %d | %d | %s %s |\n",
				entry.Index, entry.Term, typeEmoji, typeName))
		}
	} else {
		md.WriteString("*No log entries*\n\n")
	}

	// Cluster Configuration Summary
	if len(logs.Members) > 0 {
		md.WriteString("### Current Cluster Configuration\n\n")
		md.WriteString(fmt.Sprintf("**Total Members:** %d\n\n", len(logs.Members)))
		md.WriteString("| # | Server ID | Address | Role |\n")
		md.WriteString("|---|-----------|---------|------|\n")
		for i, member := range logs.Members {
			md.WriteString(fmt.Sprintf("| %d | %s |\n", i+1, member))
		}
	}

	return md.String()
}

func determineRoleMarkdown(votedFor string) string {
	if votedFor != "" {
		return "🟢 **Leader** (voted for self)"
	}
	return "🔵 **Follower**"
}

func decodeConfiguration(data []byte) []string {
	if len(data) < 1 {
		return nil
	}

	config := raft.DecodeConfiguration(data)

	var members []string
	for _, server := range config.Servers {
		emoji := "❓"
		suffrageName := "Unknown"

		switch server.Suffrage {
		case raft.Voter:
			emoji = "✅"
			suffrageName = "Voter"
		case raft.Nonvoter:
			emoji = "⭕"
			suffrageName = "NonVoter"
		case raft.Staging:
			emoji = "🔄"
			suffrageName = "Staging"
		}

		members = append(members, fmt.Sprintf("**%s** (`%s`) | %s %s",
			string(server.ID),
			string(server.Address),
			emoji,
			suffrageName))
	}

	return members
}
