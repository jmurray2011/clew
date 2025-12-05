package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
)

var (
	fieldsLogGroup string
	fieldsSample   int
	fieldsStart    string
	fieldsEnd      string
)

var fieldsCmd = &cobra.Command{
	Use:   "fields",
	Short: "Discover available fields in a log group",
	Long: `Query a log group to discover what fields are available.

Useful for structured logs (WAF, VPC Flow Logs, JSON application logs)
where you need to know what fields can be queried.

Examples:
  # Discover fields in WAF logs
  clew fields -g "aws-waf-logs-MyALB"

  # Use an alias
  clew fields -g waf

  # Sample more records for better field coverage
  clew fields -g waf --sample 100

  # Look back further if no recent data
  clew fields -g api -s 7d

  # Query a specific historical window
  clew fields -g api -s 2023-11-01T00:00:00Z -e 2023-11-30T23:59:59Z`,
	RunE: runFields,
}

func init() {
	rootCmd.AddCommand(fieldsCmd)

	fieldsCmd.Flags().StringVarP(&fieldsLogGroup, "log-group", "g", "", "Log group name (required)")
	fieldsCmd.Flags().IntVar(&fieldsSample, "sample", 20, "Number of records to sample")
	fieldsCmd.Flags().StringVarP(&fieldsStart, "start", "s", "1h", "Start time - relative (1h, 7d) or RFC3339")
	fieldsCmd.Flags().StringVarP(&fieldsEnd, "end", "e", "now", "End time - relative or RFC3339")
}

func runFields(cmd *cobra.Command, args []string) error {
	lg := fieldsLogGroup
	if lg == "" {
		lg = getDefaultLogGroup()
	} else {
		lg = resolveLogGroup(lg)
	}
	if lg == "" {
		return fmt.Errorf("--log-group is required")
	}

	rawClient, err := cloudwatch.NewLogsClient(getProfile(), getRegion())
	if err != nil {
		return err
	}

	logsClient := cloudwatch.NewClient(rawClient)
	ctx := context.Background()

	// Parse time range
	startParsed, err := parseTime(fieldsStart)
	if err != nil {
		return fmt.Errorf("invalid start time %q: %w", fieldsStart, err)
	}
	endParsed, err := parseTime(fieldsEnd)
	if err != nil {
		return fmt.Errorf("invalid end time %q: %w", fieldsEnd, err)
	}

	if fieldsEnd == "now" {
		render.Status("Sampling %d records from %s (last %s)...", fieldsSample, lg, fieldsStart)
	} else {
		render.Status("Sampling %d records from %s (%s to %s)...", fieldsSample, lg,
			startParsed.Format("2006-01-02 15:04"), endParsed.Format("2006-01-02 15:04"))
	}
	render.Newline()

	// Query to get sample records - CloudWatch automatically includes discovered fields
	// For JSON logs, it auto-parses and returns all fields
	query := fmt.Sprintf("limit %d", fieldsSample)

	results, err := logsClient.RunInsightsQuery(ctx, cloudwatch.QueryParams{
		LogGroup:  lg,
		StartTime: startParsed,
		EndTime:   endParsed,
		Query:     query,
		Limit:     fieldsSample,
	})
	if err != nil {
		return fmt.Errorf("failed to query fields: %w", err)
	}

	if len(results) == 0 {
		render.Warning("No records found in the last %s", fieldsStart)
		render.Info("Try a longer time range (-s 7d) or check if the log group has data.")
		return nil
	}

	// Collect all unique field names and sample values
	fieldInfo := make(map[string]fieldStats)
	jsonFieldInfo := make(map[string]fieldStats)

	for _, result := range results {
		for name, value := range result.Fields {
			stats := fieldInfo[name]
			stats.count++
			if stats.sampleValue == "" && value != "" {
				stats.sampleValue = truncateValue(value, 60)
			}
			fieldInfo[name] = stats

			// Try to parse JSON from @message
			if name == "@message" && strings.HasPrefix(strings.TrimSpace(value), "{") {
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(value), &jsonData); err == nil {
					jsonFields := extractJSONFields(jsonData, "")
					for fieldPath, sampleVal := range jsonFields {
						jstats := jsonFieldInfo[fieldPath]
						jstats.count++
						if jstats.sampleValue == "" && sampleVal != "" {
							jstats.sampleValue = truncateValue(sampleVal, 60)
						}
						jsonFieldInfo[fieldPath] = jstats
					}
				}
			}
		}
	}

	// Sort fields by name
	var fieldNames []string
	for name := range fieldInfo {
		fieldNames = append(fieldNames, name)
	}
	sort.Strings(fieldNames)

	// Separate system fields (@) from custom fields
	var systemFields, customFields []string
	for _, name := range fieldNames {
		if strings.HasPrefix(name, "@") {
			systemFields = append(systemFields, name)
		} else {
			customFields = append(customFields, name)
		}
	}

	// Print system fields
	if len(systemFields) > 0 {
		render.Section("System Fields")
		for _, name := range systemFields {
			stats := fieldInfo[name]
			fieldName := ui.LabelStyle.Render(fmt.Sprintf("  %-30s", name))
			countInfo := ui.MutedStyle.Render(fmt.Sprintf(" (%d/%d)", stats.count, len(results)))
			sampleVal := ""
			if stats.sampleValue != "" {
				sampleVal = ui.MutedStyle.Render("  " + stats.sampleValue)
			}
			fmt.Printf("%s%s%s\n", fieldName, countInfo, sampleVal)
		}
	}

	// Print custom fields
	if len(customFields) > 0 {
		render.Divider()
		render.Section("Custom Fields")
		for _, name := range customFields {
			stats := fieldInfo[name]
			fieldName := ui.LabelStyle.Render(fmt.Sprintf("  %-30s", name))
			countInfo := ui.MutedStyle.Render(fmt.Sprintf(" (%d/%d)", stats.count, len(results)))
			sampleVal := ""
			if stats.sampleValue != "" {
				sampleVal = ui.MutedStyle.Render("  " + stats.sampleValue)
			}
			fmt.Printf("%s%s%s\n", fieldName, countInfo, sampleVal)
		}
	}

	// Print JSON fields extracted from @message
	if len(jsonFieldInfo) > 0 {
		var jsonFieldNames []string
		for name := range jsonFieldInfo {
			jsonFieldNames = append(jsonFieldNames, name)
		}
		sort.Strings(jsonFieldNames)

		render.Divider()
		render.Section("JSON Fields (from @message)")
		for _, name := range jsonFieldNames {
			stats := jsonFieldInfo[name]
			fieldName := ui.LabelStyle.Render(fmt.Sprintf("  %-40s", name))
			countInfo := ui.MutedStyle.Render(fmt.Sprintf(" (%d/%d)", stats.count, len(results)))
			sampleVal := ""
			if stats.sampleValue != "" {
				sampleVal = ui.MutedStyle.Render("  " + stats.sampleValue)
			}
			fmt.Printf("%s%s%s\n", fieldName, countInfo, sampleVal)
		}

		render.Newline()
		render.Info("Found %d system/custom fields + %d JSON fields across %d sampled records.", len(fieldNames), len(jsonFieldNames), len(results))
	} else {
		render.Newline()
		render.Info("Found %d fields across %d sampled records.", len(fieldNames), len(results))
	}

	return nil
}

type fieldStats struct {
	count       int
	sampleValue string
}

// extractJSONFields recursively extracts field paths from a JSON object.
// Returns a map of field paths (e.g., "httpRequest.clientIp") to sample values.
func extractJSONFields(data map[string]interface{}, prefix string) map[string]string {
	result := make(map[string]string)

	for key, value := range data {
		fieldPath := key
		if prefix != "" {
			fieldPath = prefix + "." + key
		}

		switch v := value.(type) {
		case map[string]interface{}:
			// Recurse into nested objects
			nested := extractJSONFields(v, fieldPath)
			for k, val := range nested {
				result[k] = val
			}
		case []interface{}:
			// For arrays, note the type and optionally peek at first element
			if len(v) > 0 {
				if nested, ok := v[0].(map[string]interface{}); ok {
					// Array of objects - extract fields with [] notation
					nestedFields := extractJSONFields(nested, fieldPath+"[]")
					for k, val := range nestedFields {
						result[k] = val
					}
				} else {
					result[fieldPath+"[]"] = formatSampleValue(v[0])
				}
			} else {
				result[fieldPath+"[]"] = "(empty array)"
			}
		default:
			result[fieldPath] = formatSampleValue(v)
		}
	}

	return result
}

func formatSampleValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func truncateValue(s string, maxLen int) string {
	// Remove newlines
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func parseTime(s string) (time.Time, error) {
	return cloudwatch.ParseTime(s)
}
