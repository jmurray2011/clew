package cloudwatch

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricsAPI wraps the CloudWatch client with convenience methods for metrics.
type MetricsAPI struct {
	client *cloudwatch.Client
}

// NewMetricsAPI creates a new MetricsAPI wrapper from an SDK client.
func NewMetricsAPI(client *cloudwatch.Client) *MetricsAPI {
	return &MetricsAPI{client: client}
}

// MetricDataPoint represents a single data point in a time series.
type MetricDataPoint struct {
	Timestamp time.Time
	Value     float64
	Unit      string
}

// MetricResult represents the result of a metric query.
type MetricResult struct {
	Namespace  string
	MetricName string
	Dimensions map[string]string
	Statistic  string
	Period     time.Duration
	DataPoints []MetricDataPoint
}

// MetricInfo represents information about an available metric.
type MetricInfo struct {
	Namespace  string
	MetricName string
	Dimensions map[string]string
}

// MetricQueryParams holds parameters for querying metrics.
type MetricQueryParams struct {
	Namespace  string
	MetricName string
	Dimensions map[string]string
	Statistic  string
	Period     time.Duration
	StartTime  time.Time
	EndTime    time.Time
}

// GetMetricStatistics queries CloudWatch for metric statistics.
func (c *MetricsAPI) GetMetricStatistics(ctx context.Context, params MetricQueryParams) (*MetricResult, error) {
	// Build dimensions
	var dims []types.Dimension
	for name, value := range params.Dimensions {
		dims = append(dims, types.Dimension{
			Name:  aws.String(name),
			Value: aws.String(value),
		})
	}

	// Map statistic string to type
	stat := mapStatistic(params.Statistic)

	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(params.Namespace),
		MetricName: aws.String(params.MetricName),
		StartTime:  aws.Time(params.StartTime),
		EndTime:    aws.Time(params.EndTime),
		Period:     aws.Int32(int32(params.Period.Seconds())),
		Statistics: []types.Statistic{stat},
	}

	if len(dims) > 0 {
		input.Dimensions = dims
	}

	result, err := c.client.GetMetricStatistics(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric statistics: %w", err)
	}

	// Convert to our result type
	metricResult := &MetricResult{
		Namespace:  params.Namespace,
		MetricName: params.MetricName,
		Dimensions: params.Dimensions,
		Statistic:  params.Statistic,
		Period:     params.Period,
		DataPoints: make([]MetricDataPoint, 0, len(result.Datapoints)),
	}

	for _, dp := range result.Datapoints {
		point := MetricDataPoint{
			Timestamp: *dp.Timestamp,
			Unit:      string(dp.Unit),
		}

		// Get the value based on statistic type
		switch stat {
		case types.StatisticSum:
			if dp.Sum != nil {
				point.Value = *dp.Sum
			}
		case types.StatisticAverage:
			if dp.Average != nil {
				point.Value = *dp.Average
			}
		case types.StatisticMinimum:
			if dp.Minimum != nil {
				point.Value = *dp.Minimum
			}
		case types.StatisticMaximum:
			if dp.Maximum != nil {
				point.Value = *dp.Maximum
			}
		case types.StatisticSampleCount:
			if dp.SampleCount != nil {
				point.Value = *dp.SampleCount
			}
		}

		metricResult.DataPoints = append(metricResult.DataPoints, point)
	}

	// Sort by timestamp
	sort.Slice(metricResult.DataPoints, func(i, j int) bool {
		return metricResult.DataPoints[i].Timestamp.Before(metricResult.DataPoints[j].Timestamp)
	})

	return metricResult, nil
}

// ListMetrics lists available metrics, optionally filtered by namespace.
func (c *MetricsAPI) ListMetrics(ctx context.Context, namespace string, metricName string) ([]MetricInfo, error) {
	var metrics []MetricInfo

	input := &cloudwatch.ListMetricsInput{}
	if namespace != "" {
		input.Namespace = aws.String(namespace)
	}
	if metricName != "" {
		input.MetricName = aws.String(metricName)
	}

	paginator := cloudwatch.NewListMetricsPaginator(c.client, input)

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list metrics: %w", err)
		}

		for _, m := range page.Metrics {
			info := MetricInfo{
				Dimensions: make(map[string]string),
			}
			if m.Namespace != nil {
				info.Namespace = *m.Namespace
			}
			if m.MetricName != nil {
				info.MetricName = *m.MetricName
			}
			for _, d := range m.Dimensions {
				if d.Name != nil && d.Value != nil {
					info.Dimensions[*d.Name] = *d.Value
				}
			}
			metrics = append(metrics, info)
		}
	}

	return metrics, nil
}

// mapStatistic converts a string statistic name to the AWS type.
func mapStatistic(s string) types.Statistic {
	switch s {
	case "Sum", "sum", "SUM":
		return types.StatisticSum
	case "Average", "average", "avg", "AVG":
		return types.StatisticAverage
	case "Minimum", "minimum", "min", "MIN":
		return types.StatisticMinimum
	case "Maximum", "maximum", "max", "MAX":
		return types.StatisticMaximum
	case "SampleCount", "samplecount", "count", "COUNT":
		return types.StatisticSampleCount
	default:
		return types.StatisticSum
	}
}

// CommonNamespaces returns a list of common AWS namespace prefixes.
func CommonNamespaces() []string {
	return []string{
		"AWS/ApplicationELB",
		"AWS/ApiGateway",
		"AWS/CloudFront",
		"AWS/DynamoDB",
		"AWS/EBS",
		"AWS/EC2",
		"AWS/ECS",
		"AWS/ELB",
		"AWS/Lambda",
		"AWS/Logs",
		"AWS/RDS",
		"AWS/S3",
		"AWS/SNS",
		"AWS/SQS",
	}
}
