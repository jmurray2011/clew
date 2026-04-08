package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/pkg/timeutil"

	"github.com/spf13/cobra"
)

var (
	streamsLimit int
	streamsSince string
	streamsStale string
)

var streamsCmd = &cobra.Command{
	Use:               "streams <log-group>",
	Aliases:           []string{"ls"},
	Short:             "List log streams / active instances",
	ValidArgsFunction: completeLogGroups,
	Long: `List log streams in a CloudWatch log group.

Examples:
  clew streams -e prod /app/api/prod
  clew streams -e test "docs.*error" -s 10m
  clew streams -e test ? --stale 5m`,
	Args: cobra.ExactArgs(1),
	RunE: runStreams,
}

func init() {
	rootCmd.AddCommand(streamsCmd)

	streamsCmd.Flags().IntVarP(&streamsLimit, "limit", "l", 20, "Max streams to return")
	streamsCmd.Flags().StringVarP(&streamsSince, "since", "s", "", "Only show streams with events since (e.g., 1h, 10m)")
	streamsCmd.Flags().StringVar(&streamsStale, "stale", "", "Only show streams with NO events in this window (e.g., 5m)")
}

func runStreams(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()

	src, logGroup, err := app.OpenSource(ctx, args[0])
	if err != nil {
		return err
	}

	app.Render.Status("Listing streams from %s...", logGroup)

	streams, err := src.ListStreams(ctx)
	if err != nil {
		return err
	}

	if streamsSince != "" {
		sinceTime, err := timeutil.Parse(streamsSince)
		if err != nil {
			return err
		}
		var filtered = streams[:0]
		for _, s := range streams {
			if !s.LastEventTime.IsZero() && s.LastEventTime.After(sinceTime) {
				filtered = append(filtered, s)
			}
		}
		streams = filtered
	}

	if streamsStale != "" {
		staleDur, err := timeutil.ParseDurationString(streamsStale)
		if err != nil {
			return fmt.Errorf("invalid stale duration: %w", err)
		}
		cutoff := time.Now().Add(-staleDur)
		var filtered = streams[:0]
		for _, s := range streams {
			if !s.LastEventTime.IsZero() && s.LastEventTime.Before(cutoff) {
				filtered = append(filtered, s)
			}
		}
		streams = filtered
	}

	if len(streams) > streamsLimit {
		streams = streams[:streamsLimit]
	}

	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	return formatter.FormatStreams(streams)
}
