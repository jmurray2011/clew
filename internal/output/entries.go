package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/source"
	"github.com/jmurray2011/clew/internal/ui"
)

// FormatEntries outputs source entries in the configured format.
// This is the generic version that works with the source.Entry type.
func (f *Formatter) FormatEntries(entries []source.Entry) error {
	switch f.format {
	case FormatJSON:
		return f.formatEntriesJSON(entries)
	case FormatCSV:
		return f.formatEntriesCSV(entries)
	default:
		return f.formatEntriesText(entries)
	}
}

// formatEntriesText outputs entries in human-readable text format.
func (f *Formatter) formatEntriesText(entries []source.Entry) error {
	if len(entries) == 0 {
		f.renderer.NoResults()
		return nil
	}

	// Check if this is a stats/aggregation result (no standard log fields)
	isStatsResult := len(entries) > 0 && entries[0].Timestamp.IsZero() && entries[0].Message == ""

	if isStatsResult {
		return f.formatEntriesStatsText(entries)
	}

	for i, entry := range entries {
		// Print context lines first (if any)
		if len(entry.Context.Before) > 0 {
			_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("--- %d lines before ---", len(entry.Context.Before))))
			for _, ctx := range entry.Context.Before {
				_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("  %s  %s",
					ctx.Timestamp.Format("15:04:05"),
					truncateMessage(ctx.Message, 200))))
			}
			_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render("--- match ---"))
		}

		// Header line with index, timestamp and stream
		// Show [N] index for easy reference with `clew case keep N`
		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render(fmt.Sprintf("[%d] ", i+1)))
		if len(entry.Context.Before) > 0 {
			_, _ = fmt.Fprint(f.writer, ui.MatchMarkerStyle.Render(">> "))
		}
		_, _ = fmt.Fprint(f.writer, ui.TimestampStyle.Render(entry.Timestamp.Format("2006-01-02 15:04:05.000")))
		_, _ = fmt.Fprint(f.writer, " | ")
		_, _ = fmt.Fprint(f.writer, ui.LogStreamStyle.Render(entry.Stream))

		// Show shortened pointer suffix (unique chars are at the end for CloudWatch)
		if entry.Ptr != "" {
			suffix := entry.Ptr
			// For file:// URIs, show a shortened version
			if strings.HasPrefix(entry.Ptr, "file://") {
				// Show last part of path + line number
				if idx := strings.LastIndex(entry.Ptr, "/"); idx >= 0 {
					suffix = entry.Ptr[idx+1:]
				}
			} else if len(suffix) > 12 {
				// CloudWatch @ptr - show suffix
				suffix = suffix[len(suffix)-12:]
			}
			_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render(fmt.Sprintf("  @%s", suffix)))
		}
		_, _ = fmt.Fprintln(f.writer)

		// Message with indentation for multi-line content
		message := entry.Message
		lines := strings.Split(message, "\n")
		for _, line := range lines {
			// Apply highlighting if pattern is set
			if f.highlight != nil {
				line = f.highlight.ReplaceAllStringFunc(line, func(match string) string {
					return ui.HighlightStyle.Render(match)
				})
			}
			_, _ = fmt.Fprintf(f.writer, "  %s\n", line)
		}

		// Print after context lines (if any)
		if len(entry.Context.After) > 0 {
			_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("--- %d lines after ---", len(entry.Context.After))))
			for _, ctx := range entry.Context.After {
				_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("  %s  %s",
					ctx.Timestamp.Format("15:04:05"),
					truncateMessage(ctx.Message, 200))))
			}
		}

		// Add separator between entries (except for last one)
		if i < len(entries)-1 {
			_, _ = fmt.Fprintln(f.writer)
		}
	}

	return nil
}

// formatEntriesStatsText outputs stats/aggregation entries in a table format.
func (f *Formatter) formatEntriesStatsText(entries []source.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	// Collect all field names from the first entry (excluding internal @ptr)
	var headers []string
	for name := range entries[0].Fields {
		if name != "@ptr" {
			headers = append(headers, name)
		}
	}

	// Sort field names for consistent output
	sortFields(headers)

	// Build rows from entries
	var rows [][]string
	for _, entry := range entries {
		row := make([]string, len(headers))
		for i, name := range headers {
			row[i] = entry.Fields[name]
		}
		rows = append(rows, row)
	}

	// Use renderer's Table method for formatted output
	f.renderer.Table(headers, rows)
	return nil
}

// formatEntriesJSON outputs entries as a JSON array.
func (f *Formatter) formatEntriesJSON(entries []source.Entry) error {
	type jsonContext struct {
		Timestamp string `json:"timestamp"`
		Message   string `json:"message"`
	}
	type jsonEntry struct {
		Timestamp     string            `json:"timestamp"`
		Stream        string            `json:"stream"`
		Source        string            `json:"source,omitempty"`
		Message       string            `json:"message"`
		Ptr           string            `json:"ptr,omitempty"`
		ContextBefore []jsonContext     `json:"context_before,omitempty"`
		ContextAfter  []jsonContext     `json:"context_after,omitempty"`
		Fields        map[string]string `json:"fields,omitempty"`
	}

	jsonEntries := make([]jsonEntry, len(entries))
	for i, e := range entries {
		// Create fields map without the standard fields
		fields := make(map[string]string)
		for k, v := range e.Fields {
			if k != "@timestamp" && k != "@logStream" && k != "@message" && k != "@ptr" {
				fields[k] = v
			}
		}

		jsonEntries[i] = jsonEntry{
			Timestamp: e.Timestamp.Format(time.RFC3339Nano),
			Stream:    e.Stream,
			Source:    e.Source,
			Message:   e.Message,
			Ptr:       e.Ptr,
		}

		// Add context if present
		if len(e.Context.Before) > 0 {
			jsonEntries[i].ContextBefore = make([]jsonContext, len(e.Context.Before))
			for j, ctx := range e.Context.Before {
				jsonEntries[i].ContextBefore[j] = jsonContext{
					Timestamp: ctx.Timestamp.Format(time.RFC3339Nano),
					Message:   ctx.Message,
				}
			}
		}
		if len(e.Context.After) > 0 {
			jsonEntries[i].ContextAfter = make([]jsonContext, len(e.Context.After))
			for j, ctx := range e.Context.After {
				jsonEntries[i].ContextAfter[j] = jsonContext{
					Timestamp: ctx.Timestamp.Format(time.RFC3339Nano),
					Message:   ctx.Message,
				}
			}
		}

		if len(fields) > 0 {
			jsonEntries[i].Fields = fields
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonEntries)
}

// formatEntriesCSV outputs entries in CSV format.
func (f *Formatter) formatEntriesCSV(entries []source.Entry) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"timestamp", "stream", "source", "message", "ptr"}); err != nil {
		return err
	}

	// Write records
	for _, e := range entries {
		record := []string{
			e.Timestamp.Format(time.RFC3339Nano),
			e.Stream,
			e.Source,
			e.Message,
			e.Ptr,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// FormatSourceStreams outputs source stream information in the configured format.
func (f *Formatter) FormatSourceStreams(streams []source.StreamInfo) error {
	switch f.format {
	case FormatJSON:
		return f.formatSourceStreamsJSON(streams)
	case FormatCSV:
		return f.formatSourceStreamsCSV(streams)
	default:
		return f.formatSourceStreamsText(streams)
	}
}

// formatSourceStreamsText outputs streams in human-readable text format.
func (f *Formatter) formatSourceStreamsText(streams []source.StreamInfo) error {
	if len(streams) == 0 {
		_, _ = fmt.Fprintln(f.writer, ui.MutedStyle.Render("No streams found."))
		return nil
	}

	for _, s := range streams {
		_, _ = fmt.Fprintln(f.writer, ui.SuccessStyle.Render(s.Name))

		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render("  Last Event: "))
		if !s.LastTime.IsZero() {
			_, _ = fmt.Fprintln(f.writer, s.LastTime.Format("2006-01-02T15:04:05Z"))
		} else {
			_, _ = fmt.Fprintln(f.writer, "N/A")
		}

		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render("  First Event: "))
		if !s.FirstTime.IsZero() {
			_, _ = fmt.Fprintln(f.writer, s.FirstTime.Format("2006-01-02T15:04:05Z"))
		} else {
			_, _ = fmt.Fprintln(f.writer, "N/A")
		}

		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render("  Size: "))
		_, _ = fmt.Fprintln(f.writer, formatBytes(s.Size))

		_, _ = fmt.Fprintln(f.writer)
	}

	return nil
}

// formatSourceStreamsJSON outputs streams as JSON.
func (f *Formatter) formatSourceStreamsJSON(streams []source.StreamInfo) error {
	type jsonStream struct {
		Name      string `json:"name"`
		LastTime  string `json:"lastTime,omitempty"`
		FirstTime string `json:"firstTime,omitempty"`
		Size      int64  `json:"size"`
	}

	jsonStreams := make([]jsonStream, len(streams))
	for i, s := range streams {
		jsonStreams[i] = jsonStream{
			Name: s.Name,
			Size: s.Size,
		}
		if !s.LastTime.IsZero() {
			jsonStreams[i].LastTime = s.LastTime.Format("2006-01-02T15:04:05Z")
		}
		if !s.FirstTime.IsZero() {
			jsonStreams[i].FirstTime = s.FirstTime.Format("2006-01-02T15:04:05Z")
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonStreams)
}

// formatSourceStreamsCSV outputs streams in CSV format.
func (f *Formatter) formatSourceStreamsCSV(streams []source.StreamInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"name", "lastTime", "firstTime", "size"}); err != nil {
		return err
	}

	for _, s := range streams {
		lastTime := ""
		if !s.LastTime.IsZero() {
			lastTime = s.LastTime.Format("2006-01-02T15:04:05Z")
		}
		firstTime := ""
		if !s.FirstTime.IsZero() {
			firstTime = s.FirstTime.Format("2006-01-02T15:04:05Z")
		}

		record := []string{s.Name, lastTime, firstTime, fmt.Sprintf("%d", s.Size)}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
