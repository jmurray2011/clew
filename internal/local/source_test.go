package local

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jmurray2011/clew/internal/source"
)

// Helper to create a temp file with content
func createTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	return path
}

func TestNewSource(t *testing.T) {
	// Create a temp directory with test files
	dir := t.TempDir()
	createTempFile(t, dir, "app.log", "line 1\nline 2\nline 3\n")
	createTempFile(t, dir, "other.log", "other content\n")

	t.Run("single file", func(t *testing.T) {
		src, err := NewSource(filepath.Join(dir, "app.log"), "")
		if err != nil {
			t.Fatalf("NewSource failed: %v", err)
		}
		if len(src.files) != 1 {
			t.Errorf("expected 1 file, got %d", len(src.files))
		}
	})

	t.Run("glob pattern", func(t *testing.T) {
		src, err := NewSource(filepath.Join(dir, "*.log"), "")
		if err != nil {
			t.Fatalf("NewSource failed: %v", err)
		}
		if len(src.files) != 2 {
			t.Errorf("expected 2 files, got %d", len(src.files))
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := NewSource(filepath.Join(dir, "nonexistent.log"), "")
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("format hint", func(t *testing.T) {
		src, err := NewSource(filepath.Join(dir, "app.log"), "json")
		if err != nil {
			t.Fatalf("NewSource failed: %v", err)
		}
		if src.format != FormatJSON {
			t.Errorf("expected JSON format, got %v", src.format)
		}
	})
}

func TestSource_Query_Plain(t *testing.T) {
	dir := t.TempDir()
	createTempFile(t, dir, "app.log", "line 1\nline 2 contains error\nline 3\n")

	src, err := NewSource(filepath.Join(dir, "app.log"), "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()

	t.Run("all entries", func(t *testing.T) {
		entries, err := src.Query(ctx, source.QueryParams{})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}
	})

	t.Run("with filter", func(t *testing.T) {
		filter := regexp.MustCompile("error")
		entries, err := src.Query(ctx, source.QueryParams{Filter: filter})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry matching filter, got %d", len(entries))
		}
		if !strings.Contains(entries[0].Message, "error") {
			t.Errorf("expected entry to contain 'error', got %q", entries[0].Message)
		}
	})

	t.Run("with limit", func(t *testing.T) {
		entries, err := src.Query(ctx, source.QueryParams{Limit: 2})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with limit, got %d", len(entries))
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err := src.Query(ctx, source.QueryParams{})
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

func TestSource_Query_JSON(t *testing.T) {
	dir := t.TempDir()
	jsonContent := `{"timestamp": "2025-01-15T10:30:00Z", "message": "first event", "level": "info"}
{"timestamp": "2025-01-15T10:31:00Z", "message": "second event", "level": "warn"}
{"timestamp": "2025-01-15T10:32:00Z", "message": "third event", "level": "error"}
`
	createTempFile(t, dir, "app.json", jsonContent)

	src, err := NewSource(filepath.Join(dir, "app.json"), "json")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()

	t.Run("parses timestamps", func(t *testing.T) {
		entries, err := src.Query(ctx, source.QueryParams{})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 3 {
			t.Fatalf("expected 3 entries, got %d", len(entries))
		}
		// Should be sorted newest first
		if entries[0].Message != "third event" {
			t.Errorf("expected newest first, got %q", entries[0].Message)
		}
		if entries[0].Timestamp.IsZero() {
			t.Error("expected non-zero timestamp")
		}
	})

	t.Run("time range filter", func(t *testing.T) {
		start, _ := time.Parse(time.RFC3339, "2025-01-15T10:30:30Z")
		end, _ := time.Parse(time.RFC3339, "2025-01-15T10:31:30Z")

		entries, err := src.Query(ctx, source.QueryParams{
			StartTime: start,
			EndTime:   end,
		})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 entry in time range, got %d", len(entries))
		}
		if entries[0].Message != "second event" {
			t.Errorf("expected 'second event', got %q", entries[0].Message)
		}
	})

	t.Run("extracts fields", func(t *testing.T) {
		entries, err := src.Query(ctx, source.QueryParams{Limit: 1})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if entries[0].Fields["level"] != "error" {
			t.Errorf("expected level=error, got %q", entries[0].Fields["level"])
		}
	})
}

func TestSource_GetRecord(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "app.log", "line 1\nline 2\nline 3\n")

	src, err := NewSource(path, "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()

	t.Run("valid pointer", func(t *testing.T) {
		ptr := source.MakeLocalPtr(path, 2)
		entry, err := src.GetRecord(ctx, ptr)
		if err != nil {
			t.Fatalf("GetRecord failed: %v", err)
		}
		if entry.Message != "line 2" {
			t.Errorf("expected 'line 2', got %q", entry.Message)
		}
	})

	t.Run("first line", func(t *testing.T) {
		ptr := source.MakeLocalPtr(path, 1)
		entry, err := src.GetRecord(ctx, ptr)
		if err != nil {
			t.Fatalf("GetRecord failed: %v", err)
		}
		if entry.Message != "line 1" {
			t.Errorf("expected 'line 1', got %q", entry.Message)
		}
	})

	t.Run("out of range line", func(t *testing.T) {
		ptr := source.MakeLocalPtr(path, 100)
		_, err := src.GetRecord(ctx, ptr)
		if err == nil {
			t.Error("expected error for out of range line")
		}
	})

	t.Run("invalid pointer", func(t *testing.T) {
		_, err := src.GetRecord(ctx, "not-a-valid-pointer")
		if err == nil {
			t.Error("expected error for invalid pointer")
		}
	})
}

func TestSource_FetchContext(t *testing.T) {
	dir := t.TempDir()
	content := "line 1\nline 2\nline 3\nline 4\nline 5\nline 6\nline 7\n"
	path := createTempFile(t, dir, "app.log", content)

	src, err := NewSource(path, "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()

	// Get the middle entry
	entries, _ := src.Query(ctx, source.QueryParams{})
	var middleEntry source.Entry
	for _, e := range entries {
		if e.Message == "line 4" {
			middleEntry = e
			break
		}
	}

	t.Run("fetch context", func(t *testing.T) {
		before, after, err := src.FetchContext(ctx, middleEntry, 2, 2)
		if err != nil {
			t.Fatalf("FetchContext failed: %v", err)
		}
		if len(before) != 2 {
			t.Errorf("expected 2 before lines, got %d", len(before))
		}
		if len(after) != 2 {
			t.Errorf("expected 2 after lines, got %d", len(after))
		}
		// Before should be in order (oldest first)
		if before[0].Message != "line 2" {
			t.Errorf("expected 'line 2' as first before, got %q", before[0].Message)
		}
		// After should be in order (oldest first)
		if after[0].Message != "line 5" {
			t.Errorf("expected 'line 5' as first after, got %q", after[0].Message)
		}
	})

	t.Run("context at start of file", func(t *testing.T) {
		firstEntry := source.Entry{
			Ptr: source.MakeLocalPtr(path, 1),
		}
		before, after, err := src.FetchContext(ctx, firstEntry, 2, 2)
		if err != nil {
			t.Fatalf("FetchContext failed: %v", err)
		}
		if len(before) != 0 {
			t.Errorf("expected 0 before lines at start, got %d", len(before))
		}
		if len(after) != 2 {
			t.Errorf("expected 2 after lines, got %d", len(after))
		}
	})

	t.Run("context at end of file", func(t *testing.T) {
		lastEntry := source.Entry{
			Ptr: source.MakeLocalPtr(path, 7),
		}
		before, after, err := src.FetchContext(ctx, lastEntry, 2, 2)
		if err != nil {
			t.Fatalf("FetchContext failed: %v", err)
		}
		if len(before) != 2 {
			t.Errorf("expected 2 before lines, got %d", len(before))
		}
		if len(after) != 0 {
			t.Errorf("expected 0 after lines at end, got %d", len(after))
		}
	})
}

func TestSource_ListStreams(t *testing.T) {
	dir := t.TempDir()
	createTempFile(t, dir, "app.log", "content\n")
	createTempFile(t, dir, "error.log", "error content\n")

	src, err := NewSource(filepath.Join(dir, "*.log"), "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()
	streams, err := src.ListStreams(ctx)
	if err != nil {
		t.Fatalf("ListStreams failed: %v", err)
	}

	if len(streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(streams))
	}

	// Check stream names (full paths)
	hasApp := false
	hasError := false
	for _, s := range streams {
		if strings.HasSuffix(s.Name, "app.log") {
			hasApp = true
		}
		if strings.HasSuffix(s.Name, "error.log") {
			hasError = true
		}
		// Each stream should have a size > 0
		if s.Size == 0 {
			t.Errorf("expected non-zero size for %s", s.Name)
		}
	}
	if !hasApp || !hasError {
		t.Errorf("expected streams ending with app.log and error.log, got %v", streams)
	}
}

func TestSource_TypeAndMetadata(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "app.log", "content\n")

	src, err := NewSource(path, "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	if src.Type() != "local" {
		t.Errorf("Type() = %q, want 'local'", src.Type())
	}

	meta := src.Metadata()
	if meta.Type != "local" {
		t.Errorf("Metadata.Type = %q, want 'local'", meta.Type)
	}
	if meta.URI != path {
		t.Errorf("Metadata.URI = %q, want %q", meta.URI, path)
	}
}

func TestSource_Close(t *testing.T) {
	dir := t.TempDir()
	path := createTempFile(t, dir, "app.log", "content\n")

	src, err := NewSource(path, "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	// Close should be a no-op but shouldn't error
	if err := src.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestSource_Files(t *testing.T) {
	dir := t.TempDir()
	createTempFile(t, dir, "a.log", "a\n")
	createTempFile(t, dir, "b.log", "b\n")
	createTempFile(t, dir, "c.log", "c\n")

	src, err := NewSource(filepath.Join(dir, "*.log"), "plain")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	files := src.Files()
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}

	// Files should be sorted
	for i := 1; i < len(files); i++ {
		if files[i] < files[i-1] {
			t.Errorf("files not sorted: %v", files)
			break
		}
	}
}

func TestDetectFormat(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name     string
		filename string
		content  string
		want     Format
	}{
		{
			name:     "json file",
			filename: "app.json",
			content:  `{"message": "test"}` + "\n",
			want:     FormatJSON,
		},
		{
			name:     "json content",
			filename: "app.log",
			content:  `{"timestamp": "2025-01-15T10:00:00Z", "message": "test"}` + "\n",
			want:     FormatJSON,
		},
		{
			name:     "java log",
			filename: "app.log",
			content:  "2025-01-15 10:30:45,123 INFO [main] com.example.App - Started\n",
			want:     FormatJava,
		},
		{
			name:     "syslog",
			filename: "syslog",
			content:  "Jan 15 10:30:45 myhost sshd[1234]: Connection from 10.0.0.1\n",
			want:     FormatSyslog,
		},
		{
			name:     "plain text",
			filename: "app.log",
			content:  "Some random log line\nAnother line\n",
			want:     FormatPlain,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := createTempFile(t, dir, tt.filename, tt.content)
			got := DetectFormat(path)
			if got != tt.want {
				t.Errorf("DetectFormat(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		hint string
		want Format
	}{
		{"", FormatAuto},
		{"auto", FormatAuto},
		{"plain", FormatPlain},
		{"json", FormatJSON},
		{"syslog", FormatSyslog},
		{"java", FormatJava},
		{"JAVA", FormatJava},   // case insensitive
		{"unknown", FormatAuto}, // unknown defaults to auto
	}

	for _, tt := range tests {
		t.Run(tt.hint, func(t *testing.T) {
			got := parseFormat(tt.hint)
			if got != tt.want {
				t.Errorf("parseFormat(%q) = %v, want %v", tt.hint, got, tt.want)
			}
		})
	}
}

func TestSource_MultilineJava(t *testing.T) {
	dir := t.TempDir()
	content := `2025-01-15 10:30:45,123 INFO [main] com.example.App - Starting
2025-01-15 10:30:46,000 ERROR [main] com.example.App - Error occurred
java.lang.NullPointerException: value was null
	at com.example.App.process(App.java:42)
	at com.example.App.main(App.java:10)
Caused by: java.io.IOException: File not found
	at com.example.IO.read(IO.java:100)
	... 5 more
2025-01-15 10:30:47,000 INFO [main] com.example.App - Recovered
`
	path := createTempFile(t, dir, "app.log", content)

	src, err := NewSource(path, "java")
	if err != nil {
		t.Fatalf("NewSource failed: %v", err)
	}

	ctx := context.Background()
	entries, err := src.Query(ctx, source.QueryParams{})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	// Should have 3 entries (multiline exception collapsed into one)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries (multiline collapsed), got %d", len(entries))
		for i, e := range entries {
			t.Logf("Entry %d: %q", i, e.Message[:min(50, len(e.Message))])
		}
	}

	// The error entry should contain the full stack trace
	var errorEntry *source.Entry
	for _, e := range entries {
		if strings.Contains(e.Message, "Error occurred") {
			errorEntry = &e
			break
		}
	}
	if errorEntry == nil {
		t.Fatal("could not find error entry")
	}
	if !strings.Contains(errorEntry.Message, "NullPointerException") {
		t.Error("expected error entry to contain stack trace")
	}
	if !strings.Contains(errorEntry.Message, "Caused by:") {
		t.Error("expected error entry to contain chained exception")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestNewSourceFromFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := createTempFile(t, dir, "a.log", "line from a\n")
	file2 := createTempFile(t, dir, "b.log", "line from b\n")
	file3 := createTempFile(t, dir, "c.log", "line from c\n")

	t.Run("multiple files", func(t *testing.T) {
		src, err := NewSourceFromFiles([]string{file1, file2, file3}, "")
		if err != nil {
			t.Fatalf("NewSourceFromFiles failed: %v", err)
		}
		if len(src.files) != 3 {
			t.Errorf("expected 3 files, got %d", len(src.files))
		}

		// Query should return entries from all files
		ctx := context.Background()
		entries, err := src.Query(ctx, source.QueryParams{})
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 entries, got %d", len(entries))
		}
	})

	t.Run("empty file list", func(t *testing.T) {
		_, err := NewSourceFromFiles([]string{}, "")
		if err == nil {
			t.Error("expected error for empty file list")
		}
	})

	t.Run("nonexistent files filtered out", func(t *testing.T) {
		src, err := NewSourceFromFiles([]string{file1, "/nonexistent/file.log", file2}, "")
		if err != nil {
			t.Fatalf("NewSourceFromFiles failed: %v", err)
		}
		if len(src.files) != 2 {
			t.Errorf("expected 2 valid files, got %d", len(src.files))
		}
	})

	t.Run("all nonexistent", func(t *testing.T) {
		_, err := NewSourceFromFiles([]string{"/nonexistent/a.log", "/nonexistent/b.log"}, "")
		if err == nil {
			t.Error("expected error when all files nonexistent")
		}
	})

	t.Run("uri shows count", func(t *testing.T) {
		src, err := NewSourceFromFiles([]string{file1, file2, file3}, "")
		if err != nil {
			t.Fatalf("NewSourceFromFiles failed: %v", err)
		}
		meta := src.Metadata()
		if !strings.Contains(meta.URI, "+2 more") {
			t.Errorf("expected URI to show '+2 more', got %q", meta.URI)
		}
	})
}
