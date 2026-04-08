package cmd

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jmurray2011/clew/internal/discovery"
	"github.com/spf13/cobra"
)

// completeLogGroups provides tab completion by live-querying DescribeLogGroups.
// Results are cached per profile+region for 5 minutes by the discovery package.
func completeLogGroups(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	app := GetApp(cmd)
	profile := app.GetProfile()
	region := app.GetRegion()

	if profile == "" {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	groups, err := discovery.ListGroups(ctx, profile, region)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var completions []string
	for _, g := range groups {
		if strings.HasPrefix(g, toComplete) {
			completions = append(completions, g)
		}
	}
	sort.Strings(completions)

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeOutputFormat provides completion for the -o/--output flag values.
func completeOutputFormat(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	formats := []string{"text", "json", "csv"}
	var matches []string
	for _, f := range formats {
		if strings.HasPrefix(f, toComplete) {
			matches = append(matches, f)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}

// compileFilter compiles a filter string into a regexp.
func compileFilter(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid filter pattern: %w", err)
	}
	return re, nil
}
