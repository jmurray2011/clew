package errors

import (
	"strings"
	"testing"
)

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"", "", 0},
		{"a", "", 1},
		{"", "a", 1},
		{"abc", "abc", 0},
		{"abc", "ab", 1},
		{"abc", "abd", 1},
		{"kitten", "sitting", 3},
		{"prod-api", "prod-apis", 1},
		{"prod", "prod-api", 4},
	}

	for _, tc := range tests {
		got := levenshtein(tc.a, tc.b)
		if got != tc.expected {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.expected)
		}
	}
}

func TestFindSimilar(t *testing.T) {
	candidates := []string{"prod-api", "prod-web", "staging-api", "dev-api"}

	tests := []struct {
		target      string
		maxDistance int
		wantAny     []string
	}{
		{"prod-apis", 2, []string{"prod-api"}},
		{"prod", 5, []string{"prod-api", "prod-web"}},
		{"api", 5, []string{"prod-api", "dev-api"}},
	}

	for _, tc := range tests {
		got := findSimilar(tc.target, candidates, tc.maxDistance)
		for _, want := range tc.wantAny {
			found := false
			for _, g := range got {
				if g == want {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("findSimilar(%q, maxDist=%d) = %v, expected to contain %q",
					tc.target, tc.maxDistance, got, want)
			}
		}
	}
}

func TestSourceNotFoundError(t *testing.T) {
	available := []string{"prod-api", "prod-web", "staging"}
	err := SourceNotFoundError("prod-apis", available)

	errStr := err.Error()
	if !strings.Contains(errStr, "prod-apis") {
		t.Errorf("error should contain the bad alias: %s", errStr)
	}
	if !strings.Contains(errStr, "prod-api") {
		t.Errorf("error should suggest similar alias: %s", errStr)
	}
}

func TestInvalidTimeError(t *testing.T) {
	err := InvalidTimeError("yesterday")
	errStr := err.Error()

	if !strings.Contains(errStr, "yesterday") {
		t.Errorf("error should contain the bad input: %s", errStr)
	}
	if !strings.Contains(errStr, "Relative") {
		t.Errorf("error should mention relative format: %s", errStr)
	}
}
