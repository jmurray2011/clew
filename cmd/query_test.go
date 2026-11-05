package cmd

import (
	"testing"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
)

func TestTruncatedBefore(t *testing.T) {
	start := time.Date(2026, 11, 4, 16, 30, 0, 0, time.UTC)
	entries := func(ts ...time.Time) []cloudwatch.Entry {
		out := make([]cloudwatch.Entry, len(ts))
		for i, v := range ts {
			out[i] = cloudwatch.Entry{Timestamp: v}
		}
		return out
	}

	tests := []struct {
		name       string
		results    []cloudwatch.Entry
		limit      int
		want       bool
		wantOldest time.Time
	}{
		{
			name:    "under limit is not truncated",
			results: entries(start.Add(60*time.Minute), start.Add(50*time.Minute)),
			limit:   500,
			want:    false,
		},
		{
			name:       "at limit with uncovered window is truncated",
			results:    entries(start.Add(90*time.Minute), start.Add(80*time.Minute)),
			limit:      2,
			want:       true,
			wantOldest: start.Add(80 * time.Minute),
		},
		{
			name:    "at limit but window covered is not truncated",
			results: entries(start.Add(90*time.Minute), start.Add(30*time.Second)),
			limit:   2,
			want:    false,
		},
		{
			name:    "zero oldest timestamp is not truncated",
			results: entries(start.Add(90*time.Minute), time.Time{}),
			limit:   2,
			want:    false,
		},
		{
			name:    "empty results are not truncated",
			results: nil,
			limit:   0,
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldest, got := truncatedBefore(tc.results, tc.limit, start)
			if got != tc.want {
				t.Fatalf("truncatedBefore() = %v, want %v", got, tc.want)
			}
			if tc.want && !oldest.Equal(tc.wantOldest) {
				t.Fatalf("oldest = %v, want %v", oldest, tc.wantOldest)
			}
		})
	}
}
