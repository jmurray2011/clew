package source

import (
	"regexp"
	"time"
)

// Entry represents a single log entry from any source.
type Entry struct {
	Timestamp time.Time
	Message   string
	Stream    string            // Log stream name or filename
	Source    string            // Source identifier (log group, file path)
	Ptr       string            // Pointer for retrieval (CloudWatch @ptr, file://path#line)
	Fields    map[string]string // Extracted/parsed fields
	Context   EntryContext      // Before/after context lines
}

// EntryContext holds context lines around a log entry.
type EntryContext struct {
	Before []Event
	After  []Event
}

// Event is a simple log event used for streaming and context lines.
type Event struct {
	Timestamp time.Time
	Message   string
	Stream    string
}

// QueryParams defines parameters for querying logs.
type QueryParams struct {
	StartTime time.Time
	EndTime   time.Time
	Filter    *regexp.Regexp // Text/regex filter for matching
	Query     string         // Source-specific query (e.g., CloudWatch Insights syntax)
	Limit     int
	Context   int // Lines of context before/after matches
}

// TailParams defines parameters for streaming/tailing logs.
type TailParams struct {
	Filter *regexp.Regexp
}

// StreamInfo describes a log stream or file within a source.
type StreamInfo struct {
	Name      string
	Size      int64
	FirstTime time.Time
	LastTime  time.Time
}

// SourceMetadata holds metadata about a source for caching and evidence.
type SourceMetadata struct {
	Type      string // "cloudwatch", "local", "s3"
	URI       string // Original URI used to open the source
	Profile   string // AWS profile (for cloudwatch)
	Region    string // AWS region (for cloudwatch)
	AccountID string // AWS account ID (for cloudwatch)
}
