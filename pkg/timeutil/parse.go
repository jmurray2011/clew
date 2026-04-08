// Package timeutil provides shared time parsing utilities.
package timeutil

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	clerrors "github.com/jmurray2011/clew/internal/errors"
)

// Pre-compiled regex for parsing relative time formats (e.g., "2h", "30m", "7d")
var relativeTimeRe = regexp.MustCompile(`^(\d+)([mhd])$`)

// Flexible timestamp formats to try (in order).
// These use time.ParseInLocation so bare timestamps are local time.
var flexibleFormats = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02",
}

// Time-only formats (for ParseTimestamp - around command).
var timeOnlyFormats = []string{
	"15:04:05",
	"15:04",
}

// Parse parses a time string that can be relative, absolute, or a flexible format.
// Bare timestamps without timezone indicators are interpreted in local time.
//
// Accepted formats:
//   - "" or "now" → current time
//   - "2h", "30m", "7d" → relative to now
//   - "2025-01-15 14:30" → local time
//   - "2025-01-15T14:30:00Z" → explicit UTC
//   - "2025-01-15T14:30:00+01:00" → explicit offset
func Parse(input string) (time.Time, error) {
	if input == "" || input == "now" {
		return time.Now().UTC(), nil
	}

	// Try RFC3339 first (has explicit timezone)
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t.UTC(), nil
	}

	// Try RFC3339Nano
	if t, err := time.Parse(time.RFC3339Nano, input); err == nil {
		return t.UTC(), nil
	}

	// Parse relative (e.g., "2h", "30m", "7d")
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

	// Try flexible formats in local timezone
	for _, format := range flexibleFormats {
		if t, err := time.ParseInLocation(format, input, time.Local); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, clerrors.InvalidTimeError(input)
}

// ParseTimestamp parses a timestamp with extra flexibility for the around command.
// In addition to all formats supported by Parse, it also accepts time-only input
// like "14:30" or "14:30:00".
//
// The loc parameter controls how bare timestamps (without explicit timezone) are
// interpreted. Pass time.Local for local time, time.UTC when the timestamp came
// from query output (which is always UTC).
func ParseTimestamp(input string, loc *time.Location) (time.Time, error) {
	// Try time-only formats first (today in the given location)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	for _, format := range timeOnlyFormats {
		if t, err := time.ParseInLocation(format, input, loc); err == nil {
			result := today.Add(t.Sub(time.Date(0, 1, 1, 0, 0, 0, 0, loc)))
			return result.UTC(), nil
		}
	}

	// Fall through to standard Parse, but respect the location for flexible formats
	return ParseInLocation(input, loc)
}

// ParseInLocation works like Parse but uses the given location for bare timestamps.
func ParseInLocation(input string, loc *time.Location) (time.Time, error) {
	if input == "" || input == "now" {
		return time.Now().UTC(), nil
	}

	// Try RFC3339 first (has explicit timezone)
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, input); err == nil {
		return t.UTC(), nil
	}

	// Parse relative
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

	// Try flexible formats in the given location
	for _, format := range flexibleFormats {
		if t, err := time.ParseInLocation(format, input, loc); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, clerrors.InvalidTimeError(input)
}

// ParseDurationString parses a duration string like "3d", "2h", "30m" into time.Duration.
// This is for config values like cost_warning_threshold, not for time range parsing.
func ParseDurationString(input string) (time.Duration, error) {
	// Try Go's standard duration format first (e.g., "72h")
	if d, err := time.ParseDuration(input); err == nil {
		return d, nil
	}

	// Try our relative format (e.g., "3d", "2h", "30m")
	matches := relativeTimeRe.FindStringSubmatch(input)
	if matches != nil {
		value, _ := strconv.Atoi(matches[1])
		unit := matches[2]
		switch unit {
		case "m":
			return time.Duration(value) * time.Minute, nil
		case "h":
			return time.Duration(value) * time.Hour, nil
		case "d":
			return time.Duration(value) * 24 * time.Hour, nil
		}
	}

	return 0, fmt.Errorf("invalid duration: %s (use 30m, 2h, 3d)", input)
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
func ValidateTimeRange(start, end time.Time) []TimeRangeWarning {
	var warnings []TimeRangeWarning
	now := time.Now()

	if end.After(now.Add(time.Minute)) {
		futureBy := end.Sub(now)
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("end time is %s in the future - is this intentional?", FormatDuration(futureBy)),
			Level:   "warning",
		})
	}

	if start.After(now.Add(time.Minute)) {
		warnings = append(warnings, TimeRangeWarning{
			Message: "start time is in the future - no results will be returned",
			Level:   "warning",
		})
	}

	duration := end.Sub(start)
	if duration > 30*24*time.Hour {
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("querying %s of data - this may be slow and expensive for CloudWatch", FormatDuration(duration)),
			Level:   "info",
		})
	}

	if duration < time.Minute && duration > 0 {
		warnings = append(warnings, TimeRangeWarning{
			Message: fmt.Sprintf("time range is only %s - you may miss relevant logs", FormatDuration(duration)),
			Level:   "info",
		})
	}

	return warnings
}

// FormatBytes converts bytes to human-readable format.
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
