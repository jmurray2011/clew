package ui

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Renderer handles all terminal output with consistent styling.
type Renderer struct {
	out       io.Writer
	err       io.Writer
	noColor   bool
	quiet     bool
	highlight *regexp.Regexp
}

// NewRenderer creates a new Renderer with default settings.
func NewRenderer() *Renderer {
	return &Renderer{
		out: os.Stdout,
		err: os.Stderr,
	}
}

// Option is a functional option for configuring the Renderer.
type Option func(*Renderer)

// WithOutput sets the output writer.
func WithOutput(w io.Writer) Option {
	return func(r *Renderer) {
		r.out = w
	}
}

// WithError sets the error writer.
func WithError(w io.Writer) Option {
	return func(r *Renderer) {
		r.err = w
	}
}

// WithNoColor disables color output.
func WithNoColor(noColor bool) Option {
	return func(r *Renderer) {
		r.noColor = noColor
	}
}

// WithQuiet enables quiet mode (suppresses status messages).
func WithQuiet(quiet bool) Option {
	return func(r *Renderer) {
		r.quiet = quiet
	}
}

// WithHighlight sets a pattern to highlight in output.
func WithHighlight(pattern string) Option {
	return func(r *Renderer) {
		if pattern != "" {
			r.highlight, _ = regexp.Compile("(?i)(" + pattern + ")")
		}
	}
}

// NewRendererWithOptions creates a new Renderer with the given options.
func NewRendererWithOptions(opts ...Option) *Renderer {
	r := NewRenderer()
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// render applies styling if color is enabled.
func (r *Renderer) render(style lipgloss.Style, text string) string {
	if r.noColor {
		return text
	}
	return style.Render(text)
}

// --- Status and Messages ---

// Status prints a status message (suppressed in quiet mode).
func (r *Renderer) Status(format string, args ...any) {
	if r.quiet {
		return
	}
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.err, r.render(StatusStyle, msg))
}

// Info prints an informational message.
func (r *Renderer) Info(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.out, msg)
}

// Success prints a success message.
func (r *Renderer) Success(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.out, r.render(SuccessStyle, msg))
}

// Warning prints a warning message.
func (r *Renderer) Warning(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.err, r.render(WarningStyle, "Warning: "+msg))
}

// Error prints an error message.
func (r *Renderer) Error(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.err, r.render(ErrorStyle, "Error: "+msg))
}

// Debug prints a debug message (only when verbose).
func (r *Renderer) Debug(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(r.err, r.render(MutedStyle, "[DEBUG] "+msg))
}

// --- Formatted Output ---

// KeyValue prints a key-value pair.
func (r *Renderer) KeyValue(key, value string) {
	label := r.render(LabelStyle, key+":")
	fmt.Fprintf(r.out, "%s %s\n", label, value)
}

// KeyValueIndent prints an indented key-value pair.
func (r *Renderer) KeyValueIndent(key, value string, indent int) {
	prefix := strings.Repeat("  ", indent)
	label := r.render(LabelStyle, key+":")
	fmt.Fprintf(r.out, "%s%s %s\n", prefix, label, value)
}

// Section prints a section title.
func (r *Renderer) Section(title string) {
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, r.render(SectionTitleStyle, title))
}

// Divider prints a horizontal divider.
func (r *Renderer) Divider() {
	fmt.Fprintln(r.out, r.render(MutedStyle, strings.Repeat("─", 40)))
}

// Newline prints a blank line.
func (r *Renderer) Newline() {
	fmt.Fprintln(r.out)
}

// --- Log Entry Rendering ---

// LogEntry renders a log entry with timestamp and stream.
func (r *Renderer) LogEntry(timestamp, logStream, message string) {
	ts := r.render(TimestampStyle, timestamp)
	stream := r.render(LogStreamStyle, logStream)

	fmt.Fprintf(r.out, "%s | %s\n", ts, stream)

	// Apply highlighting if set
	displayMsg := message
	if r.highlight != nil && !r.noColor {
		displayMsg = r.highlight.ReplaceAllStringFunc(message, func(match string) string {
			return HighlightStyle.Render(match)
		})
	}

	// Indent message lines
	lines := strings.Split(displayMsg, "\n")
	for _, line := range lines {
		fmt.Fprintf(r.out, "  %s\n", line)
	}
}

// LogEntryWithContext renders a log entry with context lines before it.
func (r *Renderer) LogEntryWithContext(timestamp, logStream, message string, context []string) {
	if len(context) > 0 {
		fmt.Fprintln(r.out, r.render(ContextStyle, fmt.Sprintf("--- %d lines before ---", len(context))))
		for _, ctx := range context {
			fmt.Fprintln(r.out, r.render(ContextStyle, "  "+ctx))
		}
		fmt.Fprintln(r.out, r.render(ContextStyle, "--- match ---"))
	}

	// Print match marker
	fmt.Fprint(r.out, r.render(MatchMarkerStyle, ">> "))
	r.LogEntry(timestamp, logStream, message)
}

// --- Table Rendering ---

// Table renders a simple table.
func (r *Renderer) Table(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header
	headerParts := make([]string, len(headers))
	for i, h := range headers {
		headerParts[i] = r.render(LabelStyle, fmt.Sprintf("%-*s", widths[i], h))
	}
	fmt.Fprintln(r.out, strings.Join(headerParts, "  "))

	// Print separator
	sepParts := make([]string, len(headers))
	for i, w := range widths {
		sepParts[i] = strings.Repeat("-", w)
	}
	fmt.Fprintln(r.out, r.render(MutedStyle, strings.Join(sepParts, "  ")))

	// Print rows
	for _, row := range rows {
		rowParts := make([]string, len(headers))
		for i := range headers {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			rowParts[i] = fmt.Sprintf("%-*s", widths[i], cell)
		}
		fmt.Fprintln(r.out, strings.Join(rowParts, "  "))
	}
}

// --- Specialized Renderers ---

// CostEstimate represents a cost estimation result.
type CostEstimate struct {
	LogGroups     []LogGroupEstimate
	TimeRange     string
	TotalBytes    string
	EstimatedCost float64
}

// LogGroupEstimate represents a single log group's cost estimate.
type LogGroupEstimate struct {
	Name          string
	TotalSize     string
	EstimatedScan string
}

// RenderCostEstimate renders a cost estimation result.
func (r *Renderer) RenderCostEstimate(est CostEstimate) {
	r.Status("Estimating query cost...")
	r.Newline()

	for _, lg := range est.LogGroups {
		fmt.Fprintln(r.out, r.render(LabelStyle, "  "+lg.Name))
		r.KeyValueIndent("Total size", lg.TotalSize, 2)
		r.KeyValueIndent("Estimated scan", lg.EstimatedScan, 2)
	}

	r.Newline()
	r.KeyValue("Time range", est.TimeRange)
	r.KeyValue("Estimated total scan", est.TotalBytes)
	r.KeyValue("Estimated cost", fmt.Sprintf("$%.4f", est.EstimatedCost))

	if est.EstimatedCost < 0.01 {
		r.Newline()
		r.Info(r.render(MutedStyle, "(Cost is likely less than $0.01)"))
	}

	r.Newline()
	r.Info(r.render(MutedStyle, "Note: This is a rough estimate assuming uniform log distribution."))
	r.Info(r.render(MutedStyle, "Actual costs may vary based on log patterns and retention settings."))
}

// NoResults prints a "no results" message.
func (r *Renderer) NoResults() {
	fmt.Fprintln(r.out, r.render(MutedStyle, "No results found."))
}
