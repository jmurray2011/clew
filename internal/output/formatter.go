package output

import (
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/jmurray2011/clew/internal/logging"
	"github.com/jmurray2011/clew/internal/ui"
)

// Format specifies the output format type.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatCSV  Format = "csv"
)

// Formatter handles output formatting for different formats.
type Formatter struct {
	format    Format
	writer    io.Writer
	highlight *regexp.Regexp
	renderer  *ui.Renderer
}

// NewFormatter creates a new formatter with the specified format.
func NewFormatter(format string, writer io.Writer) *Formatter {
	return &Formatter{
		format:   Format(format),
		writer:   writer,
		renderer: ui.NewRendererWithOptions(ui.WithOutput(writer)),
	}
}

// WithHighlight sets a pattern to highlight in text output.
func (f *Formatter) WithHighlight(pattern string) *Formatter {
	if pattern != "" {
		re, err := regexp.Compile("(?i)(" + pattern + ")")
		if err != nil {
			logging.Warn("Invalid highlight pattern %q: %v (highlighting disabled)", pattern, err)
		} else {
			f.highlight = re
		}
	}
	return f
}

// sortFields sorts field names with common aggregation fields first.
func sortFields(fields []string) {
	priority := map[string]int{
		"time_bucket": 0,
		"count":       1,
		"sum":         2,
		"avg":         3,
		"min":         4,
		"max":         5,
	}

	sort.Slice(fields, func(i, j int) bool {
		pi, oki := priority[fields[i]]
		pj, okj := priority[fields[j]]
		if oki && !okj {
			return true
		}
		if okj && !oki {
			return false
		}
		if oki && okj {
			return pi < pj
		}
		return fields[i] < fields[j]
	})
}

// truncateMessage truncates a message to maxLen characters.
func truncateMessage(msg string, maxLen int) string {
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", "")
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}
