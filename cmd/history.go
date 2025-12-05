package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	historyClear bool
	historyRun   int
)

// HistoryEntry represents a single query in history.
type HistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	SourceURI   string    `json:"source_uri,omitempty"`   // Generic source URI (cloudwatch://, file://, etc.)
	SourceType  string    `json:"source_type,omitempty"`  // cloudwatch, local, s3
	Profile     string    `json:"profile,omitempty"`      // AWS profile used (local reference)
	AccountID   string    `json:"account_id,omitempty"`   // AWS account ID (universal identifier)
	LogGroups   []string  `json:"log_groups,omitempty"`   // Deprecated: use SourceURI
	StartTime   string    `json:"start_time"`
	EndTime     string    `json:"end_time,omitempty"`
	Filter      string    `json:"filter,omitempty"`
	Query       string    `json:"query,omitempty"`
	ResultCount int       `json:"result_count,omitempty"`
}

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View query history",
	Long: `View and manage your query history.

Shows recent queries that have been executed, allowing you to
quickly re-run previous queries.

Examples:
  # List recent queries
  clew history

  # Clear all history
  clew history --clear

  # Re-run query #3 from history
  clew history --run 3`,
	RunE: runHistory,
}

func init() {
	rootCmd.AddCommand(historyCmd)

	historyCmd.Flags().BoolVar(&historyClear, "clear", false, "Clear query history")
	historyCmd.Flags().IntVar(&historyRun, "run", 0, "Re-run query by number")
}

func runHistory(cmd *cobra.Command, args []string) error {
	historyFile, err := getHistoryFilePath()
	if err != nil {
		return err
	}

	if historyClear {
		if err := os.Remove(historyFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to clear history: %w", err)
		}
		render.Success("History cleared")
		return nil
	}

	entries, err := loadHistory()
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		render.Info("No query history found.")
		return nil
	}

	// Re-run a specific query
	if historyRun > 0 {
		if historyRun > len(entries) {
			return fmt.Errorf("query #%d not found (history has %d entries)", historyRun, len(entries))
		}
		entry := entries[historyRun-1]
		render.Status("Re-running query from %s...", entry.Timestamp.Format("2006-01-02 15:04:05"))
		render.Newline()

		// Build the source URI from history entry
		var sourceURI string

		// Prefer SourceURI if available (new format)
		if entry.SourceURI != "" {
			sourceURI = entry.SourceURI
		} else if len(entry.LogGroups) > 0 {
			// Fall back to building cloudwatch:// from LogGroups (legacy format)
			sourceURI = "cloudwatch://" + entry.LogGroups[0]
			if entry.Profile != "" {
				sourceURI += "?profile=" + entry.Profile
			}
		} else {
			return fmt.Errorf("history entry has no source URI or log groups")
		}

		// Set the flag variables
		startTime = entry.StartTime
		if entry.EndTime != "" {
			endTime = entry.EndTime
		}
		filter = entry.Filter
		queryString = entry.Query

		// Execute the query with the source URI as argument
		return runQuery(cmd, []string{sourceURI})
	}

	// Display history
	for i, entry := range entries {
		num := ui.LabelStyle.Render(fmt.Sprintf("[%d]", i+1))
		ts := ui.MutedStyle.Render(entry.Timestamp.Format("2006-01-02 15:04:05"))

		// Prefer SourceURI, fall back to LogGroups
		var sourceDisplay string
		if entry.SourceURI != "" {
			sourceDisplay = truncateString(entry.SourceURI, 40)
		} else {
			sourceDisplay = truncateString(formatLogGroups(entry.LogGroups), 40)
		}
		source := ui.SuccessStyle.Render(sourceDisplay)

		// Build filter/query summary
		var queryInfo string
		if entry.Query != "" {
			queryInfo = "-q (custom)"
		} else if entry.Filter != "" {
			queryInfo = fmt.Sprintf("-f %q", truncateString(entry.Filter, 30))
		}

		var resultInfo string
		if entry.ResultCount > 0 {
			resultInfo = ui.MutedStyle.Render(fmt.Sprintf("(%d results)", entry.ResultCount))
		}

		fmt.Printf("%s %s  %s  %s  -s %s  %s\n", num, ts, source, queryInfo, entry.StartTime, resultInfo)
	}

	render.Newline()
	render.Info("Use 'clew history --run N' to re-run a query")
	return nil
}

func getHistoryFilePath() (string, error) {
	// Check config for custom history file path
	if historyFile := viper.GetString("history_file"); historyFile != "" {
		// Expand ~ if present
		if len(historyFile) > 0 && historyFile[0] == '~' {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home directory: %w", err)
			}
			historyFile = filepath.Join(home, historyFile[1:])
		}
		return historyFile, nil
	}

	// Default location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".clew_history.json"), nil
}

// getMaxHistoryEntries returns the configured max history size (default 50)
func getMaxHistoryEntries() int {
	max := viper.GetInt("history_max")
	if max <= 0 {
		return 50 // default
	}
	return max
}

func loadHistory() ([]HistoryEntry, error) {
	historyPath, err := getHistoryFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(historyPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read history: %w", err)
	}

	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse history: %w", err)
	}

	return entries, nil
}

// AddToHistory adds a query to the history file.
// sourceURI is the primary source identifier (e.g., "cloudwatch:///log-group", "file:///path/to/file")
// sourceType is the source type ("cloudwatch", "local", "s3")
// logGroups is deprecated but kept for backward compatibility with older history entries
func AddToHistory(sourceURI, sourceType string, logGroups []string, start, end, filterStr, query string, resultCount int, profile, accountID string) error {
	entries, err := loadHistory()
	if err != nil {
		entries = []HistoryEntry{}
	}

	entry := HistoryEntry{
		Timestamp:   time.Now(),
		SourceURI:   sourceURI,
		SourceType:  sourceType,
		Profile:     profile,
		AccountID:   accountID,
		LogGroups:   logGroups, // Keep for backward compatibility
		StartTime:   start,
		EndTime:     end,
		Filter:      filterStr,
		Query:       query,
		ResultCount: resultCount,
	}

	// Prepend new entry
	entries = append([]HistoryEntry{entry}, entries...)

	// Trim to max size (oldest entries are dropped)
	maxEntries := getMaxHistoryEntries()
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}

	historyPath, err := getHistoryFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(historyPath, data, 0600)
}

func formatLogGroups(groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	if len(groups) == 1 {
		return groups[0]
	}
	return fmt.Sprintf("%s (+%d more)", groups[0], len(groups)-1)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
