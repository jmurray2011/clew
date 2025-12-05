package cmd

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/jmurray2011/clew/internal/cases"
	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/internal/source"
	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
)

var (
	startTime     string
	endTime       string
	filter        string
	queryString   string
	limit         int
	contextLines  int
	exportFile    string
	showStats     bool
	dryRun        bool
	showURL       bool
	watchInterval int
	markQuery     bool
	noCapture     bool
	logFormat     string
)

var queryCmd = &cobra.Command{
	Use:   "query <source>",
	Short: "Query logs from a source",
	Long: `Query logs from CloudWatch, local files, or other sources.

Source URIs:
  cloudwatch:///log-group?profile=x&region=y   AWS CloudWatch Logs
  file:///path/to/file.log                     Local file
  /var/log/app.log                             Local file (shorthand)
  @alias-name                                  Config alias

Supports both RFC3339 timestamps and relative time formats:
  - RFC3339: 2025-12-02T06:00:00Z
  - Relative: 2h (2 hours ago), 30m (30 minutes ago), 7d (7 days ago)

Examples:
  # CloudWatch Logs
  clew query "cloudwatch:///app/logs" -s 2h -f "error"
  clew query "cloudwatch:///app/logs?profile=prod" -s 1h -f "exception"

  # Local files
  clew query /var/log/app.log -f "error"
  clew query "file:///var/log/*.log" -s 1h -f "timeout"

  # Config alias
  clew query @prod-api -s 1h -f "error"

  # Show context lines
  clew query @prod-api -s 2h -f "exception" -B 10

  # Export results
  clew query @prod-api -s 1d -f "error" --export errors.json -o json`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

func init() {
	rootCmd.AddCommand(queryCmd)

	queryCmd.Flags().StringVarP(&startTime, "since", "s", "1h", "Start time - RFC3339 or relative (e.g., 2h, 30m, 7d)")
	queryCmd.Flags().StringVarP(&endTime, "end", "e", "now", "End time - RFC3339 or relative")
	queryCmd.Flags().StringVarP(&filter, "filter", "f", "", "Regex filter for messages")
	queryCmd.Flags().StringVarP(&queryString, "query", "q", "", "Full query (CloudWatch Insights syntax for cloudwatch sources)")
	queryCmd.Flags().IntVarP(&limit, "limit", "l", 500, "Max results to return")
	queryCmd.Flags().IntVarP(&contextLines, "context", "C", 0, "Show N lines of context before each match")
	queryCmd.Flags().StringVar(&exportFile, "export", "", "Export results to file")
	queryCmd.Flags().BoolVar(&showStats, "stats", false, "Show match count by time bucket instead of results")
	queryCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Estimate query cost without running (CloudWatch only)")
	queryCmd.Flags().BoolVar(&showURL, "url", false, "Show AWS Console URL for this query (CloudWatch only)")
	queryCmd.Flags().IntVar(&watchInterval, "watch", 0, "Re-run query every N seconds (0 = disabled)")
	queryCmd.Flags().BoolVar(&markQuery, "mark", false, "Mark this query as significant in the active case")
	queryCmd.Flags().BoolVar(&noCapture, "no-capture", false, "Don't add this query to the active case timeline")
	queryCmd.Flags().StringVar(&logFormat, "format", "auto", "Log format hint for local files: auto, plain, json, syslog, java")

	// Backward compatibility aliases
	queryCmd.Flags().IntVarP(&contextLines, "before", "B", 0, "Alias for --context")
	_ = queryCmd.Flags().MarkHidden("before")
}

func runQuery(cmd *cobra.Command, args []string) error {
	sourceURI := args[0]

	// Add format hint for local files if specified
	if logFormat != "auto" && !strings.HasPrefix(sourceURI, "cloudwatch://") && !strings.HasPrefix(sourceURI, "@") {
		if strings.Contains(sourceURI, "?") {
			sourceURI += "&format=" + logFormat
		} else if strings.HasPrefix(sourceURI, "file://") {
			sourceURI += "?format=" + logFormat
		} else {
			// Bare path - convert to file:// with format
			sourceURI = "file://" + sourceURI + "?format=" + logFormat
		}
	}

	// Parse time range
	start, err := parseTimeArg(startTime)
	if err != nil {
		return fmt.Errorf("invalid start time: %w", err)
	}
	end, err := parseTimeArg(endTime)
	if err != nil {
		return fmt.Errorf("invalid end time: %w", err)
	}

	Debugf("Time range: %s to %s", start.Format(time.RFC3339), end.Format(time.RFC3339))

	if !start.Before(end) {
		return fmt.Errorf("start time must be before end time")
	}

	// Open the source
	src, err := source.Open(sourceURI)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	ctx := context.Background()
	Debugf("Source type: %s", src.Type())

	// Handle CloudWatch-specific features
	if src.Type() == "cloudwatch" {
		cwSrc := src.(*cloudwatch.Source)

		// Handle --dry-run
		if dryRun {
			return estimateCloudWatchCost(ctx, cwSrc, start, end)
		}

		// Handle --url
		if showURL {
			defer func() {
				consoleURL := buildConsoleURL(cwSrc.Region(), []string{cwSrc.LogGroup()}, start, end, queryString)
				render.Newline()
				render.Info("AWS Console URL:")
				render.Info("  %s", consoleURL)
			}()
		}
	}

	// Build filter regex
	var filterRegex *regexp.Regexp
	if filter != "" {
		filterRegex, err = regexp.Compile("(?i)" + filter)
		if err != nil {
			return fmt.Errorf("invalid filter pattern: %w", err)
		}
	}

	// Build query params
	params := source.QueryParams{
		StartTime: start,
		EndTime:   end,
		Filter:    filterRegex,
		Query:     queryString,
		Limit:     limit,
		Context:   contextLines,
	}

	// For CloudWatch with stats mode, build a special query
	if showStats && src.Type() == "cloudwatch" && queryString == "" {
		params.Query = cloudwatch.BuildStatsQuery(filter, limit)
	}

	// Run query
	render.Status("Querying %s...", sourceURI)
	results, err := src.Query(ctx, params)
	if err != nil {
		return err
	}

	// Determine output writer
	writer := os.Stdout
	if exportFile != "" {
		writer, err = os.Create(exportFile)
		if err != nil {
			return fmt.Errorf("failed to create export file: %w", err)
		}
		defer func() { _ = writer.Close() }()
	}

	// Format output with highlighting
	formatter := output.NewFormatter(getOutputFormat(), writer)
	if filter != "" && !showStats {
		formatter.WithHighlight(filter)
	}
	if err := formatter.FormatEntries(results); err != nil {
		return err
	}

	if exportFile != "" {
		render.Success("Results exported to %s", exportFile)
	}

	// Record in case timeline (unless --no-capture)
	if !noCapture {
		captureQueryToCaseNew(sourceURI, src, start, end, filter, queryString, len(results), markQuery)
	}

	// Cache pointers for evidence collection
	cachePtrsFromEntries(results, src)

	// Watch mode
	if watchInterval > 0 {
		return runWatchModeNew(ctx, src, sourceURI, params, filter)
	}

	return nil
}

// runWatchModeNew runs the query repeatedly at the specified interval.
func runWatchModeNew(ctx context.Context, src source.Source, sourceURI string, baseParams source.QueryParams, filterPattern string) error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(watchInterval) * time.Second)
	defer ticker.Stop()

	render.Newline()
	render.Status("Watch mode: refreshing every %ds (Ctrl+C to stop)...", watchInterval)

	for {
		select {
		case <-sigChan:
			render.Info("\nWatch mode stopped.")
			return nil
		case <-ticker.C:
			// Re-parse times for each iteration
			start, err := parseTimeArg(startTime)
			if err != nil {
				render.Warning("invalid start time: %v", err)
				continue
			}
			end, err := parseTimeArg(endTime)
			if err != nil {
				render.Warning("invalid end time: %v", err)
				continue
			}

			params := baseParams
			params.StartTime = start
			params.EndTime = end

			results, err := src.Query(ctx, params)
			if err != nil {
				render.Warning("query failed: %v", err)
				continue
			}

			// Clear screen and show timestamp
			fmt.Print("\033[2J\033[H")
			render.Status("Last updated: %s (%d results)", time.Now().Format("15:04:05"), len(results))
			render.Newline()

			formatter := output.NewFormatter(getOutputFormat(), os.Stdout)
			if filterPattern != "" && !showStats {
				formatter.WithHighlight(filterPattern)
			}
			if err := formatter.FormatEntries(results); err != nil {
				render.Warning("format error: %v", err)
			}
		}
	}
}

// estimateCloudWatchCost estimates CloudWatch query cost.
func estimateCloudWatchCost(ctx context.Context, src *cloudwatch.Source, start, end time.Time) error {
	duration := end.Sub(start)

	group, err := src.Client().GetLogGroup(ctx, src.LogGroup())
	if err != nil {
		return fmt.Errorf("could not get log group info: %w", err)
	}

	var estimatedBytes int64
	if group.CreationTime.IsZero() || group.StoredBytes == 0 {
		render.Info("Cost estimate unavailable (no data)")
		return nil
	}

	groupAge := time.Since(group.CreationTime)
	if groupAge <= 0 {
		groupAge = 24 * time.Hour
	}

	ratio := float64(duration) / float64(groupAge)
	if ratio > 1 {
		ratio = 1
	}
	estimatedBytes = int64(float64(group.StoredBytes) * ratio)

	costPerGB := 0.005
	costEstimate := float64(estimatedBytes) / (1024 * 1024 * 1024) * costPerGB

	render.RenderCostEstimate(ui.CostEstimate{
		LogGroups: []ui.LogGroupEstimate{{
			Name:          src.LogGroup(),
			TotalSize:     formatBytesHuman(group.StoredBytes),
			EstimatedScan: formatBytesHuman(estimatedBytes),
		}},
		TimeRange:     fmt.Sprintf("%s to %s (%s)", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"), formatDuration(duration)),
		TotalBytes:    formatBytesHuman(estimatedBytes),
		EstimatedCost: costEstimate,
	})

	return nil
}

// captureQueryToCaseNew adds the query to the active case timeline.
func captureQueryToCaseNew(sourceURI string, src source.Source, start, end time.Time, filterStr, queryStr string, resultCount int, marked bool) {
	mgr, err := cases.NewManager()
	if err != nil {
		return
	}

	// Build command string
	var cmdParts []string
	cmdParts = append(cmdParts, "clew query")
	cmdParts = append(cmdParts, fmt.Sprintf("%q", sourceURI))
	cmdParts = append(cmdParts, fmt.Sprintf("-s %s", startTime))
	if endTime != "now" && endTime != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("-e %s", endTime))
	}
	if filterStr != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("-f %q", filterStr))
	}
	if queryStr != "" {
		cmdParts = append(cmdParts, fmt.Sprintf("-q %q", queryStr))
	}

	meta := src.Metadata()

	entry := cases.TimelineEntry{
		SourceURI:  sourceURI,
		SourceType: meta.Type,
		Profile:    meta.Profile,
		AccountID:  meta.AccountID,
		Command:    strings.Join(cmdParts, " "),
		LogGroup:   meta.URI, // Deprecated but kept for backward compat
		Filter:     filterStr,
		Query:      queryStr,
		StartTime:  start,
		EndTime:    end,
		Results:    resultCount,
		Marked:     marked,
	}

	_ = mgr.AddQueryToTimeline(entry)
}

// cachePtrsFromEntries caches pointer values for evidence collection.
func cachePtrsFromEntries(entries []source.Entry, src source.Source) {
	var ptrEntries []cases.PtrEntry
	meta := src.Metadata()

	for _, e := range entries {
		if e.Ptr != "" {
			ptrEntries = append(ptrEntries, cases.PtrEntry{
				Ptr:        e.Ptr,
				SourceURI:  meta.URI,
				SourceType: meta.Type,
				Stream:     e.Stream,
				LogGroup:   meta.URI, // Deprecated but kept for backward compat
				LogStream:  e.Stream, // Deprecated but kept for backward compat
				Profile:    meta.Profile,
				AccountID:  meta.AccountID,
			})
		}
	}

	if len(ptrEntries) == 0 {
		return
	}

	mgr, err := cases.NewManager()
	if err != nil {
		return
	}

	_ = mgr.SavePtrCacheWithMetadata(ptrEntries)
}

// parseTimeArg parses time strings (RFC3339 or relative like "2h", "30m", "7d").
func parseTimeArg(input string) (time.Time, error) {
	if input == "" || input == "now" {
		return time.Now().UTC(), nil
	}

	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, input); err == nil {
		return t, nil
	}

	// Parse relative (e.g., "2h", "30m", "7d")
	re := regexp.MustCompile(`^(\d+)([mhd])$`)
	matches := re.FindStringSubmatch(input)
	if matches != nil {
		value := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &value)
		unit := matches[2]
		var duration time.Duration
		switch unit {
		case "m":
			duration = time.Duration(value) * time.Minute
		case "h":
			duration = time.Duration(value) * time.Hour
		case "d":
			duration = time.Duration(value) * 24 * time.Hour
		}
		return time.Now().UTC().Add(-duration), nil
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s - use RFC3339 (2025-12-02T06:00:00Z) or relative (2h, 30m, 7d)", input)
}

func formatBytesHuman(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}

// buildConsoleURL generates a CloudWatch Logs Insights console URL.
func buildConsoleURL(region string, logGroups []string, start, end time.Time, query string) string {
	var sources []string
	for _, lg := range logGroups {
		sources = append(sources, "~'"+url.QueryEscape(lg))
	}
	sourceStr := strings.Join(sources, "")

	startMs := start.UnixMilli()
	endMs := end.UnixMilli()

	queryDetail := fmt.Sprintf("~(end~%d~start~%d~timeType~'ABSOLUTE~editorString~'%s~source~(%s))",
		endMs,
		startMs,
		url.QueryEscape(query),
		sourceStr,
	)

	return fmt.Sprintf("https://%s.console.aws.amazon.com/cloudwatch/home?region=%s#logsV2:logs-insights$3FqueryDetail$3D%s",
		region,
		region,
		queryDetail,
	)
}
