package timeutil

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, got time.Time)
	}{
		{
			name:  "empty string returns now",
			input: "",
			check: func(t *testing.T, got time.Time) {
				if time.Since(got) > time.Second {
					t.Error("expected time close to now")
				}
			},
		},
		{
			name:  "now returns current time",
			input: "now",
			check: func(t *testing.T, got time.Time) {
				if time.Since(got) > time.Second {
					t.Error("expected time close to now")
				}
			},
		},
		{
			name:  "RFC3339 format",
			input: "2025-01-15T10:30:00Z",
			check: func(t *testing.T, got time.Time) {
				expected := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
				if !got.Equal(expected) {
					t.Errorf("got %v, want %v", got, expected)
				}
			},
		},
		{
			name:  "relative minutes",
			input: "30m",
			check: func(t *testing.T, got time.Time) {
				diff := time.Since(got)
				if diff < 29*time.Minute || diff > 31*time.Minute {
					t.Errorf("expected ~30m ago, got diff of %v", diff)
				}
			},
		},
		{
			name:  "relative hours",
			input: "2h",
			check: func(t *testing.T, got time.Time) {
				diff := time.Since(got)
				if diff < 119*time.Minute || diff > 121*time.Minute {
					t.Errorf("expected ~2h ago, got diff of %v", diff)
				}
			},
		},
		{
			name:  "relative days",
			input: "7d",
			check: func(t *testing.T, got time.Time) {
				diff := time.Since(got)
				expectedDiff := 7 * 24 * time.Hour
				if diff < expectedDiff-time.Minute || diff > expectedDiff+time.Minute {
					t.Errorf("expected ~7d ago, got diff of %v", diff)
				}
			},
		},
		{
			name:    "invalid format",
			input:   "invalid",
			wantErr: true,
		},
		{
			name:    "invalid relative unit",
			input:   "5x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30m"},
		{90 * time.Minute, "1.5h"},
		{2 * time.Hour, "2.0h"},
		{24 * time.Hour, "1.0d"},
		{36 * time.Hour, "1.5d"},
	}

	for _, tt := range tests {
		t.Run(tt.d.String(), func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestValidateTimeRange(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		start        time.Time
		end          time.Time
		wantWarnings int
		checkMessage string // Optional substring to look for in warnings
	}{
		{
			name:         "valid range - last hour",
			start:        now.Add(-1 * time.Hour),
			end:          now,
			wantWarnings: 0,
		},
		{
			name:         "valid range - last day",
			start:        now.Add(-24 * time.Hour),
			end:          now,
			wantWarnings: 0,
		},
		{
			name:         "end time in future",
			start:        now.Add(-1 * time.Hour),
			end:          now.Add(24 * time.Hour),
			wantWarnings: 1,
			checkMessage: "in the future",
		},
		{
			name:         "start time in future",
			start:        now.Add(1 * time.Hour),
			end:          now.Add(2 * time.Hour),
			wantWarnings: 2, // Both start and end in future
			checkMessage: "no results will be returned",
		},
		{
			name:         "very large range",
			start:        now.Add(-60 * 24 * time.Hour),
			end:          now,
			wantWarnings: 1,
			checkMessage: "slow and expensive",
		},
		{
			name:         "very short range",
			start:        now.Add(-30 * time.Second),
			end:          now,
			wantWarnings: 1,
			checkMessage: "may miss relevant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := ValidateTimeRange(tt.start, tt.end)
			if len(warnings) != tt.wantWarnings {
				t.Errorf("ValidateTimeRange() returned %d warnings, want %d", len(warnings), tt.wantWarnings)
				for _, w := range warnings {
					t.Logf("  warning: %s", w.Message)
				}
			}
			if tt.checkMessage != "" && len(warnings) > 0 {
				found := false
				for _, w := range warnings {
					if contains(w.Message, tt.checkMessage) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning containing %q, got: %v", tt.checkMessage, warnings)
				}
			}
		})
	}
}

// contains checks if s contains substr (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
