package cmd

import (
	"context"
	"os"

	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/internal/source"

	"github.com/spf13/cobra"
)

var (
	streamsLimit int
)

var streamsCmd = &cobra.Command{
	Use:   "streams <source>",
	Short: "List log streams or files in a source",
	Long: `List log streams in a CloudWatch log group or files matching a pattern.

Examples:
  # List streams in a CloudWatch log group
  clew streams "cloudwatch:///app/logs?profile=prod"

  # List streams using a config alias
  clew streams @prod-api

  # List files matching a local pattern
  clew streams "file:///var/log/*.log"

  # Limit results
  clew streams @prod-api -l 50`,
	Args: cobra.ExactArgs(1),
	RunE: runStreams,
}

func init() {
	rootCmd.AddCommand(streamsCmd)

	streamsCmd.Flags().IntVarP(&streamsLimit, "limit", "l", 20, "Max streams to return")
}

func runStreams(cmd *cobra.Command, args []string) error {
	sourceURI := args[0]

	// Open the source
	src, err := source.Open(sourceURI)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	ctx := context.Background()
	render.Status("Listing streams from %s...", sourceURI)

	streams, err := src.ListStreams(ctx)
	if err != nil {
		return err
	}

	// Apply limit
	if len(streams) > streamsLimit {
		streams = streams[:streamsLimit]
	}

	// Format output
	formatter := output.NewFormatter(getOutputFormat(), os.Stdout)
	return formatter.FormatSourceStreams(streams)
}
