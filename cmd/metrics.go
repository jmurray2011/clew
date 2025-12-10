package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"
	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
)

var (
	metricsNamespace  string
	metricsMetricName string
	metricsDimensions []string
	metricsStatistic  string
	metricsPeriod     string
	metricsStartTime  string
	metricsEndTime    string
	metricsList       bool
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Query CloudWatch Metrics",
	Long: `Query CloudWatch Metrics to identify spikes and anomalies.

Use this to find interesting time periods, then use 'clew around' to
investigate logs at those times.

Examples:
  # Query ALB 5XX errors over 24h
  clew metrics -n AWS/ApplicationELB -m HTTPCode_ELB_5XX_Count -s 24h

  # With specific period
  clew metrics -n AWS/ApplicationELB -m HTTPCode_ELB_5XX_Count -s 24h --period 5m

  # With dimensions (filter to specific resource)
  clew metrics -n AWS/ApplicationELB -m HTTPCode_ELB_5XX_Count \
    -d LoadBalancer=app/my-alb/abc123 -s 24h

  # Lambda errors
  clew metrics -n AWS/Lambda -m Errors --stat Sum -s 7d --period 1h

  # List available metrics in a namespace
  clew metrics --list -n AWS/ApplicationELB

  # List all namespaces with metrics
  clew metrics --list`,
	RunE: runMetrics,
}

func init() {
	rootCmd.AddCommand(metricsCmd)

	metricsCmd.Flags().StringVarP(&metricsNamespace, "namespace", "n", "", "CloudWatch namespace (e.g., AWS/ApplicationELB)")
	metricsCmd.Flags().StringVarP(&metricsMetricName, "metric", "m", "", "Metric name (e.g., HTTPCode_ELB_5XX_Count)")
	metricsCmd.Flags().StringArrayVarP(&metricsDimensions, "dimension", "d", nil, "Dimension filter (Name=Value), can be repeated")
	metricsCmd.Flags().StringVar(&metricsStatistic, "stat", "Sum", "Statistic: Sum, Average, Minimum, Maximum, SampleCount")
	metricsCmd.Flags().StringVar(&metricsPeriod, "period", "5m", "Period for data points (e.g., 1m, 5m, 1h)")
	metricsCmd.Flags().StringVarP(&metricsStartTime, "start", "s", "1h", "Start time (duration like 1h, 24h, 7d or RFC3339)")
	metricsCmd.Flags().StringVar(&metricsEndTime, "end", "", "End time (default: now)")
	metricsCmd.Flags().BoolVar(&metricsList, "list", false, "List available metrics instead of querying")
}

func runMetrics(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	rawClient, err := cloudwatch.NewMetricsClient(app.GetProfile(), app.GetRegion())
	if err != nil {
		return err
	}

	metricsClient := cloudwatch.NewMetricsAPI(rawClient)
	ctx := cmd.Context()

	// List mode
	if metricsList {
		return listMetrics(ctx, app, metricsClient)
	}

	// Query mode requires namespace and metric
	if metricsNamespace == "" {
		return fmt.Errorf("--namespace (-n) is required for metric queries")
	}
	if metricsMetricName == "" {
		return fmt.Errorf("--metric (-m) is required for metric queries")
	}

	return queryMetrics(ctx, app, metricsClient)
}

func listMetrics(ctx context.Context, app *App, client *cloudwatch.MetricsAPI) error {
	app.Render.Status("Listing metrics...")

	metrics, err := client.ListMetrics(ctx, metricsNamespace, metricsMetricName)
	if err != nil {
		return err
	}

	if len(metrics) == 0 {
		app.Render.Info("No metrics found.")
		return nil
	}

	// Group by namespace if not filtered
	if metricsNamespace == "" {
		return formatMetricsByNamespace(metrics)
	}

	// Show metrics with dimensions
	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	return formatter.FormatMetricsList(metrics)
}

func formatMetricsByNamespace(metrics []cloudwatch.MetricInfo) error {
	// Group by namespace
	namespaces := make(map[string][]string)
	for _, m := range metrics {
		if _, ok := namespaces[m.Namespace]; !ok {
			namespaces[m.Namespace] = []string{}
		}
		// Add unique metric names
		found := false
		for _, name := range namespaces[m.Namespace] {
			if name == m.MetricName {
				found = true
				break
			}
		}
		if !found {
			namespaces[m.Namespace] = append(namespaces[m.Namespace], m.MetricName)
		}
	}

	// Print grouped
	for ns, metricNames := range namespaces {
		fmt.Println(ui.LabelStyle.Render(ns))
		for _, name := range metricNames {
			fmt.Printf("  %s\n", name)
		}
		fmt.Println()
	}

	return nil
}

func queryMetrics(ctx context.Context, app *App, client *cloudwatch.MetricsAPI) error {
	// Parse time range
	startTime, endTime, err := parseMetricsTimeRange(metricsStartTime, metricsEndTime)
	if err != nil {
		return err
	}

	// Parse period
	period, err := parseDuration(metricsPeriod)
	if err != nil {
		return fmt.Errorf("invalid period: %w", err)
	}

	// Parse dimensions
	dimensions := make(map[string]string)
	for _, d := range metricsDimensions {
		parts := strings.SplitN(d, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid dimension format: %s (expected Name=Value)", d)
		}
		dimensions[parts[0]] = parts[1]
	}

	app.Render.Status("Querying %s/%s...", metricsNamespace, metricsMetricName)

	params := cloudwatch.MetricQueryParams{
		Namespace:  metricsNamespace,
		MetricName: metricsMetricName,
		Dimensions: dimensions,
		Statistic:  metricsStatistic,
		Period:     period,
		StartTime:  startTime,
		EndTime:    endTime,
	}

	result, err := client.GetMetricStatistics(ctx, params)
	if err != nil {
		return err
	}

	if len(result.DataPoints) == 0 {
		app.Render.Info("No data points found for the specified time range.")
		return nil
	}

	// Format and display results
	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	return formatter.FormatMetricResult(result)
}

func parseMetricsTimeRange(startStr, endStr string) (time.Time, time.Time, error) {
	now := time.Now()
	endTime := now

	// Parse end time if specified
	if endStr != "" {
		t, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end time: %w", err)
		}
		endTime = t
	}

	// Parse start time
	var startTime time.Time

	// Try as duration first
	if d, err := parseDuration(startStr); err == nil {
		startTime = endTime.Add(-d)
	} else {
		// Try as RFC3339
		t, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start time: %s (use duration like 1h, 24h, 7d or RFC3339 format)", startStr)
		}
		startTime = t
	}

	return startTime, endTime, nil
}

// parseDuration parses a duration string like "1h", "24h", "7d", "5m"
func parseDuration(s string) (time.Duration, error) {
	// Handle days
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		var d int
		if _, err := fmt.Sscanf(days, "%d", &d); err != nil {
			return 0, err
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
