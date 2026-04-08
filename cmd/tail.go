package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/jmurray2011/clew/pkg/lru"

	"github.com/spf13/cobra"
)

const (
	DefaultTailLookback  = 30 * time.Second
	TailLRUCacheCapacity = 10000
)

var (
	tailFilter   string
	tailInterval int
	tailStream   string
)

var tailCmd = &cobra.Command{
	Use:               "tail [log-group]",
	Aliases:           []string{"t"},
	Short:             "Follow logs in real-time",
	ValidArgsFunction: completeLogGroups,
	Long: `Follow CloudWatch logs in real-time, similar to 'tail -f'.

Examples:
  clew tail -e prod /app/api/prod -f "error|exception"
  clew tail -e test "docs.*error" --stream i-abc123
  clew tail -e test ? --interval 2`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTail,
}

func init() {
	rootCmd.AddCommand(tailCmd)

	tailCmd.Flags().StringVarP(&tailFilter, "filter", "f", "", "Filter pattern for messages")
	tailCmd.Flags().IntVar(&tailInterval, "interval", 5, "Polling interval in seconds")
	tailCmd.Flags().StringVar(&tailStream, "stream", "", "Substring match on @logStream")
}

func runTail(cmd *cobra.Command, args []string) error {
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

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		app.Render.Info("\nStopping tail...")
		cancel()
	}()

	var filterRegex *regexp.Regexp
	if tailFilter != "" {
		filterRegex, err = regexp.Compile("(?i)" + tailFilter)
		if err != nil {
			return fmt.Errorf("invalid filter pattern: %w", err)
		}
	}

	params := cloudwatch.TailInput{
		Filter: filterRegex,
		Stream: tailStream,
	}

	events, err := src.Tail(ctx, params)
	if err != nil {
		app.Debugf("Streaming tail not supported, using polling: %v", err)
		return runPollingTail(ctx, app, src, logGroup, filterRegex)
	}

	var highlightRe *regexp.Regexp
	if tailFilter != "" {
		highlightRe, _ = regexp.Compile("(?i)(" + tailFilter + ")")
	}

	app.Render.Status("Tailing %s (Ctrl+C to stop)...", logGroup)
	app.Render.Newline()

	for event := range events {
		msg := event.Message
		if highlightRe != nil {
			msg = highlightRe.ReplaceAllStringFunc(msg, func(match string) string {
				return ui.HighlightStyle.Render(match)
			})
		}

		ts := ui.TimestampStyle.Render(event.Timestamp.Format("15:04:05"))
		stream := ui.LogStreamStyle.Render(event.Stream)
		fmt.Printf("%s | %s | %s\n", ts, stream, msg)
	}

	return nil
}

func runPollingTail(ctx context.Context, app *App, src *cloudwatch.Source, logGroup string, filterRegex *regexp.Regexp) error {
	var highlightRe *regexp.Regexp
	if tailFilter != "" {
		highlightRe, _ = regexp.Compile("(?i)(" + tailFilter + ")")
	}

	startTime := time.Now().Add(-DefaultTailLookback)
	seenEvents := lru.New(TailLRUCacheCapacity)

	app.Render.Status("Tailing %s (Ctrl+C to stop)...", logGroup)
	app.Render.Newline()

	ticker := time.NewTicker(time.Duration(tailInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			params := cloudwatch.QueryInput{
				StartTime: startTime,
				EndTime:   time.Now(),
				Filter:    filterRegex,
				Stream:    tailStream,
				Limit:     100,
			}

			results, err := src.Query(ctx, params)
			if err != nil {
				app.Render.Warning("query failed: %v", err)
				continue
			}

			for _, entry := range results {
				eventKey := fmt.Sprintf("%s-%d", entry.Stream, entry.Timestamp.UnixNano())
				if !seenEvents.Add(eventKey) {
					continue
				}

				msg := entry.Message
				if highlightRe != nil {
					msg = highlightRe.ReplaceAllStringFunc(msg, func(match string) string {
						return ui.HighlightStyle.Render(match)
					})
				}

				ts := ui.TimestampStyle.Render(entry.Timestamp.Format("15:04:05"))
				stream := ui.LogStreamStyle.Render(entry.Stream)
				fmt.Printf("%s | %s | %s\n", ts, stream, msg)
			}

			if len(results) > 0 {
				startTime = results[len(results)-1].Timestamp.Add(-time.Second)
			}
		}
	}
}
