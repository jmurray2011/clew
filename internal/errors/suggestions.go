// Package errors provides enhanced error messages with suggestions.
package errors

import (
	"fmt"
	"sort"
	"strings"
)

// SuggestiveError is an error that includes suggestions for fixing the problem.
type SuggestiveError struct {
	Message     string
	Suggestions []string
	HelpCommand string
}

func (e *SuggestiveError) Error() string {
	var b strings.Builder
	b.WriteString(e.Message)

	if len(e.Suggestions) > 0 {
		b.WriteString("\n\nDid you mean one of these?\n")
		for _, s := range e.Suggestions {
			b.WriteString("  ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}

	if e.HelpCommand != "" {
		b.WriteString("\nRun '")
		b.WriteString(e.HelpCommand)
		b.WriteString("' for more information.")
	}

	return b.String()
}

// SourceNotFoundError creates an error for when a source alias isn't found.
func SourceNotFoundError(alias string, available []string) error {
	similar := findSimilar(alias, available, 3)
	return &SuggestiveError{
		Message:     fmt.Sprintf("source %q not found", alias),
		Suggestions: similar,
		HelpCommand: "clew sources",
	}
}

// NoCaseError creates an error for when no case is active.
func NoCaseError() error {
	return &SuggestiveError{
		Message: "no active case",
		Suggestions: []string{
			"clew case new \"Description\"  - Start a new investigation",
			"clew case list                - See existing cases",
			"clew case open <id>           - Open an existing case",
		},
	}
}

// InvalidTimeError creates an error for invalid time format.
func InvalidTimeError(input string) error {
	return &SuggestiveError{
		Message: fmt.Sprintf("invalid time format %q", input),
		Suggestions: []string{
			"Relative: 1h, 30m, 2d, 1w (hours, minutes, days, weeks ago)",
			"Absolute: 2024-01-15T10:30:00Z (RFC3339)",
			"Date only: 2024-01-15 (midnight UTC)",
		},
	}
}

// MissingFlagError creates an error for a missing required flag.
func MissingFlagError(flag, description string, examples []string) error {
	return &SuggestiveError{
		Message:     fmt.Sprintf("%s is required", flag),
		Suggestions: examples,
	}
}

// findSimilar finds strings similar to target using Levenshtein distance.
func findSimilar(target string, candidates []string, maxDistance int) []string {
	type match struct {
		value    string
		distance int
	}

	var matches []match
	targetLower := strings.ToLower(target)

	for _, c := range candidates {
		cLower := strings.ToLower(c)
		d := levenshtein(targetLower, cLower)
		if d <= maxDistance {
			matches = append(matches, match{value: c, distance: d})
		}
	}

	// Sort by distance (closest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].distance < matches[j].distance
	})

	// Return top 3
	var result []string
	for i := 0; i < len(matches) && i < 3; i++ {
		result = append(result, matches[i].value)
	}

	return result
}

// levenshtein calculates the Levenshtein distance between two strings.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
	}

	// Initialize first column
	for i := 0; i <= len(a); i++ {
		matrix[i][0] = i
	}

	// Initialize first row
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min(
				matrix[i-1][j]+1,      // deletion
				matrix[i][j-1]+1,      // insertion
				matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[len(a)][len(b)]
}

func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
