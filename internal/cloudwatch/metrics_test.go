package cloudwatch

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

func TestMapStatistic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected types.Statistic
	}{
		// Sum variants
		{"Sum lowercase", "sum", types.StatisticSum},
		{"Sum titlecase", "Sum", types.StatisticSum},
		{"Sum uppercase", "SUM", types.StatisticSum},

		// Average variants
		{"Average lowercase", "average", types.StatisticAverage},
		{"Average titlecase", "Average", types.StatisticAverage},
		{"Average short", "avg", types.StatisticAverage},
		{"Average short upper", "AVG", types.StatisticAverage},

		// Minimum variants
		{"Minimum lowercase", "minimum", types.StatisticMinimum},
		{"Minimum titlecase", "Minimum", types.StatisticMinimum},
		{"Minimum short", "min", types.StatisticMinimum},
		{"Minimum short upper", "MIN", types.StatisticMinimum},

		// Maximum variants
		{"Maximum lowercase", "maximum", types.StatisticMaximum},
		{"Maximum titlecase", "Maximum", types.StatisticMaximum},
		{"Maximum short", "max", types.StatisticMaximum},
		{"Maximum short upper", "MAX", types.StatisticMaximum},

		// SampleCount variants
		{"SampleCount titlecase", "SampleCount", types.StatisticSampleCount},
		{"SampleCount lowercase", "samplecount", types.StatisticSampleCount},
		{"SampleCount short", "count", types.StatisticSampleCount},
		{"SampleCount short upper", "COUNT", types.StatisticSampleCount},

		// Default case
		{"Unknown defaults to Sum", "unknown", types.StatisticSum},
		{"Empty string defaults to Sum", "", types.StatisticSum},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapStatistic(tt.input)
			if result != tt.expected {
				t.Errorf("mapStatistic(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCommonNamespaces(t *testing.T) {
	namespaces := CommonNamespaces()

	// Should return a non-empty list
	if len(namespaces) == 0 {
		t.Fatal("CommonNamespaces() returned empty list")
	}

	// Should include common AWS services
	expectedNamespaces := map[string]bool{
		"AWS/Lambda":         false,
		"AWS/EC2":            false,
		"AWS/ECS":            false,
		"AWS/ApplicationELB": false,
		"AWS/RDS":            false,
		"AWS/S3":             false,
		"AWS/DynamoDB":       false,
		"AWS/Logs":           false,
	}

	for _, ns := range namespaces {
		if _, ok := expectedNamespaces[ns]; ok {
			expectedNamespaces[ns] = true
		}
	}

	for ns, found := range expectedNamespaces {
		if !found {
			t.Errorf("CommonNamespaces() missing expected namespace %q", ns)
		}
	}

	// All namespaces should start with "AWS/"
	for _, ns := range namespaces {
		if len(ns) < 4 || ns[:4] != "AWS/" {
			t.Errorf("CommonNamespaces() contains invalid namespace %q (should start with AWS/)", ns)
		}
	}
}
