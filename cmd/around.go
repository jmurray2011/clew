package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/pkg/timeutil"

	"github.com/spf13/cobra"
)

var (
	aroundTimestamp string
	aroundWindow    string
	aroundLimit     int
	aroundFilter    string
	aroundStream    string
	aroundUTC       bool
	aroundContext   int
)

var aroundCmd = &cobra.Command{
	Use:               "around <log-group>",
	Short:             "Show logs around a timestamp",
	ValidArgsFunction: completeLogGroups,
	Long: `Query logs in a time window centered on a specific timestamp.

The timestamp accepts flexible formats:
  14:30                 Today, local timezone (or UTC with --utc)
  2025-01-15 14:30      Local timezone (or UTC with --utc)
  2025-01-15T14:30:00Z  Explicit UTC (always UTC regardless of --utc)

Use --utc when pasting timestamps from query output, which is always UTC.

Examples:
  clew around -e test "docs.*error" -t "14:31"
  clew around -e test "docs.*error" -t "2026-04-07 11:17:23" --utc
  clew around -e prod ? -t "2025-01-15 14:30" --window 10m`,
	Args: cobra.ExactArgs(1),
	RunE: runAround,
}

func init() {
	rootCmd.AddCommand(aroundCmd)

	aroundCmd.Flags().StringVarP(&aroundTimestamp, "timestamp", "t", "", "Center timestamp (required)")
	aroundCmd.Flags().StringVar(&aroundWindow, "window", "5m", "Time window before/after timestamp (e.g., 2m, 5m, 10m)")
	aroundCmd.Flags().IntVarP(&aroundLimit, "limit", "l", 200, "Max results")
	aroundCmd.Flags().StringVarP(&aroundFilter, "filter", "f", "", "Filter pattern for messages")
	aroundCmd.Flags().StringVar(&aroundStream, "stream", "", "Substring match on @logStream")
	aroundCmd.Flags().IntVarP(&aroundContext, "context", "C", 0, "Show N lines before each match")
	aroundCmd.Flags().BoolVar(&aroundUTC, "utc", false, "Interpret bare timestamps as UTC (use when pasting from query output)")

	_ = aroundCmd.MarkFlagRequired("timestamp")
}

func runAround(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()

	src, logGroup, err := app.OpenSource(ctx, args[0])
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}

	// Choose timezone for bare timestamp interpretation
	loc := time.Local
	if aroundUTC {
		loc = time.UTC
	}

	centerTime, err := timeutil.ParseTimestamp(aroundTimestamp, loc)
	if err != nil {
		return err
	}

	windowDur, err := time.ParseDuration(aroundWindow)
	if err != nil {
		return fmt.Errorf("invalid window duration: %w", err)
	}

	startTime := centerTime.Add(-windowDur)
	endTime := centerTime.Add(windowDur)

	app.Render.Status("Querying logs around %s (±%s)...", centerTime.Format("15:04:05"), aroundWindow)
	app.Debugf("Source: %s", logGroup)
	app.Debugf("Time range: %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	params := cloudwatch.QueryInput{
		StartTime: startTime,
		EndTime:   endTime,
		Stream:    aroundStream,
		Limit:     aroundLimit,
		Context:   aroundContext,
		SortAsc:   true,
	}

	if aroundFilter != "" {
		re, err := compileFilter(aroundFilter)
		if err != nil {
			return err
		}
		params.Filter = re
	}

	results, err := src.Query(ctx, params)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	if aroundFilter != "" {
		formatter.WithHighlight(aroundFilter)
	}
	if err := formatter.FormatEntries(results); err != nil {
		return err
	}

	app.Render.Newline()
	app.Render.Info("Found %d log entries in ±%s window around %s",
		len(results), aroundWindow, centerTime.Format("15:04:05"))

	// Warn if results were truncated before covering the full window
	if len(results) >= aroundLimit && len(results) > 0 {
		last := results[len(results)-1].Timestamp
		if last.Before(endTime.Add(-time.Minute)) {
			app.Render.Warning("Results hit the limit (%d) — only covers up to %s. Use -l to increase or -f to filter.",
				aroundLimit, last.Format("15:04:05"))
		}
	}

	return nil
}
