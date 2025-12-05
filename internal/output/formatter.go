package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
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
// The pattern is treated as a regular expression.
func (f *Formatter) WithHighlight(pattern string) *Formatter {
	if pattern != "" {
		re, err := regexp.Compile("(?i)(" + pattern + ")")
		if err != nil {
			log.Printf("[WARN] Invalid highlight pattern %q: %v (highlighting disabled)", pattern, err)
		} else {
			f.highlight = re
		}
	}
	return f
}

// FormatLogResults outputs log results in the configured format.
func (f *Formatter) FormatLogResults(results []cloudwatch.LogResult) error {
	switch f.format {
	case FormatJSON:
		return f.formatJSON(results)
	case FormatCSV:
		return f.formatCSV(results)
	default:
		return f.formatText(results)
	}
}

// formatText outputs results in human-readable text format.
func (f *Formatter) formatText(results []cloudwatch.LogResult) error {
	if len(results) == 0 {
		f.renderer.NoResults()
		return nil
	}

	// Check if this is a stats/aggregation result (no standard log fields)
	isStatsResult := len(results) > 0 && results[0].Timestamp == "" && results[0].Message == ""

	if isStatsResult {
		return f.formatStatsText(results)
	}

	for i, result := range results {
		// Print context lines first (if any)
		if len(result.Context) > 0 {
			fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("--- %d lines before ---", len(result.Context))))
			for _, ctx := range result.Context {
				fmt.Fprintln(f.writer, ui.ContextStyle.Render(fmt.Sprintf("  %s  %s",
					ctx.Timestamp.Format("15:04:05"),
					truncateMessage(ctx.Message, 200))))
			}
			fmt.Fprintln(f.writer, ui.ContextStyle.Render("--- match ---"))
		}

		// Header line with index, timestamp and log stream
		// Show [N] index for easy reference with `clew case keep N`
		fmt.Fprint(f.writer, ui.MutedStyle.Render(fmt.Sprintf("[%d] ", i+1)))
		if len(result.Context) > 0 {
			fmt.Fprint(f.writer, ui.MatchMarkerStyle.Render(">> "))
		}
		fmt.Fprint(f.writer, ui.TimestampStyle.Render(result.Timestamp))
		fmt.Fprint(f.writer, " | ")
		fmt.Fprint(f.writer, ui.LogStreamStyle.Render(result.LogStream))

		// Show shortened @ptr suffix (unique chars are at the end)
		if ptr, ok := result.Fields["@ptr"]; ok && ptr != "" {
			suffix := ptr
			if len(suffix) > 12 {
				suffix = suffix[len(suffix)-12:]
			}
			fmt.Fprint(f.writer, ui.MutedStyle.Render(fmt.Sprintf("  @%s", suffix)))
		}
		fmt.Fprintln(f.writer)

		// Message with indentation for multi-line content
		message := result.Message
		lines := strings.Split(message, "\n")
		for _, line := range lines {
			// Apply highlighting if pattern is set
			if f.highlight != nil {
				line = f.highlight.ReplaceAllStringFunc(line, func(match string) string {
					return ui.HighlightStyle.Render(match)
				})
			}
			fmt.Fprintf(f.writer, "  %s\n", line)
		}

		// Add separator between entries (except for last one)
		if i < len(results)-1 {
			fmt.Fprintln(f.writer)
		}
	}

	return nil
}

// formatStatsText outputs stats/aggregation results in a table format.
func (f *Formatter) formatStatsText(results []cloudwatch.LogResult) error {
	if len(results) == 0 {
		return nil
	}

	// Collect all field names from the first result (excluding internal @ptr)
	var headers []string
	for name := range results[0].Fields {
		if name != "@ptr" {
			headers = append(headers, name)
		}
	}

	// Sort field names for consistent output
	sortFields(headers)

	// Build rows from results
	var rows [][]string
	for _, result := range results {
		row := make([]string, len(headers))
		for i, name := range headers {
			row[i] = result.Fields[name]
		}
		rows = append(rows, row)
	}

	// Use renderer's Table method for formatted output
	f.renderer.Table(headers, rows)
	return nil
}

// sortFields sorts field names with common aggregation fields first.
func sortFields(fields []string) {
	// Priority order for common stats fields
	priority := map[string]int{
		"time_bucket": 0,
		"count":       1,
		"blocks":      1,
		"sum":         2,
		"avg":         3,
		"min":         4,
		"max":         5,
	}

	sort.Slice(fields, func(i, j int) bool {
		pi, oki := priority[fields[i]]
		pj, okj := priority[fields[j]]

		// Known priority fields come first
		if oki && !okj {
			return true
		}
		if okj && !oki {
			return false
		}
		if oki && okj {
			return pi < pj
		}
		// Alphabetical for unknown fields
		return fields[i] < fields[j]
	})
}

// truncateMessage truncates a message to maxLen characters.
func truncateMessage(msg string, maxLen int) string {
	// Remove newlines for context display
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", "")
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

// formatJSON outputs results as a JSON array.
func (f *Formatter) formatJSON(results []cloudwatch.LogResult) error {
	type jsonContext struct {
		Timestamp string `json:"timestamp"`
		Message   string `json:"message"`
	}
	type jsonResult struct {
		Timestamp string            `json:"timestamp"`
		LogStream string            `json:"logStream"`
		Message   string            `json:"message"`
		Ptr       string            `json:"ptr,omitempty"`
		Context   []jsonContext     `json:"context,omitempty"`
		Fields    map[string]string `json:"fields,omitempty"`
	}

	jsonResults := make([]jsonResult, len(results))
	for i, r := range results {
		// Create fields map without the standard fields (but keep @ptr separately)
		fields := make(map[string]string)
		for k, v := range r.Fields {
			if k != "@timestamp" && k != "@logStream" && k != "@message" && k != "@ptr" {
				fields[k] = v
			}
		}

		jsonResults[i] = jsonResult{
			Timestamp: r.Timestamp,
			LogStream: r.LogStream,
			Message:   r.Message,
			Ptr:       r.Fields["@ptr"], // Include @ptr for evidence collection
		}

		// Add context if present
		if len(r.Context) > 0 {
			jsonResults[i].Context = make([]jsonContext, len(r.Context))
			for j, ctx := range r.Context {
				jsonResults[i].Context[j] = jsonContext{
					Timestamp: ctx.Timestamp.Format("2006-01-02T15:04:05.000Z"),
					Message:   ctx.Message,
				}
			}
		}

		if len(fields) > 0 {
			jsonResults[i].Fields = fields
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonResults)
}

// formatCSV outputs results in CSV format.
func (f *Formatter) formatCSV(results []cloudwatch.LogResult) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"timestamp", "logStream", "message", "ptr"}); err != nil {
		return err
	}

	// Write records
	for _, r := range results {
		record := []string{r.Timestamp, r.LogStream, r.Message, r.Fields["@ptr"]}
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

// formatStreamsText outputs streams in human-readable text format.
func (f *Formatter) formatStreamsText(streams []cloudwatch.StreamInfo) error {
	if len(streams) == 0 {
		fmt.Fprintln(f.writer, ui.MutedStyle.Render("No streams found."))
		return nil
	}

	for _, s := range streams {
		fmt.Fprintln(f.writer, ui.SuccessStyle.Render(s.Name))

		fmt.Fprint(f.writer, ui.MutedStyle.Render("  Last Event: "))
		if !s.LastEventTime.IsZero() {
			fmt.Fprintln(f.writer, s.LastEventTime.Format("2006-01-02T15:04:05Z"))
		} else {
			fmt.Fprintln(f.writer, "N/A")
		}

		fmt.Fprint(f.writer, ui.MutedStyle.Render("  First Event: "))
		if !s.FirstEventTime.IsZero() {
			fmt.Fprintln(f.writer, s.FirstEventTime.Format("2006-01-02T15:04:05Z"))
		} else {
			fmt.Fprintln(f.writer, "N/A")
		}

		fmt.Fprint(f.writer, ui.MutedStyle.Render("  Stored Bytes: "))
		fmt.Fprintln(f.writer, formatBytes(s.StoredBytes))

		fmt.Fprintln(f.writer)
	}

	return nil
}

// formatStreamsJSON outputs streams as JSON.
func (f *Formatter) formatStreamsJSON(streams []cloudwatch.StreamInfo) error {
	type jsonStream struct {
		Name           string `json:"name"`
		LastEventTime  string `json:"lastEventTime,omitempty"`
		FirstEventTime string `json:"firstEventTime,omitempty"`
		StoredBytes    int64  `json:"storedBytes"`
	}

	jsonStreams := make([]jsonStream, len(streams))
	for i, s := range streams {
		jsonStreams[i] = jsonStream{
			Name:        s.Name,
			StoredBytes: s.StoredBytes,
		}
		if !s.LastEventTime.IsZero() {
			jsonStreams[i].LastEventTime = s.LastEventTime.Format("2006-01-02T15:04:05Z")
		}
		if !s.FirstEventTime.IsZero() {
			jsonStreams[i].FirstEventTime = s.FirstEventTime.Format("2006-01-02T15:04:05Z")
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonStreams)
}

// formatStreamsCSV outputs streams in CSV format.
func (f *Formatter) formatStreamsCSV(streams []cloudwatch.StreamInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"name", "lastEventTime", "firstEventTime", "storedBytes"}); err != nil {
		return err
	}

	for _, s := range streams {
		lastEvent := ""
		if !s.LastEventTime.IsZero() {
			lastEvent = s.LastEventTime.Format("2006-01-02T15:04:05Z")
		}
		firstEvent := ""
		if !s.FirstEventTime.IsZero() {
			firstEvent = s.FirstEventTime.Format("2006-01-02T15:04:05Z")
		}

		record := []string{s.Name, lastEvent, firstEvent, fmt.Sprintf("%d", s.StoredBytes)}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// formatBytes converts bytes to human-readable format.
func formatBytes(bytes int64) string {
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

// FormatLogGroups outputs log group information in the configured format.
func (f *Formatter) FormatLogGroups(groups []cloudwatch.LogGroupInfo) error {
	switch f.format {
	case FormatJSON:
		return f.formatGroupsJSON(groups)
	case FormatCSV:
		return f.formatGroupsCSV(groups)
	default:
		return f.formatGroupsText(groups)
	}
}

// formatGroupsText outputs log groups in human-readable text format.
func (f *Formatter) formatGroupsText(groups []cloudwatch.LogGroupInfo) error {
	if len(groups) == 0 {
		fmt.Fprintln(f.writer, ui.MutedStyle.Render("No log groups found."))
		return nil
	}

	for _, g := range groups {
		fmt.Fprintln(f.writer, ui.SuccessStyle.Render(g.Name))

		fmt.Fprint(f.writer, ui.MutedStyle.Render("  Size: "))
		fmt.Fprint(f.writer, formatBytes(g.StoredBytes))

		if g.RetentionDays > 0 {
			fmt.Fprintf(f.writer, "  |  Retention: %d days", g.RetentionDays)
		} else {
			fmt.Fprint(f.writer, "  |  Retention: Never expire")
		}

		if !g.CreationTime.IsZero() {
			fmt.Fprintf(f.writer, "  |  Created: %s", g.CreationTime.Format("2006-01-02"))
		}

		fmt.Fprintln(f.writer)
	}

	return nil
}

// formatGroupsJSON outputs log groups as JSON.
func (f *Formatter) formatGroupsJSON(groups []cloudwatch.LogGroupInfo) error {
	type jsonGroup struct {
		Name          string `json:"name"`
		StoredBytes   int64  `json:"storedBytes"`
		RetentionDays int    `json:"retentionDays,omitempty"`
		CreationTime  string `json:"creationTime,omitempty"`
	}

	jsonGroups := make([]jsonGroup, len(groups))
	for i, g := range groups {
		jsonGroups[i] = jsonGroup{
			Name:          g.Name,
			StoredBytes:   g.StoredBytes,
			RetentionDays: g.RetentionDays,
		}
		if !g.CreationTime.IsZero() {
			jsonGroups[i].CreationTime = g.CreationTime.Format("2006-01-02T15:04:05Z")
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonGroups)
}

// formatGroupsCSV outputs log groups in CSV format.
func (f *Formatter) formatGroupsCSV(groups []cloudwatch.LogGroupInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"name", "storedBytes", "retentionDays", "creationTime"}); err != nil {
		return err
	}

	for _, g := range groups {
		creationTime := ""
		if !g.CreationTime.IsZero() {
			creationTime = g.CreationTime.Format("2006-01-02T15:04:05Z")
		}

		record := []string{g.Name, fmt.Sprintf("%d", g.StoredBytes), fmt.Sprintf("%d", g.RetentionDays), creationTime}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// FormatMetricResult outputs metric query results in the configured format.
func (f *Formatter) FormatMetricResult(result *cloudwatch.MetricResult) error {
	switch f.format {
	case FormatJSON:
		return f.formatMetricResultJSON(result)
	case FormatCSV:
		return f.formatMetricResultCSV(result)
	default:
		return f.formatMetricResultText(result)
	}
}

// formatMetricResultText outputs metric results in human-readable text format.
func (f *Formatter) formatMetricResultText(result *cloudwatch.MetricResult) error {
	// Header
	fmt.Fprintf(f.writer, "%s/%s (%s)\n",
		ui.LabelStyle.Render(result.Namespace),
		ui.SuccessStyle.Render(result.MetricName),
		result.Statistic)

	if len(result.Dimensions) > 0 {
		dims := make([]string, 0, len(result.Dimensions))
		for k, v := range result.Dimensions {
			dims = append(dims, fmt.Sprintf("%s=%s", k, v))
		}
		fmt.Fprintf(f.writer, "Dimensions: %s\n", strings.Join(dims, ", "))
	}
	fmt.Fprintln(f.writer)

	// Find max value for scaling the bar chart
	maxVal := 0.0
	for _, dp := range result.DataPoints {
		if dp.Value > maxVal {
			maxVal = dp.Value
		}
	}

	// Print data points with ASCII bar chart
	barWidth := 40
	for _, dp := range result.DataPoints {
		ts := ui.TimestampStyle.Render(dp.Timestamp.Format("2006-01-02 15:04"))

		// Format value
		var valStr string
		if dp.Value == float64(int64(dp.Value)) {
			valStr = fmt.Sprintf("%8.0f", dp.Value)
		} else {
			valStr = fmt.Sprintf("%8.2f", dp.Value)
		}

		// Draw bar
		barLen := 0
		if maxVal > 0 {
			barLen = int((dp.Value / maxVal) * float64(barWidth))
		}
		bar := strings.Repeat("█", barLen)

		// Color the bar based on value relative to max
		ratio := 0.0
		if maxVal > 0 {
			ratio = dp.Value / maxVal
		}
		if ratio > 0.8 {
			bar = ui.ErrorStyle.Render(bar)
		} else if ratio > 0.5 {
			bar = ui.WarningStyle.Render(bar)
		} else {
			bar = ui.SuccessStyle.Render(bar)
		}

		fmt.Fprintf(f.writer, "%s  %s  %s\n", ts, valStr, bar)
	}

	return nil
}

// formatMetricResultJSON outputs metric results as JSON.
func (f *Formatter) formatMetricResultJSON(result *cloudwatch.MetricResult) error {
	type jsonDataPoint struct {
		Timestamp string  `json:"timestamp"`
		Value     float64 `json:"value"`
		Unit      string  `json:"unit,omitempty"`
	}
	type jsonResult struct {
		Namespace  string            `json:"namespace"`
		MetricName string            `json:"metricName"`
		Statistic  string            `json:"statistic"`
		Period     string            `json:"period"`
		Dimensions map[string]string `json:"dimensions,omitempty"`
		DataPoints []jsonDataPoint   `json:"dataPoints"`
	}

	jr := jsonResult{
		Namespace:  result.Namespace,
		MetricName: result.MetricName,
		Statistic:  result.Statistic,
		Period:     result.Period.String(),
		Dimensions: result.Dimensions,
		DataPoints: make([]jsonDataPoint, len(result.DataPoints)),
	}

	for i, dp := range result.DataPoints {
		jr.DataPoints[i] = jsonDataPoint{
			Timestamp: dp.Timestamp.Format(time.RFC3339),
			Value:     dp.Value,
			Unit:      dp.Unit,
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jr)
}

// formatMetricResultCSV outputs metric results in CSV format.
func (f *Formatter) formatMetricResultCSV(result *cloudwatch.MetricResult) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"timestamp", "value", "unit"}); err != nil {
		return err
	}

	for _, dp := range result.DataPoints {
		record := []string{
			dp.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%f", dp.Value),
			dp.Unit,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}

// FormatMetricsList outputs a list of available metrics.
func (f *Formatter) FormatMetricsList(metrics []cloudwatch.MetricInfo) error {
	switch f.format {
	case FormatJSON:
		return f.formatMetricsListJSON(metrics)
	case FormatCSV:
		return f.formatMetricsListCSV(metrics)
	default:
		return f.formatMetricsListText(metrics)
	}
}

// formatMetricsListText outputs metrics list in human-readable format.
func (f *Formatter) formatMetricsListText(metrics []cloudwatch.MetricInfo) error {
	for _, m := range metrics {
		fmt.Fprintln(f.writer, ui.SuccessStyle.Render(m.MetricName))
		if len(m.Dimensions) > 0 {
			for k, v := range m.Dimensions {
				fmt.Fprintf(f.writer, "  %s: %s\n", ui.MutedStyle.Render(k), v)
			}
		}
	}
	return nil
}

// formatMetricsListJSON outputs metrics list as JSON.
func (f *Formatter) formatMetricsListJSON(metrics []cloudwatch.MetricInfo) error {
	type jsonMetric struct {
		Namespace  string            `json:"namespace"`
		MetricName string            `json:"metricName"`
		Dimensions map[string]string `json:"dimensions,omitempty"`
	}

	jsonMetrics := make([]jsonMetric, len(metrics))
	for i, m := range metrics {
		jsonMetrics[i] = jsonMetric{
			Namespace:  m.Namespace,
			MetricName: m.MetricName,
			Dimensions: m.Dimensions,
		}
	}

	encoder := json.NewEncoder(f.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(jsonMetrics)
}

// formatMetricsListCSV outputs metrics list in CSV format.
func (f *Formatter) formatMetricsListCSV(metrics []cloudwatch.MetricInfo) error {
	writer := csv.NewWriter(f.writer)
	defer writer.Flush()

	if err := writer.Write([]string{"namespace", "metricName", "dimensions"}); err != nil {
		return err
	}

	for _, m := range metrics {
		dims := make([]string, 0, len(m.Dimensions))
		for k, v := range m.Dimensions {
			dims = append(dims, fmt.Sprintf("%s=%s", k, v))
		}
		record := []string{m.Namespace, m.MetricName, strings.Join(dims, ";")}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return nil
}
