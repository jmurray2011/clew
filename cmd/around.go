package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/internal/source"

	"github.com/spf13/cobra"
)

var (
	aroundTimestamp string
	aroundWindow    string
	aroundLimit     int
)

var aroundCmd = &cobra.Command{
	Use:   "around <source>",
	Short: "Show logs around a specific timestamp",
	Long: `Query logs in a time window centered on a specific timestamp.

Useful when investigating an issue: find the error, then see what
happened before and after it.

Source URIs:
  cloudwatch:///log-group      AWS CloudWatch Logs (use -p for profile)
  file:///path/to/file.log     Local file
  /var/log/app.log             Local file (shorthand)
  @alias-name                  Config alias

Examples:
  # Show logs 5 minutes before/after a timestamp
  clew around "cloudwatch:///app/logs" -p prod -t "2025-12-04T10:30:00Z"

  # Specify a different window size
  clew around @prod-api -t "2025-12-04T10:30:00Z" --window 10m

  # Use with local files
  clew around /var/log/app.log -t "2025-12-04T10:30:00Z" --window 2m`,
	Args: cobra.ExactArgs(1),
	RunE: runAround,
}

func init() {
	rootCmd.AddCommand(aroundCmd)

	aroundCmd.Flags().StringVarP(&aroundTimestamp, "timestamp", "t", "", "Center timestamp - RFC3339 format (required)")
	aroundCmd.Flags().StringVar(&aroundWindow, "window", "5m", "Time window before/after timestamp (e.g., 2m, 5m, 10m)")
	aroundCmd.Flags().IntVarP(&aroundLimit, "limit", "l", 200, "Max results to return")

	_ = aroundCmd.MarkFlagRequired("timestamp")
}

func runAround(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	sourceURI := args[0]

	// Parse center timestamp
	centerTime, err := parseTimestamp(aroundTimestamp)
	if err != nil {
		return err
	}

	// Parse window duration
	windowDur, err := time.ParseDuration(aroundWindow)
	if err != nil {
		return fmt.Errorf("invalid window duration: %w", err)
	}

	// Calculate time range
	startTime := centerTime.Add(-windowDur)
	endTime := centerTime.Add(windowDur)

	// Open the source
	opts := source.OpenOptions{
		Profile: app.GetProfile(),
		Region:  app.GetRegion(),
	}
	src, err := source.OpenWithOptions(sourceURI, opts)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	app.Render.Status("Querying logs around %s (±%s)...", centerTime.Format("15:04:05"), aroundWindow)

	ctx := cmd.Context()
	app.Debugf("Source type: %s", src.Type())
	app.Debugf("Time range: %s to %s", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))

	// Build query params
	params := source.QueryParams{
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     aroundLimit,
	}

	results, err := src.Query(ctx, params)
	if err != nil {
		return err
	}

	// Cache pointers with metadata for evidence support
	cachePtrsFromEntries(ctx, results, src)

	// Format output
	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	if err := formatter.FormatEntries(results); err != nil {
		return err
	}

	// Show summary
	app.Render.Newline()
	app.Render.Info("Found %d log entries in ±%s window around %s",
		len(results), aroundWindow, centerTime.Format("15:04:05"))

	return nil
}

// parseTimestamp parses a timestamp string in various formats.
func parseTimestamp(input string) (time.Time, error) {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t, nil
	}

	// Try common formats
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000",
		"2006-01-02 15:04:05.000",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, input); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid timestamp format (use RFC3339: 2025-12-04T10:30:00Z)")
}

