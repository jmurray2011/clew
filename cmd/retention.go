package cmd

import (
	"os"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"

	"github.com/spf13/cobra"
)

var (
	retentionLogGroup string
)

var retentionCmd = &cobra.Command{
	Use:   "retention",
	Short: "View log group retention settings",
	Long: `View retention settings for CloudWatch log groups.

Without arguments, lists all log groups with their retention settings.
With -g, shows a specific log group's retention.

Examples:
  # List all log groups with retention info
  clew retention

  # Show retention for a specific log group
  clew retention -g "/app/logs"`,
	RunE: runRetention,
}

func init() {
	rootCmd.AddCommand(retentionCmd)

	retentionCmd.Flags().StringVarP(&retentionLogGroup, "log-group", "g", "", "Log group name")
}

func runRetention(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	rawClient, err := cloudwatch.NewLogsClient(app.GetProfile(), app.GetRegion())
	if err != nil {
		return err
	}

	logsClient := cloudwatch.NewClient(rawClient)
	ctx := cmd.Context()

	// Resolve alias if provided
	lg := retentionLogGroup
	if lg != "" {
		lg = resolveLogGroup(lg)
	}

	// List groups with retention info
	var groups []cloudwatch.LogGroupInfo
	if lg != "" {
		// Get specific group
		group, err := logsClient.GetLogGroup(ctx, lg)
		if err != nil {
			return err
		}
		groups = []cloudwatch.LogGroupInfo{group}
	} else {
		// List all groups
		groups, err = logsClient.ListLogGroups(ctx, "", 100)
		if err != nil {
			return err
		}
	}

	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	return formatter.FormatLogGroups(groups)
}
