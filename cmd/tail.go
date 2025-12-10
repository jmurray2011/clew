package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/jmurray2011/clew/internal/source"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/jmurray2011/clew/pkg/lru"

	"github.com/spf13/cobra"
)

// Tail command configuration constants
const (
	// DefaultTailLookback is how far back to start when beginning a tail session
	DefaultTailLookback = 30 * time.Second

	// TailLRUCacheCapacity is the capacity for the LRU deduplication cache
	// This prevents unbounded memory growth during long tail sessions
	TailLRUCacheCapacity = 10000
)

var (
	tailFilter   string
	tailInterval int
)

var tailCmd = &cobra.Command{
	Use:   "tail <source>",
	Short: "Tail logs in real-time",
	Long: `Follow logs in real-time, similar to 'tail -f'.

Source URIs:
  cloudwatch:///log-group?profile=x&region=y   AWS CloudWatch Logs
  file:///path/to/file.log                     Local file
  /var/log/app.log                             Local file (shorthand)
  @alias-name                                  Config alias

Examples:
  # Tail CloudWatch logs
  clew tail "cloudwatch:///app/logs?profile=prod"

  # Tail a local file
  clew tail /var/log/app.log

  # Tail with a filter
  clew tail @prod-api -f "error|exception"

  # Faster polling (every 2 seconds)
  clew tail @prod-api --interval 2`,
	Args: cobra.ExactArgs(1),
	RunE: runTail,
}

func init() {
	rootCmd.AddCommand(tailCmd)

	tailCmd.Flags().StringVarP(&tailFilter, "filter", "f", "", "Filter pattern for messages")
	tailCmd.Flags().IntVar(&tailInterval, "interval", 5, "Polling interval in seconds")
}

func runTail(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	sourceURI := args[0]

	src, err := source.Open(sourceURI)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Handle Ctrl+C gracefully
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	go func() {
		<-sigChan
		app.Render.Info("\nStopping tail...")
		cancel()
	}()

	// Compile filter regex
	var filterRegex *regexp.Regexp
	if tailFilter != "" {
		filterRegex, err = regexp.Compile("(?i)" + tailFilter)
		if err != nil {
			return fmt.Errorf("invalid filter pattern: %w", err)
		}
	}

	// Try streaming tail first (if source supports it)
	params := source.TailParams{
		Filter: filterRegex,
	}

	events, err := src.Tail(ctx, params)
	if err != nil {
		// Fall back to polling-based tail
		app.Debugf("Streaming tail not supported, using polling: %v", err)
		return runPollingTail(ctx, app, src, sourceURI, filterRegex)
	}

	// Compile highlight regex
	var highlightRe *regexp.Regexp
	if tailFilter != "" {
		highlightRe, _ = regexp.Compile("(?i)(" + tailFilter + ")")
	}

	app.Render.Status("Tailing %s (Ctrl+C to stop)...", sourceURI)
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

// runPollingTail implements polling-based tailing for sources that don't support streaming.
func runPollingTail(ctx context.Context, app *App, src source.Source, sourceURI string, filterRegex *regexp.Regexp) error {
	// Compile highlight regex
	var highlightRe *regexp.Regexp
	if tailFilter != "" {
		highlightRe, _ = regexp.Compile("(?i)(" + tailFilter + ")")
	}

	// Start from a short lookback to catch recent events
	startTime := time.Now().Add(-DefaultTailLookback)
	// Use LRU cache to prevent unbounded memory growth while maintaining dedup state
	seenEvents := lru.New(TailLRUCacheCapacity)

	app.Render.Status("Tailing %s (Ctrl+C to stop)...", sourceURI)
	app.Render.Newline()

	ticker := time.NewTicker(time.Duration(tailInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			params := source.QueryParams{
				StartTime: startTime,
				EndTime:   time.Now(),
				Filter:    filterRegex,
				Limit:     100,
			}

			results, err := src.Query(ctx, params)
			if err != nil {
				app.Render.Warning("query failed: %v", err)
				continue
			}

			for _, entry := range results {
				// Deduplicate events using LRU cache
				eventKey := fmt.Sprintf("%s-%d", entry.Stream, entry.Timestamp.UnixNano())
				if !seenEvents.Add(eventKey) {
					continue // Already seen
				}

				// Highlight filter matches in message
				msg := entry.Message
				if highlightRe != nil {
					msg = highlightRe.ReplaceAllStringFunc(msg, func(match string) string {
						return ui.HighlightStyle.Render(match)
					})
				}

				// Print formatted event
				ts := ui.TimestampStyle.Render(entry.Timestamp.Format("15:04:05"))
				stream := ui.LogStreamStyle.Render(entry.Stream)
				fmt.Printf("%s | %s | %s\n", ts, stream, msg)
			}

			// Move start time forward, keep a small overlap
			if len(results) > 0 {
				startTime = results[len(results)-1].Timestamp.Add(-time.Second)
			}
		}
	}
}
