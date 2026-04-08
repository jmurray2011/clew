package cloudwatch

import (
	"regexp"
	"time"
)

// Entry represents a single log entry returned from a query.
type Entry struct {
	Timestamp time.Time
	Message   string
	Stream    string            // Log stream name (e.g., instance ID)
	Source    string            // Log group name
	Ptr       string            // CloudWatch @ptr for record retrieval
	Fields    map[string]string // All fields from the query result
	Context   EntryContext      // Before/after context lines
}

// EntryContext holds context lines around a log entry.
type EntryContext struct {
	Before []Event
	After  []Event
}

// Event is a simple log event used for streaming (tail) and context lines.
type Event struct {
	Timestamp time.Time
	Message   string
	Stream    string
}

// QueryInput defines parameters for querying logs.
type QueryInput struct {
	StartTime time.Time
	EndTime   time.Time
	Filter    *regexp.Regexp // Text/regex filter for matching
	Query     string         // Raw CloudWatch Insights query (overrides filter)
	Stream    string         // Substring match on @logStream
	Limit     int
	Context   int  // Lines of context before matches
	SortAsc   bool // Sort oldest first (default: newest first)
}

// TailInput defines parameters for tailing logs.
type TailInput struct {
	Filter *regexp.Regexp
	Stream string // Substring match on @logStream
}
