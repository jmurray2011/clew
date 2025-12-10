package cmd

import (
	"os"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/output"

	"github.com/spf13/cobra"
)

var (
	groupsPrefix string
	groupsLimit  int
)

var groupsCmd = &cobra.Command{
	Use:   "groups",
	Short: "List available log groups",
	Long: `List CloudWatch log groups in the account.

Examples:
  # List all log groups
  clew groups

  # Filter by prefix
  clew groups --prefix "/aws/lambda"

  # Output as JSON
  clew groups -o json`,
	RunE: runGroups,
}

func init() {
	rootCmd.AddCommand(groupsCmd)

	groupsCmd.Flags().StringVar(&groupsPrefix, "prefix", "", "Filter log groups by prefix")
	groupsCmd.Flags().IntVarP(&groupsLimit, "limit", "l", 50, "Max log groups to return")
}

func runGroups(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	rawClient, err := cloudwatch.NewLogsClient(app.GetProfile(), app.GetRegion())
	if err != nil {
		return err
	}

	logsClient := cloudwatch.NewClient(rawClient)
	ctx := cmd.Context()

	groups, err := logsClient.ListLogGroups(ctx, groupsPrefix, groupsLimit)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(app.GetOutputFormat(), os.Stdout)
	return formatter.FormatLogGroups(groups)
}
