package local

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmurray2011/clew/internal/logging"
	"github.com/jmurray2011/clew/internal/source"
)

// Default configuration values
const (
	// DefaultEventChanBuffer is the default buffer size for tail event channels
	DefaultEventChanBuffer = 100

	// MaxScanTokenSize is the maximum line size when scanning log files (1MB)
	MaxScanTokenSize = 1024 * 1024

	// FormatDetectionSampleLines is how many lines to sample when detecting log format
	FormatDetectionSampleLines = 10

	// LogRotationDelay is how long to wait for log rotation to complete before reopening
	LogRotationDelay = 100 * time.Millisecond
)

func init() {
	source.Register("file", openSource)
}

// Source implements source.Source for local filesystem logs.
type Source struct {
	pattern       string
	files         []string
	format        Format
	parser        Parser
	uri           string
	droppedEvents int64 // atomic counter for dropped events during tail
}

// openSource opens a local file source from a parsed URL.
func openSource(u *url.URL) (source.Source, error) {
	pattern := u.Path
	if pattern == "" {
		return nil, fmt.Errorf("file:// URI requires a path")
	}

	// Expand ~ to home directory
	if strings.HasPrefix(pattern, "/~/") {
		if home, err := os.UserHomeDir(); err == nil {
			pattern = filepath.Join(home, pattern[3:])
		}
	}

	formatHint := u.Query().Get("format")

	return NewSource(pattern, formatHint)
}

// NewSource creates a new local file source.
// The pattern can be a specific file path or a glob pattern.
// The formatHint specifies the log format (auto, plain, json, syslog, java).
func NewSource(pattern, formatHint string) (*Source, error) {
	// Expand glob pattern
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid file pattern %q: %w", pattern, err)
	}

	// If no glob match, check if it's a literal file that might not exist yet
	if len(files) == 0 {
		if _, err := os.Stat(pattern); err != nil {
			return nil, fmt.Errorf("no files match pattern %q", pattern)
		}
		files = []string{pattern}
	}

	// Sort files for consistent ordering
	sort.Strings(files)

	// Determine format
	format := parseFormat(formatHint)
	if format == FormatAuto && len(files) > 0 {
		format = DetectFormat(files[0])
	}

	return &Source{
		pattern: pattern,
		files:   files,
		format:  format,
		parser:  NewParser(format),
		uri:     pattern,
	}, nil
}

// NewSourceFromFiles creates a local file source from explicit file paths.
// This is used when the shell expands a glob before clew receives it.
func NewSourceFromFiles(files []string, formatHint string) (*Source, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no files provided")
	}

	// Verify files exist
	var validFiles []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			validFiles = append(validFiles, f)
		}
	}

	if len(validFiles) == 0 {
		return nil, fmt.Errorf("none of the specified files exist")
	}

	// Sort files for consistent ordering
	sort.Strings(validFiles)

	// Determine format
	format := parseFormat(formatHint)
	if format == FormatAuto && len(validFiles) > 0 {
		format = DetectFormat(validFiles[0])
	}

	// Use first file as the URI for display
	uri := validFiles[0]
	if len(validFiles) > 1 {
		uri = fmt.Sprintf("%s (+%d more)", validFiles[0], len(validFiles)-1)
	}

	return &Source{
		pattern: uri,
		files:   validFiles,
		format:  format,
		parser:  NewParser(format),
		uri:     uri,
	}, nil
}

// Query returns log entries matching the given parameters.
func (s *Source) Query(ctx context.Context, params source.QueryParams) ([]source.Entry, error) {
	var results []source.Entry

	for _, file := range s.files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		entries, err := s.queryFile(ctx, file, params)
		if err != nil {
			return nil, fmt.Errorf("error reading %s: %w", file, err)
		}
		results = append(results, entries...)
	}

	// Sort by timestamp (newest first for consistency with CloudWatch)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})

	// Apply limit
	if params.Limit > 0 && len(results) > params.Limit {
		results = results[:params.Limit]
	}

	// Fetch context lines if requested
	if params.Context > 0 {
		for i := range results {
			before, after, err := s.FetchContext(ctx, results[i], params.Context, params.Context)
			if err == nil {
				results[i].Context = source.EntryContext{
					Before: before,
					After:  after,
				}
			}
		}
	}

	return results, nil
}

// queryFile reads and filters a single file.
func (s *Source) queryFile(ctx context.Context, filepath string, params source.QueryParams) ([]source.Entry, error) {
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var results []source.Entry
	scanner := bufio.NewScanner(f)

	// Handle large lines
	buf := make([]byte, MaxScanTokenSize)
	scanner.Buffer(buf, MaxScanTokenSize)

	lineNum := 0
	var currentEntry *source.Entry

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		lineNum++
		line := scanner.Text()

		// Handle multiline entries
		if s.parser.IsMultiline() && currentEntry != nil && s.parser.ShouldJoin(line) {
			currentEntry.Message += "\n" + line
			continue
		}

		// If we had a multiline entry, finalize it
		if currentEntry != nil {
			if s.matchesParams(*currentEntry, params) {
				results = append(results, *currentEntry)
			}
			currentEntry = nil
		}

		// Parse the new line
		entry := s.parser.ParseLine(line, lineNum, filepath)
		if entry == nil {
			continue
		}

		// For multiline parsers, start accumulating
		if s.parser.IsMultiline() {
			currentEntry = entry
		} else {
			if s.matchesParams(*entry, params) {
				results = append(results, *entry)
			}
		}
	}

	// Don't forget the last entry for multiline
	if currentEntry != nil {
		if s.matchesParams(*currentEntry, params) {
			results = append(results, *currentEntry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// matchesParams checks if an entry matches the query parameters.
func (s *Source) matchesParams(entry source.Entry, params source.QueryParams) bool {
	// Time range filter - only apply if entry has a parsed timestamp
	// Plain text files may not have parseable timestamps, so we skip time filtering for those
	if !entry.Timestamp.IsZero() {
		if !params.StartTime.IsZero() && entry.Timestamp.Before(params.StartTime) {
			return false
		}
		if !params.EndTime.IsZero() && entry.Timestamp.After(params.EndTime) {
			return false
		}
	}

	// Regex filter
	if params.Filter != nil && !params.Filter.MatchString(entry.Message) {
		return false
	}

	return true
}

// Tail streams log events in real-time using fsnotify.
func (s *Source) Tail(ctx context.Context, params source.TailParams) (<-chan source.Event, error) {
	if len(s.files) == 0 {
		return nil, fmt.Errorf("no files to tail")
	}

	// For now, only support tailing a single file
	if len(s.files) > 1 {
		return nil, fmt.Errorf("tailing multiple files not yet supported; specify a single file")
	}

	filePath := s.files[0]

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Open file and seek to end
	f, err := os.Open(filePath)
	if err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Seek to end of file
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		_ = f.Close()
		_ = watcher.Close()
		return nil, fmt.Errorf("failed to seek to end: %w", err)
	}

	// Add file to watcher
	if err := watcher.Add(filePath); err != nil {
		_ = f.Close()
		_ = watcher.Close()
		return nil, fmt.Errorf("failed to watch file: %w", err)
	}

	events := make(chan source.Event, DefaultEventChanBuffer)

	go s.tailLoop(ctx, f, watcher, filePath, offset, params, events)

	return events, nil
}

// tailLoop handles the actual tailing logic in a goroutine.
func (s *Source) tailLoop(ctx context.Context, f *os.File, watcher *fsnotify.Watcher, filePath string, offset int64, params source.TailParams, events chan<- source.Event) {
	defer close(events)
	defer func() { _ = f.Close() }()
	defer func() { _ = watcher.Close() }()

	reader := bufio.NewReader(f)
	lineNum := 0

	// Count initial lines (for accurate line numbers)
	initialOffset := offset
	_, _ = f.Seek(0, io.SeekStart)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lineNum++
	}
	_, _ = f.Seek(initialOffset, io.SeekStart)
	reader.Reset(f)

	var currentEntry *source.Entry

	for {
		select {
		case <-ctx.Done():
			// Finalize any pending multiline entry
			if currentEntry != nil {
				s.emitEntry(currentEntry, params, events)
			}
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write == fsnotify.Write {
				// Read new content
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						if err == io.EOF {
							// Partial line - we can't unread it with bufio.Reader,
							// so we just discard and wait for more data
							break
						}
						return
					}

					line = strings.TrimRight(line, "\n\r")
					lineNum++

					// Handle multiline entries
					if s.parser.IsMultiline() && currentEntry != nil && s.parser.ShouldJoin(line) {
						currentEntry.Message += "\n" + line
						continue
					}

					// Emit previous entry if exists
					if currentEntry != nil {
						s.emitEntry(currentEntry, params, events)
						currentEntry = nil
					}

					// Parse new line
					entry := s.parser.ParseLine(line, lineNum, filePath)
					if entry == nil {
						continue
					}

					if s.parser.IsMultiline() {
						currentEntry = entry
					} else {
						s.emitEntry(entry, params, events)
					}
				}
			}

			// Handle file truncation (log rotation)
			if event.Op&fsnotify.Remove == fsnotify.Remove || event.Op&fsnotify.Rename == fsnotify.Rename {
				// File was removed/renamed (log rotation)
				// Try to reopen
				time.Sleep(LogRotationDelay)
				newFile, err := os.Open(filePath)
				if err == nil {
					_ = f.Close()
					f = newFile
					reader.Reset(f)
					lineNum = 0
					// Re-add to watcher
					_ = watcher.Remove(filePath)
					_ = watcher.Add(filePath)
				}
			}

		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			// Ignore watcher errors and continue tailing
			// (transient FS errors shouldn't crash the tail)
		}
	}
}

// emitEntry sends an entry to the events channel if it matches the filter.
func (s *Source) emitEntry(entry *source.Entry, params source.TailParams, events chan<- source.Event) {
	// Apply filter
	if params.Filter != nil && !params.Filter.MatchString(entry.Message) {
		return
	}

	event := source.Event{
		Timestamp: entry.Timestamp,
		Message:   entry.Message,
		Stream:    entry.Stream,
	}

	select {
	case events <- event:
	default:
		// Channel full, drop event and track it
		dropped := atomic.AddInt64(&s.droppedEvents, 1)
		// Log warning on first drop and every 100 drops thereafter
		if dropped == 1 || dropped%100 == 0 {
			logging.Warn("Event buffer full, dropped %d event(s) - consider increasing buffer size", dropped)
		}
	}
}

// GetRecord retrieves a single log entry by its pointer.
func (s *Source) GetRecord(ctx context.Context, ptr string) (*source.Entry, error) {
	info, ok := source.ParseLocalPtr(ptr)
	if !ok {
		return nil, fmt.Errorf("invalid local pointer: %s", ptr)
	}

	f, err := os.Open(info.FilePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if lineNum == info.LineNum {
			line := scanner.Text()
			entry := s.parser.ParseLine(line, lineNum, info.FilePath)
			if entry == nil {
				// Return a basic entry if parser returns nil
				return &source.Entry{
					Message: line,
					Stream:  filepath.Base(info.FilePath),
					Source:  info.FilePath,
					Ptr:     ptr,
				}, nil
			}
			return entry, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("line %d not found in %s", info.LineNum, info.FilePath)
}

// FetchContext retrieves context lines around a log entry.
func (s *Source) FetchContext(ctx context.Context, entry source.Entry, before, after int) ([]source.Event, []source.Event, error) {
	info, ok := source.ParseLocalPtr(entry.Ptr)
	if !ok {
		return nil, nil, fmt.Errorf("invalid local pointer: %s", entry.Ptr)
	}

	f, err := os.Open(info.FilePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	// Read all lines into memory for context retrieval
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	targetIdx := info.LineNum - 1 // Convert to 0-indexed
	if targetIdx < 0 || targetIdx >= len(lines) {
		return nil, nil, fmt.Errorf("line %d out of range", info.LineNum)
	}

	// Collect before context
	var beforeLines []source.Event
	startIdx := targetIdx - before
	if startIdx < 0 {
		startIdx = 0
	}
	for i := startIdx; i < targetIdx; i++ {
		beforeLines = append(beforeLines, source.Event{
			Message: lines[i],
			Stream:  filepath.Base(info.FilePath),
		})
	}

	// Collect after context
	var afterLines []source.Event
	endIdx := targetIdx + after + 1
	if endIdx > len(lines) {
		endIdx = len(lines)
	}
	for i := targetIdx + 1; i < endIdx; i++ {
		afterLines = append(afterLines, source.Event{
			Message: lines[i],
			Stream:  filepath.Base(info.FilePath),
		})
	}

	return beforeLines, afterLines, nil
}

// ListStreams returns available log files within this source.
func (s *Source) ListStreams(ctx context.Context) ([]source.StreamInfo, error) {
	var streams []source.StreamInfo

	for _, file := range s.files {
		info, err := os.Stat(file)
		if err != nil {
			continue // Skip files we can't stat
		}

		streams = append(streams, source.StreamInfo{
			Name:     file,
			Size:     info.Size(),
			LastTime: info.ModTime(),
		})
	}

	return streams, nil
}

// Type returns the source type identifier.
func (s *Source) Type() string {
	return "local"
}

// Metadata returns source metadata for caching and evidence collection.
func (s *Source) Metadata() source.SourceMetadata {
	return source.SourceMetadata{
		Type: "local",
		URI:  s.uri,
	}
}

// Close releases any resources held by the source.
func (s *Source) Close() error {
	return nil
}

// Files returns the list of files matched by this source's pattern.
func (s *Source) Files() []string {
	return s.files
}

// Format detection and helper functions

// Format represents a log file format.
type Format int

const (
	FormatAuto Format = iota
	FormatPlain
	FormatJSON
	FormatSyslog
	FormatJava
)

func (f Format) String() string {
	switch f {
	case FormatPlain:
		return "plain"
	case FormatJSON:
		return "json"
	case FormatSyslog:
		return "syslog"
	case FormatJava:
		return "java"
	default:
		return "auto"
	}
}

// parseFormat converts a format string to a Format constant.
func parseFormat(s string) Format {
	switch strings.ToLower(s) {
	case "json":
		return FormatJSON
	case "syslog":
		return FormatSyslog
	case "java":
		return FormatJava
	case "plain":
		return FormatPlain
	default:
		return FormatAuto
	}
}

// DetectFormat attempts to detect the log format by reading the first few lines.
func DetectFormat(filepath string) Format {
	f, err := os.Open(filepath)
	if err != nil {
		return FormatPlain
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	linesChecked := 0

	for scanner.Scan() && linesChecked < FormatDetectionSampleLines {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		linesChecked++

		// Check for JSON (starts with {)
		if strings.HasPrefix(line, "{") {
			return FormatJSON
		}

		// Check for Java log pattern (e.g., "2025-01-15 10:30:45,123 INFO")
		if isJavaLogLine(line) {
			return FormatJava
		}

		// Check for syslog pattern (e.g., "Jan 15 10:30:45 hostname")
		if isSyslogLine(line) {
			return FormatSyslog
		}
	}

	return FormatPlain
}

// isJavaLogLine checks if a line looks like a Java log entry.
func isJavaLogLine(line string) bool {
	// Common Java log patterns:
	// - "2025-01-15 10:30:45,123 INFO [class] message"
	// - "2025-01-15T10:30:45.123Z INFO message"
	// - "15:07:20,910 |-INFO in ..." (logback status)
	if len(line) < 15 {
		return false
	}

	// Check for logback status format: "HH:MM:SS,mmm |-LEVEL"
	if len(line) >= 20 && line[2] == ':' && line[5] == ':' &&
		(line[8] == ',' || line[8] == '.') && strings.Contains(line, "|-") {
		return true
	}

	// Check for date-like prefix
	hasDate := len(line) >= 10 &&
		((line[4] == '-' && line[7] == '-') || // YYYY-MM-DD
			(line[2] == '/' && line[5] == '/')) // MM/DD/YY

	if !hasDate {
		return false
	}

	// Check for log level keywords
	upper := strings.ToUpper(line)
	return strings.Contains(upper, " INFO ") ||
		strings.Contains(upper, " DEBUG ") ||
		strings.Contains(upper, " WARN ") ||
		strings.Contains(upper, " ERROR ") ||
		strings.Contains(upper, " TRACE ") ||
		strings.Contains(upper, " FATAL ")
}

// isSyslogLine checks if a line looks like a syslog entry.
func isSyslogLine(line string) bool {
	// RFC3164: "Jan 15 10:30:45 hostname program[pid]: message"
	// RFC5424: "<priority>1 2025-01-15T10:30:45.123Z hostname..."
	if len(line) < 15 {
		return false
	}

	// Check for RFC5424 priority prefix
	if strings.HasPrefix(line, "<") {
		idx := strings.Index(line, ">")
		if idx > 0 && idx < 5 {
			return true
		}
	}

	// Check for RFC3164 month prefix
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun",
		"Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	for _, month := range months {
		if strings.HasPrefix(line, month+" ") {
			return true
		}
	}

	return false
}
