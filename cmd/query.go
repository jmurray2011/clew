package cmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/pkg/timeutil"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

var (
	startTime    string
	endTime      string
	filter       string
	queryString  string
	queryStream  string
	limit        int
	contextLines int
	exportFile   string
	showStats    bool
)

var queryCmd = &cobra.Command{
	Use:               "query [log-group]",
	Aliases:           []string{"q"},
	Short:             "Search logs",
	ValidArgsFunction: completeLogGroups,
	Long: `Query CloudWatch Logs using Insights.

The log group argument can be:
  An exact name:      /app/api/prod
  A search pattern:   "docs.*error"
  Interactive picker:  ?
  Omitted:            shows your recent groups (frecency)

Examples:
  clew query -e test "docs.*error" -s 2h -f "error"
  clew query -e prod /app/api/prod -s 1h --stream i-abc123
  clew query -e test ? -s 30m -f "error|exception" --stats
  clew query -s 1h -f "error"  # frecency prompt`,
	Args: cobra.MaximumNArgs(1),
	RunE: runQuery,
}

func init() {
	rootCmd.AddCommand(queryCmd)

	queryCmd.Flags().StringVarP(&startTime, "since", "s", "1h", "Start time: relative (2h, 30m, 7d) or absolute (2025-01-15 14:30)")
	queryCmd.Flags().StringVarP(&endTime, "until", "u", "now", "End time (default: now)")
	queryCmd.Flags().StringVarP(&filter, "filter", "f", "", "Case-insensitive regex filter against log messages")
	queryCmd.Flags().StringVarP(&queryString, "query", "q", "", "Raw CloudWatch Logs Insights query (overrides -f)")
	queryCmd.Flags().StringVar(&queryStream, "stream", "", "Substring match on @logStream (e.g., i-abc123)")
	queryCmd.Flags().IntVarP(&limit, "limit", "l", 500, "Max results")
	queryCmd.Flags().IntVarP(&contextLines, "context", "C", 0, "Show N lines before each match")
	queryCmd.Flags().StringVar(&exportFile, "export", "", "Write results to file")
	queryCmd.Flags().BoolVar(&showStats, "stats", false, "Show match counts by 5-minute time bucket")
}

func runQuery(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	ctx := cmd.Context()

	sourceArg := ""
	if len(args) > 0 {
		sourceArg = args[0]
	}

	src, logGroup, err := app.OpenSource(ctx, sourceArg)
	if err != nil {
		return err
	}

	// Parse time range
	start, err := timeutil.Parse(startTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}
	end, err := timeutil.Parse(endTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	app.Debugf("Time range: %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	if !start.Before(end) {
		return fmt.Errorf("start time must be before end time")
	}

	for _, warning := range timeutil.ValidateTimeRange(start, end) {
		if warning.Level == "warning" {
			app.Render.Warning("%s", warning.Message)
		} else {
			app.Render.Info("%s", warning.Message)
		}
	}

	if err := maybeCostWarning(app, start, end); err != nil {
		return err
	}

	var filterRegex *regexp.Regexp
	if filter != "" {
		filterRegex, err = regexp.Compile("(?i)" + filter)
		if err != nil {
			return fmt.Errorf("invalid filter pattern: %w", err)
		}
	}

	params := cloudwatch.QueryInput{
		StartTime: start,
		EndTime:   end,
		Filter:    filterRegex,
		Query:     queryString,
		Stream:    queryStream,
		Limit:     limit,
		Context:   contextLines,
	}

	if showStats && queryString == "" {
		params.Query = cloudwatch.BuildStatsQuery(filter, queryStream, limit)
	}

	app.Render.Status("Querying %s...", logGroup)
	results, err := src.Query(ctx, params)
	if err != nil {
		return err
	}

	writer := os.Stdout
	if exportFile != "" {
		writer, err = os.Create(exportFile)
		if err != nil {
			return fmt.Errorf("failed to create export file: %w", err)
		}
		defer func() { _ = writer.Close() }()
	}

	formatter := output.NewFormatter(app.GetOutputFormat(), writer)
	if filter != "" && !showStats {
		formatter.WithHighlight(filter)
	}
	if err := formatter.FormatEntries(results); err != nil {
		return err
	}

	if exportFile != "" {
		app.Render.Success("Results exported to %s", exportFile)
	}

	return nil
}

// maybeCostWarning shows an interactive cost warning for wide time ranges.
func maybeCostWarning(app *App, start, end time.Time) error {
	threshold := 3 * 24 * time.Hour
	if t := viper.GetString("cost_warning_threshold"); t != "" {
		if d, err := timeutil.ParseDurationString(t); err == nil {
			threshold = d
		}
	}

	duration := end.Sub(start)
	if duration <= threshold {
		return nil
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) || app.Config.Quiet {
		return nil
	}

	app.Render.Warning("Querying %s of data — this may be slow/expensive for CloudWatch.", timeutil.FormatDuration(duration))
	fmt.Fprint(os.Stderr, "Continue? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "n" || answer == "no" {
		return fmt.Errorf("query cancelled")
	}

	return nil
}
