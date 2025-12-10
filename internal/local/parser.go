package local

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/source"
)

// Parser interface for parsing log lines.
type Parser interface {
	// ParseLine parses a single line and returns an Entry.
	// Returns nil if the line should be skipped.
	ParseLine(line string, lineNum int, filePath string) *source.Entry

	// IsMultiline returns true if this parser handles multiline entries.
	IsMultiline() bool

	// ShouldJoin returns true if the line should be joined to the previous entry.
	// Only called when IsMultiline() is true.
	ShouldJoin(line string) bool
}

// NewParser creates a parser for the given format.
func NewParser(format Format) Parser {
	switch format {
	case FormatJSON:
		return &JSONParser{}
	case FormatSyslog:
		return &SyslogParser{}
	case FormatJava:
		// Initialize with today's date as default reference for time-only entries
		now := time.Now()
		return &JavaParser{
			referenceDate: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local),
		}
	default:
		return &PlainParser{}
	}
}

// PlainParser treats each line as a separate log entry with no timestamp parsing.
type PlainParser struct{}

func (p *PlainParser) ParseLine(line string, lineNum int, filePath string) *source.Entry {
	if line == "" {
		return nil
	}

	return &source.Entry{
		Timestamp: time.Time{}, // Unknown timestamp for plain format
		Message:   line,
		Stream:    filepath.Base(filePath),
		Source:    filePath,
		Ptr:       source.MakeLocalPtr(filePath, lineNum),
	}
}

func (p *PlainParser) IsMultiline() bool        { return false }
func (p *PlainParser) ShouldJoin(string) bool { return false }

// JSONParser handles JSON Lines format (one JSON object per line).
// Extracts timestamp from common fields and stores other fields in Fields map.
type JSONParser struct{}

// Common timestamp field names in JSON logs
var jsonTimestampFields = []string{
	"timestamp", "time", "@timestamp", "ts", "datetime", "date",
	"Timestamp", "Time", "DateTime", "Date",
}

// Common message field names in JSON logs
var jsonMessageFields = []string{
	"message", "msg", "log", "text", "body",
	"Message", "Msg", "Log", "Text", "Body",
}

func (p *JSONParser) ParseLine(line string, lineNum int, filePath string) *source.Entry {
	line = strings.TrimSpace(line)
	if line == "" || line[0] != '{' {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err != nil {
		// Invalid JSON, treat as plain text
		return &source.Entry{
			Message: line,
			Stream:  filepath.Base(filePath),
			Source:  filePath,
			Ptr:     source.MakeLocalPtr(filePath, lineNum),
		}
	}

	// Extract timestamp
	var ts time.Time
	for _, field := range jsonTimestampFields {
		if val, ok := data[field]; ok {
			ts = parseJSONTimestamp(val)
			if !ts.IsZero() {
				delete(data, field)
				break
			}
		}
	}

	// Extract message
	var message string
	for _, field := range jsonMessageFields {
		if val, ok := data[field]; ok {
			if s, ok := val.(string); ok {
				message = s
				delete(data, field)
				break
			}
		}
	}

	// If no message field found, use the entire JSON as message
	if message == "" {
		message = line
	}

	// Convert remaining fields to string map
	fields := make(map[string]string)
	for k, v := range data {
		switch val := v.(type) {
		case string:
			fields[k] = val
		case float64:
			if val == float64(int64(val)) {
				fields[k] = strconv.FormatInt(int64(val), 10)
			} else {
				fields[k] = strconv.FormatFloat(val, 'f', -1, 64)
			}
		case bool:
			fields[k] = strconv.FormatBool(val)
		default:
			// For nested objects, re-serialize to JSON
			if b, err := json.Marshal(v); err == nil {
				fields[k] = string(b)
			}
		}
	}

	return &source.Entry{
		Timestamp: ts,
		Message:   message,
		Stream:    filepath.Base(filePath),
		Source:    filePath,
		Ptr:       source.MakeLocalPtr(filePath, lineNum),
		Fields:    fields,
	}
}

func (p *JSONParser) IsMultiline() bool        { return false }
func (p *JSONParser) ShouldJoin(string) bool { return false }

// parseJSONTimestamp attempts to parse a timestamp from various formats.
func parseJSONTimestamp(val interface{}) time.Time {
	switch v := val.(type) {
	case string:
		// Try common formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04:05",
			"2006/01/02 15:04:05",
			"02/Jan/2006:15:04:05 -0700", // Common Log Format
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t
			}
		}
	case float64:
		// Unix timestamp (seconds or milliseconds)
		if v > 1e12 {
			// Milliseconds
			return time.UnixMilli(int64(v))
		}
		return time.Unix(int64(v), 0)
	case int64:
		if v > 1e12 {
			return time.UnixMilli(v)
		}
		return time.Unix(v, 0)
	}
	return time.Time{}
}

// SyslogParser handles RFC3164/5424 syslog format.
type SyslogParser struct{}

// RFC3164 pattern: "Jan  2 15:04:05 hostname program[pid]: message"
var syslog3164Pattern = regexp.MustCompile(`^([A-Z][a-z]{2})\s+(\d{1,2})\s+(\d{2}):(\d{2}):(\d{2})\s+(\S+)\s+(.*)$`)

// RFC5424 pattern: "<pri>1 2006-01-02T15:04:05.000000Z hostname app proc msgid structured msg"
var syslog5424Pattern = regexp.MustCompile(`^<(\d+)>1\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$`)

var months = map[string]time.Month{
	"Jan": time.January, "Feb": time.February, "Mar": time.March,
	"Apr": time.April, "May": time.May, "Jun": time.June,
	"Jul": time.July, "Aug": time.August, "Sep": time.September,
	"Oct": time.October, "Nov": time.November, "Dec": time.December,
}

func (p *SyslogParser) ParseLine(line string, lineNum int, filePath string) *source.Entry {
	if line == "" {
		return nil
	}

	entry := &source.Entry{
		Stream: filepath.Base(filePath),
		Source: filePath,
		Ptr:    source.MakeLocalPtr(filePath, lineNum),
		Fields: make(map[string]string),
	}

	// Try RFC5424 first (more specific)
	if matches := syslog5424Pattern.FindStringSubmatch(line); matches != nil {
		// Parse priority
		if pri, err := strconv.Atoi(matches[1]); err == nil {
			entry.Fields["facility"] = strconv.Itoa(pri / 8)
			entry.Fields["severity"] = strconv.Itoa(pri % 8)
		}

		// Parse timestamp
		if ts, err := time.Parse(time.RFC3339Nano, matches[2]); err == nil {
			entry.Timestamp = ts
		} else if ts, err := time.Parse(time.RFC3339, matches[2]); err == nil {
			entry.Timestamp = ts
		}

		entry.Fields["hostname"] = matches[3]
		entry.Fields["app"] = matches[4]
		entry.Fields["procid"] = matches[5]
		entry.Fields["msgid"] = matches[6]
		entry.Message = matches[7]

		return entry
	}

	// Try RFC3164
	if matches := syslog3164Pattern.FindStringSubmatch(line); matches != nil {
		month := months[matches[1]]
		day, _ := strconv.Atoi(matches[2])
		hour, _ := strconv.Atoi(matches[3])
		min, _ := strconv.Atoi(matches[4])
		sec, _ := strconv.Atoi(matches[5])

		// RFC3164 doesn't include year, assume current year
		year := time.Now().Year()
		entry.Timestamp = time.Date(year, month, day, hour, min, sec, 0, time.Local)

		entry.Fields["hostname"] = matches[6]
		entry.Message = matches[7]

		// Try to extract program name from message
		if idx := strings.Index(entry.Message, ":"); idx > 0 {
			progPart := entry.Message[:idx]
			// Check for program[pid] pattern
			if pidIdx := strings.Index(progPart, "["); pidIdx > 0 {
				entry.Fields["program"] = progPart[:pidIdx]
				pid := strings.TrimSuffix(progPart[pidIdx+1:], "]")
				entry.Fields["pid"] = pid
			} else {
				entry.Fields["program"] = progPart
			}
			entry.Message = strings.TrimSpace(entry.Message[idx+1:])
		}

		return entry
	}

	// Fallback to plain text
	entry.Message = line
	return entry
}

func (p *SyslogParser) IsMultiline() bool        { return false }
func (p *SyslogParser) ShouldJoin(string) bool { return false }

// JavaParser handles Java log format with multiline stack traces.
// Supports common formats like Log4j, Logback, java.util.logging.
type JavaParser struct {
	// referenceDate is used for time-only log entries (like logback status).
	// Updated when we encounter a full date, defaults to today.
	referenceDate time.Time
}

// Common Java log patterns - full date patterns first, then time-only patterns
var javaPatterns = []*regexp.Regexp{
	// Pattern 0: Log4j/Logback with millis: "2025-01-15 10:30:45,123 INFO [thread] class - message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s+(\w+)\s+\[([^\]]+)\]\s+(\S+)\s+-\s+(.*)$`),
	// Pattern 1: Log4j2 with millis: "2025-01-15 10:30:45.123 [thread] INFO class - message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2}\.\d{3})\s+\[([^\]]+)\]\s+(\w+)\s+(\S+)\s+-\s+(.*)$`),
	// Pattern 2: Spring Boot/Logback without millis: "2025-01-15 10:30:45 [thread] INFO class - message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})\s+\[([^\]]+)\]\s+(\w+)\s+(\S+)\s+-\s+(.*)$`),
	// Pattern 3: Simple with millis: "2025-01-15 10:30:45,123 INFO message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s+(\w+)\s+(.*)$`),
	// Pattern 4: Simple without millis: "2025-01-15 10:30:45 INFO message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2}:\d{2})\s+(\w+)\s+(.*)$`),
	// Pattern 5: ISO with level: "2025-01-15T10:30:45.123Z INFO message"
	regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z?)\s+(\w+)\s+(.*)$`),
}

// Time-only patterns for logs that lack a date (like logback internal status)
var javaTimeOnlyPatterns = []*regexp.Regexp{
	// Pattern 0: Logback status: "15:07:20,910 |-INFO in ch.qos.logback..."
	regexp.MustCompile(`^(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s+\|-(\w+)\s+in\s+(.*)$`),
	// Pattern 1: Time-only with level: "15:07:20,910 INFO message"
	regexp.MustCompile(`^(\d{2}:\d{2}:\d{2}[,\.]\d{3})\s+(\w+)\s+(.*)$`),
}

// ansiEscapePattern matches ANSI escape sequences
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

func (p *JavaParser) ParseLine(line string, lineNum int, filePath string) *source.Entry {
	if line == "" {
		return nil
	}

	// Strip ANSI escape codes for pattern matching (logs often have color codes)
	cleanLine := stripANSI(line)

	entry := &source.Entry{
		Stream: filepath.Base(filePath),
		Source: filePath,
		Ptr:    source.MakeLocalPtr(filePath, lineNum),
		Fields: make(map[string]string),
	}

	// Try full date patterns first (using cleaned line for matching)
	for i, pattern := range javaPatterns {
		if matches := pattern.FindStringSubmatch(cleanLine); matches != nil {
			switch i {
			case 0: // Log4j/Logback with millis: level [thread] class - message
				entry.Timestamp = parseJavaTimestamp(matches[1], matches[2])
				entry.Fields["level"] = matches[3]
				entry.Fields["thread"] = matches[4]
				entry.Fields["logger"] = matches[5]
				entry.Message = matches[6]
			case 1: // Log4j2 with millis: [thread] level class - message
				entry.Timestamp = parseJavaTimestamp(matches[1], matches[2])
				entry.Fields["thread"] = matches[3]
				entry.Fields["level"] = matches[4]
				entry.Fields["logger"] = matches[5]
				entry.Message = matches[6]
			case 2: // Spring Boot without millis: [thread] level class - message
				entry.Timestamp = parseJavaTimestamp(matches[1], matches[2])
				entry.Fields["thread"] = matches[3]
				entry.Fields["level"] = matches[4]
				entry.Fields["logger"] = matches[5]
				entry.Message = matches[6]
			case 3: // Simple with millis
				entry.Timestamp = parseJavaTimestamp(matches[1], matches[2])
				entry.Fields["level"] = matches[3]
				entry.Message = matches[4]
			case 4: // Simple without millis
				entry.Timestamp = parseJavaTimestamp(matches[1], matches[2])
				entry.Fields["level"] = matches[3]
				entry.Message = matches[4]
			case 5: // ISO format
				if ts, err := time.Parse("2006-01-02T15:04:05.000Z", matches[1]); err == nil {
					entry.Timestamp = ts
				} else if ts, err := time.Parse("2006-01-02T15:04:05.000", matches[1]); err == nil {
					entry.Timestamp = ts
				}
				entry.Fields["level"] = matches[2]
				entry.Message = matches[3]
			}
			// Update reference date from full timestamp for subsequent time-only entries
			if !entry.Timestamp.IsZero() {
				p.referenceDate = time.Date(
					entry.Timestamp.Year(), entry.Timestamp.Month(), entry.Timestamp.Day(),
					0, 0, 0, 0, entry.Timestamp.Location(),
				)
			}
			return entry
		}
	}

	// Try time-only patterns (using reference date)
	for i, pattern := range javaTimeOnlyPatterns {
		if matches := pattern.FindStringSubmatch(cleanLine); matches != nil {
			switch i {
			case 0: // Logback status: "15:07:20,910 |-INFO in ..."
				entry.Timestamp = parseTimeOnly(matches[1], p.referenceDate)
				entry.Fields["level"] = matches[2]
				entry.Message = matches[3]
			case 1: // Time-only with level: "15:07:20,910 INFO message"
				entry.Timestamp = parseTimeOnly(matches[1], p.referenceDate)
				entry.Fields["level"] = matches[2]
				entry.Message = matches[3]
			}
			return entry
		}
	}

	// No pattern matched, return plain entry (with ANSI stripped for readability)
	entry.Message = cleanLine
	return entry
}

func parseJavaTimestamp(datePart, timePart string) time.Time {
	// Normalize comma to dot in milliseconds
	timePart = strings.Replace(timePart, ",", ".", 1)
	combined := datePart + " " + timePart

	// Try with milliseconds first
	if ts, err := time.Parse("2006-01-02 15:04:05.000", combined); err == nil {
		return ts
	}
	// Try without milliseconds
	if ts, err := time.Parse("2006-01-02 15:04:05", combined); err == nil {
		return ts
	}
	return time.Time{}
}

// parseTimeOnly parses a time-only string and combines it with the reference date.
func parseTimeOnly(timePart string, refDate time.Time) time.Time {
	// Normalize comma to dot in milliseconds
	timePart = strings.Replace(timePart, ",", ".", 1)

	// Parse time components
	if ts, err := time.Parse("15:04:05.000", timePart); err == nil {
		return time.Date(
			refDate.Year(), refDate.Month(), refDate.Day(),
			ts.Hour(), ts.Minute(), ts.Second(), ts.Nanosecond(),
			refDate.Location(),
		)
	}
	if ts, err := time.Parse("15:04:05", timePart); err == nil {
		return time.Date(
			refDate.Year(), refDate.Month(), refDate.Day(),
			ts.Hour(), ts.Minute(), ts.Second(), 0,
			refDate.Location(),
		)
	}
	return time.Time{}
}

func (p *JavaParser) IsMultiline() bool { return true }

// Exception class name pattern: com.example.SomeException: message
var exceptionPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*\.[A-Z][A-Za-z0-9_]*(Exception|Error|Throwable)`)

func (p *JavaParser) ShouldJoin(line string) bool {
	// Blank lines within a multiline message should be preserved
	if len(line) == 0 {
		return true
	}

	// Strip ANSI codes for pattern matching
	cleanLine := stripANSI(line)

	// Check for common stack trace patterns
	trimmed := strings.TrimLeft(cleanLine, " \t")

	// If line starts with whitespace and has content, likely continuation
	if len(trimmed) < len(cleanLine) {
		// Indented "at " is a stack frame
		if strings.HasPrefix(trimmed, "at ") {
			return true
		}
		// Indented "..." for suppressed frames
		if strings.HasPrefix(trimmed, "... ") {
			return true
		}
	}

	// "Caused by:" is a chained exception
	if strings.HasPrefix(trimmed, "Caused by:") {
		return true
	}

	// "Suppressed:" for suppressed exceptions
	if strings.HasPrefix(trimmed, "Suppressed:") {
		return true
	}

	// Exception class names (e.g., "java.lang.NullPointerException: message")
	if exceptionPattern.MatchString(trimmed) {
		return true
	}

	// If line doesn't start with a timestamp, it's likely a continuation
	// Check if it matches any of our timestamp patterns (full date)
	for _, pattern := range javaPatterns {
		if pattern.MatchString(cleanLine) {
			return false // This is a new log entry
		}
	}

	// Also check time-only patterns (like logback status)
	for _, pattern := range javaTimeOnlyPatterns {
		if pattern.MatchString(cleanLine) {
			return false // This is a new log entry
		}
	}

	// Doesn't look like a new log entry - join it
	return true
}
