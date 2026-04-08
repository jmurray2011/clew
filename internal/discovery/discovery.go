// Package discovery provides live log group search and interactive selection.
package discovery

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"golang.org/x/term"
)

// Cache holds DescribeLogGroups results per profile+region, with a TTL.
var (
	cache    = make(map[string]cacheEntry)
	cacheMu  sync.Mutex
	cacheTTL = 5 * time.Minute
)

type cacheEntry struct {
	groups []string
	time   time.Time
}

// ListGroups returns all log group names for a profile+region, using a 5-minute cache.
func ListGroups(ctx context.Context, profile, region string) ([]string, error) {
	key := profile + "|" + region

	cacheMu.Lock()
	if entry, ok := cache[key]; ok && time.Since(entry.time) < cacheTTL {
		cacheMu.Unlock()
		return entry.groups, nil
	}
	cacheMu.Unlock()

	rawClient, err := cloudwatch.NewLogsClient(profile, region)
	if err != nil {
		return nil, err
	}

	client := cloudwatch.NewClient(rawClient)
	infos, err := client.ListLogGroups(ctx, "", 500)
	if err != nil {
		return nil, err
	}

	names := make([]string, len(infos))
	for i, g := range infos {
		names[i] = g.Name
	}

	cacheMu.Lock()
	cache[key] = cacheEntry{groups: names, time: time.Now()}
	cacheMu.Unlock()

	return names, nil
}

// SearchGroups returns log groups matching a pattern.
// Supports both glob-style patterns (*syslog, /aws/lambda/*) and regex (docs.*error).
func SearchGroups(ctx context.Context, profile, region, pattern string) ([]string, error) {
	re, err := regexp.Compile("(?i)" + globToRegex(pattern))
	if err != nil {
		return nil, fmt.Errorf("invalid search pattern: %w", err)
	}

	all, err := ListGroups(ctx, profile, region)
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, name := range all {
		if re.MatchString(name) {
			matches = append(matches, name)
		}
	}

	return matches, nil
}

// IsExactGroup checks if a name matches an existing log group exactly.
func IsExactGroup(ctx context.Context, profile, region, name string) (bool, error) {
	all, err := ListGroups(ctx, profile, region)
	if err != nil {
		return false, err
	}

	for _, g := range all {
		if g == name {
			return true, nil
		}
	}
	return false, nil
}

// PickGroup shows a numbered list of log groups and prompts the user to select one.
// Returns the selected log group name or an error.
// groups is the list to pick from; label is a description shown in the prompt.
func PickGroup(groups []string, label string) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		if len(groups) == 1 {
			return groups[0], nil
		}
		// Non-interactive: list the matches so the user knows what to refine
		var b strings.Builder
		b.WriteString("multiple log groups matched (non-interactive, cannot prompt):\n")
		show := groups
		if len(show) > 10 {
			show = show[:10]
		}
		for _, g := range show {
			b.WriteString("  " + g + "\n")
		}
		if len(groups) > 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(groups)-10))
		}
		b.WriteString("Specify an exact log group name or narrow your search pattern.")
		return "", fmt.Errorf("%s", b.String())
	}

	if len(groups) == 0 {
		return "", fmt.Errorf("no log groups found")
	}

	// Show max 20 results
	display := groups
	if len(display) > 20 {
		display = display[:20]
	}

	fmt.Fprintf(os.Stderr, "\n%s:\n", label)
	for i, g := range display {
		fmt.Fprintf(os.Stderr, "  %2d. %s\n", i+1, g)
	}
	if len(groups) > 20 {
		fmt.Fprintf(os.Stderr, "  ... and %d more (refine your search)\n", len(groups)-20)
	}

	fmt.Fprint(os.Stderr, "\nSelect [1-")
	fmt.Fprintf(os.Stderr, "%d] or type to search: ", len(display))

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)

	// Try as a number
	if n, err := strconv.Atoi(answer); err == nil {
		if n >= 1 && n <= len(display) {
			return display[n-1], nil
		}
		return "", fmt.Errorf("selection %d out of range", n)
	}

	// Treat as a search pattern — filter and re-prompt
	if answer != "" {
		re, err := regexp.Compile("(?i)" + answer)
		if err != nil {
			return "", fmt.Errorf("invalid pattern: %w", err)
		}
		var filtered []string
		for _, g := range groups {
			if re.MatchString(g) {
				filtered = append(filtered, g)
			}
		}
		if len(filtered) == 0 {
			return "", fmt.Errorf("no log groups matching %q", answer)
		}
		if len(filtered) == 1 {
			return filtered[0], nil
		}
		return PickGroup(filtered, fmt.Sprintf("Matches for %q", answer))
	}

	return "", fmt.Errorf("no selection made")
}

// globToRegex converts a glob-style pattern to a regex.
// Handles * → .*, ? → ., and escapes other regex metacharacters.
// If the pattern already looks like valid regex (contains . followed by * or +),
// it's returned as-is.
func globToRegex(pattern string) string {
	// If it contains regex-specific sequences like .*, .+, [, (, leave it alone
	for _, sig := range []string{".*", ".+", "[", "(", "\\d", "\\w"} {
		if strings.Contains(pattern, sig) {
			return pattern
		}
	}

	// Check if it has glob characters
	hasGlob := strings.ContainsAny(pattern, "*?")
	if !hasGlob {
		return pattern
	}

	// Convert glob to regex
	var b strings.Builder
	for _, c := range pattern {
		switch c {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '.', '+', '^', '$', '{', '}', '|', '\\':
			b.WriteByte('\\')
			b.WriteRune(c)
		default:
			b.WriteRune(c)
		}
	}
	return b.String()
}
