package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/jmurray2011/clew/pkg/timeutil"
)

// FormatEntries outputs log entries in the configured format.
func (f *Formatter) FormatEntries(entries []cloudwatch.Entry) error {
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
func (f *Formatter) formatEntriesText(entries []cloudwatch.Entry) error {
	if len(entries) == 0 {
		f.renderer.NoResults()
		return nil
	}

	// Check if this is a stats/aggregation result (no standard log fields)
	isStatsResult := entries[0].Timestamp.IsZero() && entries[0].Message == ""

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
					ctx.Message)))
			}
			_, _ = fmt.Fprintln(f.writer, ui.ContextStyle.Render("--- match ---"))
		}

		// Header line: timestamp | stream
		if len(entry.Context.Before) > 0 {
			_, _ = fmt.Fprint(f.writer, ui.MatchMarkerStyle.Render(">> "))
		}
		_, _ = fmt.Fprint(f.writer, ui.TimestampStyle.Render(entry.Timestamp.Format("2006-01-02 15:04:05.000")))
		_, _ = fmt.Fprint(f.writer, " | ")
		_, _ = fmt.Fprint(f.writer, ui.LogStreamStyle.Render(entry.Stream))
		_, _ = fmt.Fprintln(f.writer)

		// Message with indentation for multi-line content
		lines := strings.Split(entry.Message, "\n")
		for _, line := range lines {
			if f.highlight != nil {
				line = f.highlight.ReplaceAllStringFunc(line, func(match string) string {
					return ui.HighlightStyle.Render(match)
				})
			}
			_, _ = fmt.Fprintf(f.writer, "  %s\n", line)
		}

		if i < len(entries)-1 {
			_, _ = fmt.Fprintln(f.writer)
		}
	}

	return nil
}

// formatEntriesStatsText outputs stats/aggregation entries in a table format.
func (f *Formatter) formatEntriesStatsText(entries []cloudwatch.Entry) error {
	if len(entries) == 0 {
		return nil
	}

	var headers []string
	for name := range entries[0].Fields {
		if name != "@ptr" {
			headers = append(headers, name)
		}
	}
	sortFields(headers)

	var rows [][]string
	for _, entry := range entries {
		row := make([]string, len(headers))
		for i, name := range headers {
			row[i] = entry.Fields[name]
		}
		rows = append(rows, row)
	}

	f.renderer.Table(headers, rows)
	return nil
}

// formatEntriesJSON outputs entries as a JSON array.
func (f *Formatter) formatEntriesJSON(entries []cloudwatch.Entry) error {
	type jsonContext struct {
		Timestamp string `json:"timestamp"`
		Message   string `json:"message"`
	}
	type jsonEntry struct {
		Timestamp     string            `json:"timestamp"`
		Stream        string            `json:"stream"`
		Source        string            `json:"source,omitempty"`
		Message       string            `json:"message"`
		ContextBefore []jsonContext     `json:"context_before,omitempty"`
		ContextAfter  []jsonContext     `json:"context_after,omitempty"`
		Fields        map[string]string `json:"fields,omitempty"`
	}

	jsonEntries := make([]jsonEntry, len(entries))
	for i, e := range entries {
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
		}

		if len(e.Context.Before) > 0 {
			jsonEntries[i].ContextBefore = make([]jsonContext, len(e.Context.Before))
			for j, ctx := range e.Context.Before {
				jsonEntries[i].ContextBefore[j] = jsonContext{
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
func (f *Formatter) formatEntriesCSV(entries []cloudwatch.Entry) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"timestamp", "stream", "source", "message"}); err != nil {
		return err
	}

	for _, e := range entries {
		record := []string{
			e.Timestamp.Format(time.RFC3339Nano),
			e.Stream,
			e.Source,
			e.Message,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// FormatStreams outputs stream information in the configured format.
func (f *Formatter) FormatStreams(streams []cloudwatch.StreamInfo) error {
	switch f.format {
	case FormatJSON:
		return f.formatStreamsJSON(streams)
	case FormatCSV:
		return f.formatStreamsCSV(streams)
	default:
		return f.formatStreamsText(streams)
	}
}

func (f *Formatter) formatStreamsText(streams []cloudwatch.StreamInfo) error {
	if len(streams) == 0 {
		_, _ = fmt.Fprintln(f.writer, ui.MutedStyle.Render("No streams found."))
		return nil
	}

	for _, s := range streams {
		name := ui.SuccessStyle.Render(s.Name)
		if !s.LastEventTime.IsZero() {
			ago := time.Since(s.LastEventTime)
			_, _ = fmt.Fprintf(f.writer, "%s   last event: %s ago\n", name, timeutil.FormatDuration(ago))
		} else {
			_, _ = fmt.Fprintf(f.writer, "%s   last event: N/A\n", name)
		}
	}

	return nil
}

func (f *Formatter) formatStreamsJSON(streams []cloudwatch.StreamInfo) error {
	type jsonStream struct {
		Name      string `json:"name"`
		LastTime  string `json:"lastTime,omitempty"`
		FirstTime string `json:"firstTime,omitempty"`
	}

	jsonStreams := make([]jsonStream, len(streams))
	for i, s := range streams {
		jsonStreams[i] = jsonStream{Name: s.Name}
		if !s.LastEventTime.IsZero() {
			jsonStreams[i].LastTime = s.LastEventTime.Format(time.RFC3339)
		}
		if !s.FirstEventTime.IsZero() {
			jsonStreams[i].FirstTime = s.FirstEventTime.Format(time.RFC3339)
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonStreams)
}

func (f *Formatter) formatStreamsCSV(streams []cloudwatch.StreamInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"name", "lastTime", "firstTime"}); err != nil {
		return err
	}

	for _, s := range streams {
		lastTime := ""
		if !s.LastEventTime.IsZero() {
			lastTime = s.LastEventTime.Format(time.RFC3339)
		}
		firstTime := ""
		if !s.FirstEventTime.IsZero() {
			firstTime = s.FirstEventTime.Format(time.RFC3339)
		}
		if err := writer.Write([]string{s.Name, lastTime, firstTime}); err != nil {
			return err
		}
	}

	return nil
}
