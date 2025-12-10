package cmd

import (
	"strings"
	"testing"
	"time"

	"github.com/jmurray2011/clew/pkg/timeutil"
	"github.com/spf13/cobra"
)

// Note: Tests for time parsing, byte formatting, and duration formatting
// have been moved to pkg/timeutil/parse_test.go. These tests verify the
// integration with cmd package.

func TestTimeutilParseIntegration(t *testing.T) {
	// Verify that timeutil.Parse works as expected for cmd package usage
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string", "", false},
		{"now keyword", "now", false},
		{"RFC3339", "2025-12-03T10:00:00Z", false},
		{"relative minutes", "30m", false},
		{"relative hours", "2h", false},
		{"relative days", "7d", false},
		{"invalid format", "invalid", true},
		{"unsupported unit", "5s", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := timeutil.Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("timeutil.Parse(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestTimeutilFormatBytesIntegration(t *testing.T) {
	// Verify that timeutil.FormatBytes works as expected for cmd package usage
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := timeutil.FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("timeutil.FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestTimeutilFormatDurationIntegration(t *testing.T) {
	// Verify that timeutil.FormatDuration works as expected for cmd package usage
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{time.Hour, "1.0h"},
		{24 * time.Hour, "1.0d"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := timeutil.FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("timeutil.FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestBuildConsoleURL(t *testing.T) {
	tests := []struct {
		name      string
		region    string
		logGroups []string
		start     time.Time
		end       time.Time
		query     string
		checks    []string // strings that should be in the URL
	}{
		{
			name:      "basic URL",
			region:    "us-east-1",
			logGroups: []string{"/app/logs"},
			start:     time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			end:       time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC),
			query:     "fields @message",
			checks: []string{
				"us-east-1.console.aws.amazon.com",
				"cloudwatch",
				"logs-insights",
			},
		},
		{
			name:      "multiple log groups",
			region:    "eu-west-1",
			logGroups: []string{"/app/api", "/app/web"},
			start:     time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
			end:       time.Date(2025, 12, 2, 0, 0, 0, 0, time.UTC),
			query:     "",
			checks: []string{
				"eu-west-1.console.aws.amazon.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildConsoleURL(tt.region, tt.logGroups, tt.start, tt.end, tt.query)
			for _, check := range tt.checks {
				if !strings.Contains(got, check) {
					t.Errorf("buildConsoleURL() = %q, should contain %q", got, check)
				}
			}
		})
	}
}

func TestGenerateDefaultConfig(t *testing.T) {
	config := generateDefaultConfig("/home/user")

	// Check essential parts of the config
	checks := []string{
		"output: text",
		"history_max:",
		"AWS settings",
		"aliases:",
	}

	for _, check := range checks {
		if !strings.Contains(config, check) {
			t.Errorf("generateDefaultConfig() should contain %q", check)
		}
	}
}

func TestExtractJSONFields(t *testing.T) {
	tests := []struct {
		name   string
		data   map[string]interface{}
		prefix string
		want   map[string]string
	}{
		{
			name:   "simple fields",
			data:   map[string]interface{}{"name": "test", "count": float64(42)},
			prefix: "",
			want:   map[string]string{"name": "test", "count": "42"},
		},
		{
			name:   "with prefix",
			data:   map[string]interface{}{"name": "test"},
			prefix: "data",
			want:   map[string]string{"data.name": "test"},
		},
		{
			name:   "nested object",
			data:   map[string]interface{}{"user": map[string]interface{}{"name": "John"}},
			prefix: "",
			want:   map[string]string{"user.name": "John"},
		},
		{
			name:   "boolean field",
			data:   map[string]interface{}{"active": true},
			prefix: "",
			want:   map[string]string{"active": "true"},
		},
		{
			name:   "nil field",
			data:   map[string]interface{}{"value": nil},
			prefix: "",
			want:   map[string]string{"value": "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONFields(tt.data, tt.prefix)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("extractJSONFields()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFormatSampleValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"string", "hello", "hello"},
		{"int", float64(42), "42"},
		{"float", 3.14, "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "null"},
		{"long string", strings.Repeat("a", 100), strings.Repeat("a", 100)}, // no truncation in formatSampleValue
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSampleValue(tt.value)
			if got != tt.want {
				t.Errorf("formatSampleValue(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestTruncateValue(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello wo..."},
		{"empty", "", 5, ""},
		{"newlines replaced", "hello\nworld", 20, "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateValue(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateValue(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"RFC3339", "2025-12-03T10:00:00Z", false},
		{"RFC3339 with offset", "2025-12-03T10:00:00-05:00", false},
		{"CloudWatch format", "2025-12-03 10:00:00.000", false},
		{"invalid", "not a timestamp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimestamp(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseTimestamp(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"5m", "5m", 5 * time.Minute, false},
		{"1h", "1h", time.Hour, false},
		{"15m", "15m", 15 * time.Minute, false},
		{"invalid", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeTypst(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escaping needed", "hello world", "hello world"},
		{"escape hash", "issue #123", `issue \#123`},
		{"escape at", "user@email.com", `user\@email.com`},
		{"escape dollar", "cost $50", `cost \$50`},
		{"escape underscore", "log_file", `log\_file`},
		{"multiple escapes", "#1 @user $100", `\#1 \@user \$100`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeTypst(tt.input)
			if got != tt.want {
				t.Errorf("escapeTypst(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWrapLongLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		check    func(string) bool
	}{
		{
			name:     "short line unchanged",
			input:    "hello",
			maxWidth: 80,
			check:    func(s string) bool { return s == "hello" },
		},
		{
			name:     "long line wrapped",
			input:    strings.Repeat("a", 100),
			maxWidth: 50,
			check: func(s string) bool {
				lines := strings.Split(s, "\n")
				for _, line := range lines {
					if len(line) > 50 {
						return false
					}
				}
				return len(lines) > 1
			},
		},
		{
			name:     "preserves existing newlines",
			input:    "line1\nline2",
			maxWidth: 80,
			check: func(s string) bool {
				return strings.Contains(s, "\n")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrapLongLines(tt.input, tt.maxWidth)
			if !tt.check(got) {
				t.Errorf("wrapLongLines(%q, %d) = %q, check failed", tt.input, tt.maxWidth, got)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},  // s[:8-3] + "..." = "hello..."
		{"longer truncated", "hello world test", 12, "hello wor..."}, // s[:12-3] + "..."
		{"empty", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestFormatLogGroups(t *testing.T) {
	tests := []struct {
		name   string
		groups []string
		want   string
	}{
		{"single group", []string{"/app/logs"}, "/app/logs"},
		{"multiple groups", []string{"/app/api", "/app/web"}, "/app/api (+1 more)"},
		{"three groups", []string{"/app/api", "/app/web", "/app/worker"}, "/app/api (+2 more)"},
		{"empty", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLogGroups(tt.groups)
			if got != tt.want {
				t.Errorf("formatLogGroups(%v) = %q, want %q", tt.groups, got, tt.want)
			}
		})
	}
}

// Tests for App struct - demonstrating testability of the cmd package

func TestNewAppWithConfig(t *testing.T) {
	cfg := Config{
		Profile:      "test-profile",
		Region:       "us-west-2",
		OutputFormat: "json",
		Verbose:      true,
	}

	app := NewAppWithConfig(cfg, nil, nil)

	if app.Config.Profile != "test-profile" {
		t.Errorf("expected profile 'test-profile', got %q", app.Config.Profile)
	}
	if app.Config.Region != "us-west-2" {
		t.Errorf("expected region 'us-west-2', got %q", app.Config.Region)
	}
	if app.Config.OutputFormat != "json" {
		t.Errorf("expected output format 'json', got %q", app.Config.OutputFormat)
	}
	if !app.Config.Verbose {
		t.Error("expected verbose to be true")
	}
}

func TestAppGetters(t *testing.T) {
	cfg := Config{
		Profile:      "my-profile",
		Region:       "eu-west-1",
		OutputFormat: "csv",
	}

	app := NewAppWithConfig(cfg, nil, nil)

	// Test getter methods
	if got := app.GetProfile(); got != "my-profile" {
		t.Errorf("GetProfile() = %q, want 'my-profile'", got)
	}
	if got := app.GetRegion(); got != "eu-west-1" {
		t.Errorf("GetRegion() = %q, want 'eu-west-1'", got)
	}
	if got := app.GetOutputFormat(); got != "csv" {
		t.Errorf("GetOutputFormat() = %q, want 'csv'", got)
	}
}

func TestAppAccountIDCache(t *testing.T) {
	app := NewAppWithConfig(Config{}, nil, nil)

	// Initially empty
	if len(app.AccountIDCache) != 0 {
		t.Error("expected empty account ID cache")
	}

	// Can store and retrieve
	app.AccountIDCache["test-profile"] = "123456789012"
	if got := app.AccountIDCache["test-profile"]; got != "123456789012" {
		t.Errorf("expected account ID '123456789012', got %q", got)
	}
}

func TestSetAndGetApp(t *testing.T) {
	cfg := Config{
		Profile: "context-test",
		Verbose: true,
	}
	app := NewAppWithConfig(cfg, nil, nil)

	// Create a context with the app
	ctx := SetApp(t.Context(), app)

	// Create a mock command with the context
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	// Retrieve the app
	retrieved := GetApp(cmd)
	if retrieved.Config.Profile != "context-test" {
		t.Errorf("expected profile 'context-test', got %q", retrieved.Config.Profile)
	}
	if !retrieved.Config.Verbose {
		t.Error("expected verbose to be true")
	}
}
