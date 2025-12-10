// Package timeutil provides shared time parsing utilities.
package timeutil

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Pre-compiled regex for parsing relative time formats (e.g., "2h", "30m", "7d")
var relativeTimeRe = regexp.MustCompile(`^(\d+)([mhd])$`)

// Parse parses a time string that can be either RFC3339 format or a relative
// duration like "2h", "30m", or "7d".
//
// Examples:
//   - "now" or "" -> current time
//   - "2h" -> 2 hours ago
//   - "30m" -> 30 minutes ago
//   - "7d" -> 7 days ago
//   - "2025-12-02T06:00:00Z" -> specific RFC3339 time
func Parse(input string) (time.Time, error) {
	if input == "" || input == "now" {
		return time.Now().UTC(), nil
	}

	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t, nil
	}

	// Parse relative (e.g., "2h", "30m", "7d") using pre-compiled regex
	matches := relativeTimeRe.FindStringSubmatch(input)
	if matches != nil {
		value, _ := strconv.Atoi(matches[1])
		unit := matches[2]
		var duration time.Duration
		switch unit {
		case "m":
			duration = time.Duration(value) * time.Minute
		case "h":
			duration = time.Duration(value) * time.Hour
		case "d":
			duration = time.Duration(value) * 24 * time.Hour
		}
		return time.Now().UTC().Add(-duration), nil
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s - use RFC3339 (2025-12-02T06:00:00Z) or relative (2h, 30m, 7d)", input)
}

// FormatDuration formats a duration in a human-readable way.
func FormatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

// TimeRangeWarning represents a validation warning for a time range.
type TimeRangeWarning struct {
	Message string
	Level   string // "warning" or "info"
}

// ValidateTimeRange checks a time range for potential issues and returns warnings.
// This helps catch user mistakes without blocking the operation.
func ValidateTimeRange(start, end time.Time) []TimeRangeWarning {
	var warnings []TimeRangeWarning
	now := time.Now()

	// Check if end time is significantly in the future (more than 1 minute)
	// This could indicate a typo in the year or other mistake
	if end.After(now.Add(time.Minute)) {
		futureBy := end.Sub(now)
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("end time is %s in the future - is this intentional?", FormatDuration(futureBy)),
			Level:   "warning",
		})
	}

	// Check if start time is in the future
	if start.After(now.Add(time.Minute)) {
		warnings = append(warnings, TimeRangeWarning{
			Message: "start time is in the future - no results will be returned",
			Level:   "warning",
		})
	}

	// Check for very large time ranges (more than 30 days)
	// CloudWatch Insights has limits and costs scale with data scanned
	duration := end.Sub(start)
	if duration > 30*24*time.Hour {
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("querying %s of data - this may be slow and expensive for CloudWatch", FormatDuration(duration)),
			Level:   "info",
		})
	}

	// Check for very short time ranges that might miss data
	if duration < time.Minute && duration > 0 {
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("time range is only %s - you may miss relevant logs", FormatDuration(duration)),
			Level:   "info",
		})
	}

	return warnings
}

// FormatBytes converts bytes to human-readable format (e.g., "1.5 MB").
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
