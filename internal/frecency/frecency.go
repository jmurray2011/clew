// Package frecency tracks log group usage and suggests groups ranked by
// frequency × recency.
package frecency

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// MaxEntries is the maximum number of history entries to keep.
	MaxEntries = 500
)

// Entry is a single history record.
type Entry struct {
	Profile  string    `json:"profile"`
	LogGroup string    `json:"log_group"`
	Time     time.Time `json:"time"`
}

// Suggestion is a log group ranked by frecency.
type Suggestion struct {
	LogGroup string
	Score    float64
	LastUsed time.Time
	Count    int
}

// historyPath returns the path to the history file.
func historyPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".clew", "history.json")
}

// Record appends a query to the history file.
func Record(profile, logGroup string) error {
	entries, _ := loadHistory()

	entries = append(entries, Entry{
		Profile:  profile,
		LogGroup: logGroup,
		Time:     time.Now(),
	})

	// FIFO eviction
	if len(entries) > MaxEntries {
		entries = entries[len(entries)-MaxEntries:]
	}

	return saveHistory(entries)
}

// Suggest returns the top N log groups for a profile, ranked by frecency.
func Suggest(profile string, n int) []Suggestion {
	entries, _ := loadHistory()

	// Aggregate per log group
	type stats struct {
		count    int
		lastUsed time.Time
	}
	grouped := make(map[string]*stats)

	for _, e := range entries {
		if e.Profile != profile {
			continue
		}
		s, ok := grouped[e.LogGroup]
		if !ok {
			s = &stats{}
			grouped[e.LogGroup] = s
		}
		s.count++
		if e.Time.After(s.lastUsed) {
			s.lastUsed = e.Time
		}
	}

	// Score: frequency × recency decay
	// recency = 1 / (1 + hours_since_last_use)
	now := time.Now()
	var suggestions []Suggestion
	for lg, s := range grouped {
		hoursSince := now.Sub(s.lastUsed).Hours()
		recency := 1.0 / (1.0 + math.Log1p(hoursSince))
		score := float64(s.count) * recency

		suggestions = append(suggestions, Suggestion{
			LogGroup: lg,
			Score:    score,
			LastUsed: s.lastUsed,
			Count:    s.count,
		})
	}

	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Score > suggestions[j].Score
	})

	if len(suggestions) > n {
		suggestions = suggestions[:n]
	}

	return suggestions
}

// FormatSuggestion returns a human-readable description of when a group was last used.
func FormatSuggestion(s Suggestion) string {
	ago := time.Since(s.LastUsed)
	switch {
	case ago < time.Hour:
		return fmt.Sprintf("%dm ago", int(ago.Minutes()))
	case ago < 24*time.Hour:
		return fmt.Sprintf("%.0fh ago", ago.Hours())
	default:
		return fmt.Sprintf("%.0fd ago", ago.Hours()/24)
	}
}

func loadHistory() ([]Entry, error) {
	path := historyPath()
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func saveHistory(entries []Entry) error {
	path := historyPath()
	if path == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
