package output

import (
	"bytes"
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
