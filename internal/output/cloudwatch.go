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

// FormatLogGroups outputs CloudWatch log group information in the configured format.
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

func (f *Formatter) formatGroupsText(groups []cloudwatch.LogGroupInfo) error {
	if len(groups) == 0 {
		_, _ = fmt.Fprintln(f.writer, ui.MutedStyle.Render("No log groups found."))
		return nil
	}

	for _, g := range groups {
		_, _ = fmt.Fprintln(f.writer, ui.SuccessStyle.Render(g.Name))

		_, _ = fmt.Fprint(f.writer, ui.MutedStyle.Render("  Size: "))
		_, _ = fmt.Fprint(f.writer, timeutil.FormatBytes(g.StoredBytes))

		if g.RetentionDays > 0 {
			_, _ = fmt.Fprintf(f.writer, "  |  Retention: %d days", g.RetentionDays)
		} else {
			_, _ = fmt.Fprint(f.writer, "  |  Retention: Never expire")
		}

		if !g.CreationTime.IsZero() {
			_, _ = fmt.Fprintf(f.writer, "  |  Created: %s", g.CreationTime.Format("2006-01-02"))
		}

		_, _ = fmt.Fprintln(f.writer)
	}

	return nil
}

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

// FormatMetricResult outputs CloudWatch metric query results in the configured format.
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

func (f *Formatter) formatMetricResultText(result *cloudwatch.MetricResult) error {
	_, _ = fmt.Fprintf(f.writer, "%s/%s (%s)\n",
		ui.LabelStyle.Render(result.Namespace),
		ui.SuccessStyle.Render(result.MetricName),
		result.Statistic)

	if len(result.Dimensions) > 0 {
		dims := make([]string, 0, len(result.Dimensions))
		for k, v := range result.Dimensions {
			dims = append(dims, fmt.Sprintf("%s=%s", k, v))
		}
		_, _ = fmt.Fprintf(f.writer, "Dimensions: %s\n", strings.Join(dims, ", "))
	}
	_, _ = fmt.Fprintln(f.writer)

	maxVal := 0.0
	for _, dp := range result.DataPoints {
		if dp.Value > maxVal {
			maxVal = dp.Value
		}
	}

	barWidth := 40
	for _, dp := range result.DataPoints {
		ts := ui.TimestampStyle.Render(dp.Timestamp.Format("2006-01-02 15:04"))

		var valStr string
		if dp.Value == float64(int64(dp.Value)) {
			valStr = fmt.Sprintf("%8.0f", dp.Value)
		} else {
			valStr = fmt.Sprintf("%8.2f", dp.Value)
		}

		barLen := 0
		if maxVal > 0 {
			barLen = int((dp.Value / maxVal) * float64(barWidth))
		}
		bar := strings.Repeat("█", barLen)

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

		_, _ = fmt.Fprintf(f.writer, "%s  %s  %s\n", ts, valStr, bar)
	}

	return nil
}

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

// FormatMetricsList outputs a list of available CloudWatch metrics.
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

func (f *Formatter) formatMetricsListText(metrics []cloudwatch.MetricInfo) error {
	for _, m := range metrics {
		_, _ = fmt.Fprintln(f.writer, ui.SuccessStyle.Render(m.MetricName))
		if len(m.Dimensions) > 0 {
			for k, v := range m.Dimensions {
				_, _ = fmt.Fprintf(f.writer, "  %s: %s\n", ui.MutedStyle.Render(k), v)
			}
		}
	}
	return nil
}

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
