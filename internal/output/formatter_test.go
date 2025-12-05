package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
)

func TestNewFormatter(t *testing.T) {
	var buf bytes.Buffer

	tests := []struct {
		format string
		want   Format
	}{
		{"text", FormatText},
		{"json", FormatJSON},
		{"csv", FormatCSV},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			f := NewFormatter(tt.format, &buf)
			if f.format != tt.want {
				t.Errorf("NewFormatter(%q).format = %v, want %v", tt.format, f.format, tt.want)
			}
		})
	}
}

func TestFormatLogResults_Empty(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("text", &buf)

	err := f.FormatLogResults([]cloudwatch.LogResult{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "No results found") {
		t.Errorf("expected 'No results found' message, got: %s", buf.String())
	}
}

func TestFormatLogResults_JSON(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("json", &buf)

	results := []cloudwatch.LogResult{
		{
			Timestamp: "2025-01-15T10:30:00Z",
			LogStream: "test-stream",
			Message:   "test message",
			Fields: map[string]string{
				"@timestamp": "2025-01-15T10:30:00Z",
				"@message":   "test message",
			},
		},
	}

	err := f.FormatLogResults(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it's valid JSON
	var parsed []map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, buf.String())
	}

	if len(parsed) != 1 {
		t.Errorf("expected 1 result, got %d", len(parsed))
	}
}

func TestFormatLogResults_CSV(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("csv", &buf)

	results := []cloudwatch.LogResult{
		{
			Timestamp: "2025-01-15T10:30:00Z",
			LogStream: "test-stream",
			Message:   "test message",
			Fields: map[string]string{
				"@timestamp": "2025-01-15T10:30:00Z",
				"@message":   "test message",
				"@logStream": "test-stream",
			},
		},
	}

	err := f.FormatLogResults(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// CSV should have header row and data row
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Errorf("expected at least 2 lines (header + data), got %d", len(lines))
	}
}

func TestFormatLogResults_Text(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("text", &buf)

	results := []cloudwatch.LogResult{
		{
			Timestamp: "2025-01-15T10:30:00Z",
			LogStream: "test-stream",
			Message:   "test error message",
			Fields: map[string]string{
				"@timestamp": "2025-01-15T10:30:00Z",
				"@message":   "test error message",
				"@logStream": "test-stream",
			},
		},
	}

	err := f.FormatLogResults(results)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Text format should contain the timestamp and message
	if !strings.Contains(output, "test-stream") {
		t.Errorf("expected output to contain stream name, got: %s", output)
	}
	if !strings.Contains(output, "test error message") {
		t.Errorf("expected output to contain message, got: %s", output)
	}
}

func TestFormatLogGroups(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("text", &buf)

	groups := []cloudwatch.LogGroupInfo{
		{
			Name:          "/app/logs",
			RetentionDays: 30,
			StoredBytes:   1024 * 1024,
			CreationTime:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	err := f.FormatLogGroups(groups)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "/app/logs") {
		t.Errorf("expected output to contain log group name, got: %s", output)
	}
}

func TestFormatStreams(t *testing.T) {
	var buf bytes.Buffer
	f := NewFormatter("text", &buf)

	streams := []cloudwatch.StreamInfo{
		{
			Name:           "stream-1",
			LastEventTime:  time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
			FirstEventTime: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			StoredBytes:    1024,
		},
	}

	err := f.FormatStreams(streams)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "stream-1") {
		t.Errorf("expected output to contain stream name, got: %s", output)
	}
}
