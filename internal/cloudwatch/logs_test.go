package cloudwatch

import (
	"strings"
	"testing"
	"time"
)

func TestConvertToFilterPattern(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		want   string
	}{
		{
			name:   "empty filter",
			filter: "",
			want:   "",
		},
		{
			name:   "simple filter",
			filter: "error",
			want:   "error",
		},
		{
			name:   "pipe-separated OR",
			filter: "error|exception",
			want:   `?"error" ?"exception"`,
		},
		{
			name:   "three terms",
			filter: "error|warn|critical",
			want:   `?"error" ?"warn" ?"critical"`,
		},
		{
			name:   "with extra whitespace",
			filter: " error | exception ",
			want:   `?"error" ?"exception"`,
		},
		{
			name:   "empty term after split",
			filter: "error||exception",
			want:   `?"error" ?"exception"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToFilterPattern(tt.filter)
			if got != tt.want {
				t.Errorf("convertToFilterPattern(%q) = %q, want %q", tt.filter, got, tt.want)
			}
		})
	}
}

func TestParseLogTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "CloudWatch Insights format",
			input:   "2025-12-03 19:13:20.000",
			wantErr: false,
		},
		{
			name:    "CloudWatch Insights without millis",
			input:   "2025-12-03 19:13:20",
			wantErr: false,
		},
		{
			name:    "RFC3339",
			input:   "2025-12-03T19:13:20Z",
			wantErr: false,
		},
		{
			name:    "RFC3339 with timezone",
			input:   "2025-12-03T19:13:20-05:00",
			wantErr: false,
		},
		{
			name:    "ISO with milliseconds",
			input:   "2025-12-03T19:13:20.123Z",
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "not a timestamp",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseLogTimestamp(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseLogTimestamp(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		check   func(t time.Time) bool
		wantErr bool
	}{
		{
			name:  "empty string returns now",
			input: "",
			check: func(t time.Time) bool {
				return time.Since(t) < time.Second
			},
			wantErr: false,
		},
		{
			name:  "now keyword",
			input: "now",
			check: func(t time.Time) bool {
				return time.Since(t) < time.Second
			},
			wantErr: false,
		},
		{
			name:  "RFC3339 format",
			input: "2025-12-03T19:13:20Z",
			check: func(t time.Time) bool {
				return t.Year() == 2025 && t.Month() == 12 && t.Day() == 3
			},
			wantErr: false,
		},
		{
			name:  "relative minutes",
			input: "30m",
			check: func(t time.Time) bool {
				diff := time.Since(t)
				return diff >= 29*time.Minute && diff <= 31*time.Minute
			},
			wantErr: false,
		},
		{
			name:  "relative hours",
			input: "2h",
			check: func(t time.Time) bool {
				diff := time.Since(t)
				return diff >= 119*time.Minute && diff <= 121*time.Minute
			},
			wantErr: false,
		},
		{
			name:  "relative days",
			input: "7d",
			check: func(t time.Time) bool {
				diff := time.Since(t)
				return diff >= 167*time.Hour && diff <= 169*time.Hour
			},
			wantErr: false,
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "unsupported unit",
			input:   "5s",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil && !tt.check(got) {
				t.Errorf("ParseTime(%q) = %v, check failed", tt.input, got)
			}
		})
	}
}

func TestBuildDefaultQuery(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		limit  int
		checks []string // strings that should be in the output
	}{
		{
			name:   "no filter",
			filter: "",
			limit:  100,
			checks: []string{"fields @timestamp", "sort @timestamp desc", "limit 100"},
		},
		{
			name:   "with filter",
			filter: "error",
			limit:  50,
			checks: []string{"filter @message like", "(?i)(error)", "limit 50"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDefaultQuery(tt.filter, tt.limit)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("BuildDefaultQuery(%q, %d) = %q, should contain %q", tt.filter, tt.limit, got, check)
				}
			}
		})
	}
}

func TestBuildStatsQuery(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		limit  int
		checks []string
	}{
		{
			name:   "no filter",
			filter: "",
			limit:  100,
			checks: []string{"stats count()", "bin(5m)", "limit 100"},
		},
		{
			name:   "with filter",
			filter: "error",
			limit:  50,
			checks: []string{"filter @message like", "(?i)(error)", "stats count()", "limit 50"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildStatsQuery(tt.filter, tt.limit)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("BuildStatsQuery(%q, %d) = %q, should contain %q", tt.filter, tt.limit, got, check)
				}
			}
		})
	}
}

func TestBuildInsightsQuery(t *testing.T) {
	tests := []struct {
		name   string
		filter string
		limit  int
		checks []string
	}{
		{
			name:   "no filter default limit",
			filter: "",
			limit:  0,
			checks: []string{"fields @timestamp", "@message", "@logStream", "@ptr", "limit 100"},
		},
		{
			name:   "with filter",
			filter: "error",
			limit:  50,
			checks: []string{"filter @message like", "(?i)(error)", "limit 50"},
		},
		{
			name:   "negative limit uses default",
			filter: "",
			limit:  -1,
			checks: []string{"limit 100"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildInsightsQuery(tt.filter, tt.limit)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("buildInsightsQuery(%q, %d) = %q, should contain %q", tt.filter, tt.limit, got, check)
				}
			}
		})
	}
}

func TestPreCompiledRegexes(t *testing.T) {
	// Ensure pre-compiled regexes work correctly
	t.Run("pipeRegex splits on pipe", func(t *testing.T) {
		parts := pipeRegex.Split("a|b|c", -1)
		if len(parts) != 3 {
			t.Errorf("expected 3 parts, got %d", len(parts))
		}
	})

	t.Run("whitespaceRegex replaces multiple spaces", func(t *testing.T) {
		got := whitespaceRegex.ReplaceAllString("hello   world", " ")
		if got != "hello world" {
			t.Errorf("expected 'hello world', got %q", got)
		}
	})

	t.Run("trimSpaceRegex trims leading/trailing space", func(t *testing.T) {
		got := trimSpaceRegex.ReplaceAllString("  hello  ", "")
		if got != "hello" {
			t.Errorf("expected 'hello', got %q", got)
		}
	})
}
