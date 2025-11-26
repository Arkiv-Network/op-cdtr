package raft

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/arkiv-network/op-cdtr/pkg/flags"
	"github.com/arkiv-network/op-cdtr/pkg/output"
	"github.com/arkiv-network/op-cdtr/pkg/store"
	"github.com/hashicorp/raft"
	"github.com/urfave/cli/v2"
)

func SearchCommand() *cli.Command {
	return &cli.Command{
		Name:        "search",
		Usage:       "Search for specific log entries",
		Description: "Search for log entries by index range, term, type, or block number",
		Action:      SearchAction,
		Flags: []cli.Flag{
			flags.StateDirFlag,
			flags.SearchIndexFlag,
			flags.SearchTermFlag,
			flags.SearchTypeFlag,
			flags.SearchBlockNumberFlag,
			flags.SearchStartIndexFlag,
			flags.SearchEndIndexFlag,
			flags.SearchLatestFlag,
			flags.NoPagerFlag,
		},
	}
}

// SearchAction handles the search subcommand
func SearchAction(ctx *cli.Context) error {
	stateDir := ctx.String("state-dir")
	searchIndex := ctx.Uint64("index")
	searchTerm := ctx.Uint64("term")
	searchType := ctx.String("type")
	searchBlockNumber := ctx.Uint64("block-number")
	startIndex := ctx.Uint64("start-index")
	endIndex := ctx.Uint64("end-index")
	latest := ctx.Bool("latest")
	noPager := ctx.Bool("no-pager")

	// Open log store
	logPath := filepath.Join(stateDir, store.RaftLogDB)
	logReader, err := store.OpenLogStore(logPath)
	if err != nil {
		return err
	}
	defer logReader.Close() //nolint:errcheck

	// Get index bounds
	firstIndex, err := logReader.FirstIndex()
	if err != nil {
		return fmt.Errorf("failed to get first index: %w", err)
	}

	lastIndex, err := logReader.LastIndex()
	if err != nil {
		return fmt.Errorf("failed to get last index: %w", err)
	}

	// Determine search range
	searchStart := firstIndex
	searchEnd := lastIndex

	// If --latest flag is set, search backwards from last index for command entries
	if latest {
		searchType = "command" // Force type to command for latest
		// We'll search backwards and return the first match
	} else {
		if startIndex > 0 {
			searchStart = startIndex
		}
		if endIndex > 0 {
			searchEnd = endIndex
		}

		// Validate range
		if searchStart < firstIndex {
			searchStart = firstIndex
		}
		if searchEnd > lastIndex {
			searchEnd = lastIndex
		}

		// If searching by specific index, narrow the range
		if searchIndex > 0 {
			searchStart = searchIndex
			searchEnd = searchIndex
		}
	}

	// Search for matching entries
	var matchingEntries []LogEntryData

	// If --latest, search backwards in log first, then try snapshot
	if latest {
		// First, search backwards through log for latest command entry with unsafe head
		found := false
		for idx := lastIndex; idx >= firstIndex; idx-- {
			var logEntry raft.Log
			if err := logReader.GetLog(idx, &logEntry); err != nil {
				continue
			}

			// Only look for command entries with data (unsafe heads)
			if logEntry.Type == raft.LogCommand && len(logEntry.Data) > 0 {
				unsafeHead := decodeUnsafeHead(logEntry.Data)
				if unsafeHead != nil {
					entryData := LogEntryData{
						Index:      logEntry.Index,
						Term:       logEntry.Term,
						Type:       logEntry.Type,
						DataSize:   len(logEntry.Data),
						UnsafeHead: unsafeHead,
					}
					matchingEntries = append(matchingEntries, entryData)
					found = true
					break // Found the latest in log, stop searching
				}
			}
		}

		// If not found in log, try snapshot as fallback
		if !found {
			envelope, err := store.ReadLatestSnapshot(stateDir)
			if err == nil && envelope != nil {
				// Found in snapshot
				matchingEntries = append(matchingEntries, LogEntryData{
					Index:    0, // Snapshot doesn't have a specific index
					Term:     0, // Will show as snapshot
					Type:     raft.LogCommand,
					DataSize: 0,
					UnsafeHead: &UnsafeHeadData{
						BlockNumber: uint64(envelope.ExecutionPayload.BlockNumber),
						BlockHash:   envelope.ExecutionPayload.BlockHash.Hex(),
					},
				})
			}
		}
	} else {
		// Normal forward search
		for idx := searchStart; idx <= searchEnd; idx++ {
			var logEntry raft.Log
			if err := logReader.GetLog(idx, &logEntry); err != nil {
				continue
			}

			// Apply filters
			matches := true

			if searchTerm > 0 && logEntry.Term != searchTerm {
				matches = false
			}

			if searchType != "" {
				typeMatches := false
				switch searchType {
				case "command":
					typeMatches = (logEntry.Type == raft.LogCommand)
				case "noop":
					typeMatches = (logEntry.Type == raft.LogNoop)
				case "configuration":
					typeMatches = (logEntry.Type == raft.LogConfiguration)
				case "barrier":
					typeMatches = (logEntry.Type == raft.LogBarrier)
				}
				if !typeMatches {
					matches = false
				}
			}

			// Filter by block number (only for command entries)
			if searchBlockNumber > 0 && logEntry.Type == raft.LogCommand && len(logEntry.Data) > 0 {
				unsafeHead := decodeUnsafeHead(logEntry.Data)
				if unsafeHead == nil || unsafeHead.BlockNumber != searchBlockNumber {
					matches = false
				}
			}

			if matches {
				entryData := LogEntryData{
					Index:    logEntry.Index,
					Term:     logEntry.Term,
					Type:     logEntry.Type,
					DataSize: len(logEntry.Data),
				}

				// Decode configuration
				if logEntry.Type == raft.LogConfiguration {
					entryData.Members = decodeConfiguration(logEntry.Data)
				}

				// Decode unsafe head
				if logEntry.Type == raft.LogCommand && len(logEntry.Data) > 0 {
					entryData.UnsafeHead = decodeUnsafeHead(logEntry.Data)
				}

				matchingEntries = append(matchingEntries, entryData)
			}
		}
	}

	// Build markdown output
	md := buildSearchMarkdown(stateDir, matchingEntries, searchStart, searchEnd)

	// Render with standardized output and pager support
	if err := output.RenderMarkdown(md, noPager); err != nil {
		return err
	}
	return nil
}

func buildSearchMarkdown(stateDir string, entries []LogEntryData, searchStart, searchEnd uint64) string {
	var md strings.Builder

	md.WriteString("# 🔍 Raft Log Search Results\n\n")
	md.WriteString(fmt.Sprintf("**Directory:** `%s`\n", stateDir))

	// Add search criteria summary
	md.WriteString("**Search Parameters:**\n")
	if searchStart == searchEnd {
		md.WriteString(fmt.Sprintf("- **Index:** %d\n", searchStart))
	} else {
		md.WriteString(fmt.Sprintf("- **Range:** Index %d to %d\n", searchStart, searchEnd))
	}
	md.WriteString(fmt.Sprintf("- **Matches Found:** %d\n\n", len(entries)))

	if len(entries) == 0 {
		md.WriteString("## 📭 No Results\n\n")
		md.WriteString("*No matching entries found in the specified range*\n")
		return md.String()
	}

	md.WriteString("## 📋 Matching Entries\n\n")

	// Summary table for quick overview
	md.WriteString("| Index | Term | Type |\n")
	md.WriteString("|-------|------|------|\n")

	for _, entry := range entries {
		if entry.Index == 0 && entry.Term == 0 {
			// This is from a snapshot
			md.WriteString("| 📸 | - | Snapshot |\n")
		} else {
			typeEmoji := getLogTypeEmoji(uint8(entry.Type))
			typeName := entry.Type.String()

			md.WriteString(fmt.Sprintf("| %d | %d | %s %s |\n",
				entry.Index, entry.Term, typeEmoji, typeName))
		}
	}

	md.WriteString("\n## 📄 Detailed View\n\n")

	for _, entry := range entries {
		if entry.Index == 0 && entry.Term == 0 {
			// This is from a snapshot
			md.WriteString("### 📸 Snapshot (Latest Unsafe Head)\n\n")
			md.WriteString(fmt.Sprintf("- **Block Number:** `%d`\n", entry.UnsafeHead.BlockNumber))
			md.WriteString(fmt.Sprintf("- **Block Hash:** `%s`\n", entry.UnsafeHead.BlockHash))
			md.WriteString("\n---\n\n")
			continue
		}

		md.WriteString(fmt.Sprintf("### Entry %d (Term %d)\n\n", entry.Index, entry.Term))
		md.WriteString(fmt.Sprintf("- **Type:** %s %s\n", getLogTypeEmoji(uint8(entry.Type)), entry.Type.String()))
		md.WriteString(fmt.Sprintf("- **Size:** %d bytes\n", entry.DataSize))

		// Show cluster members for configuration entries
		if len(entry.Members) > 0 {
			md.WriteString("\n**👥 Cluster Members:**\n\n")
			for _, member := range entry.Members {
				md.WriteString(fmt.Sprintf("- %s\n", member))
			}
		}

		// Show unsafe head for command entries
		if entry.UnsafeHead != nil {
			md.WriteString("\n**⛓️  Unsafe Head:**\n")
			md.WriteString(fmt.Sprintf("- Block Number: `%d`\n", entry.UnsafeHead.BlockNumber))
			md.WriteString(fmt.Sprintf("- Block Hash: `%s`\n", entry.UnsafeHead.BlockHash))
		}

		md.WriteString("\n---\n\n")
	}

	return md.String()
}

func getLogTypeEmoji(logType uint8) string {
	lt := raft.LogType(logType)

	// Use raft library's String() method and add emojis
	switch lt {
	case raft.LogCommand:
		return "📝"
	case raft.LogNoop:
		return "⭕"
	case raft.LogConfiguration:
		return "⚙️"
	case raft.LogAddPeerDeprecated:
		return "➕"
	case raft.LogRemovePeerDeprecated:
		return "➖"
	case raft.LogBarrier:
		return "🚧"
	default:
		return "❓"
	}
}
