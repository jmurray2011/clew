package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/jmurray2011/clew/internal/source"
	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
)

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List configured source aliases",
	Long: `List source aliases defined in the configuration file.

Source aliases can be defined in ~/.clew/config.yaml:

  sources:
    prod-api:
      uri: cloudwatch:///app/api/prod?profile=prod&region=us-east-1
    staging:
      uri: cloudwatch:///app/api/staging?profile=staging
    local:
      uri: file:///var/log/app.log
      format: java

Use aliases with @ prefix in commands:
  clew query @prod-api -s 1h -f "error"
  clew tail @staging -f "exception"`,
	RunE: runSources,
}

func init() {
	rootCmd.AddCommand(sourcesCmd)
}

func runSources(cmd *cobra.Command, args []string) error {
	cfg, err := source.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if len(cfg.Sources) == 0 {
		render.Info("No source aliases configured.")
		render.Newline()
		render.Info("Create aliases in %s:", source.ConfigPath())
		fmt.Println()
		fmt.Println("  sources:")
		fmt.Println("    prod-api:")
		fmt.Println("      uri: cloudwatch:///app/api/prod?profile=prod&region=us-east-1")
		fmt.Println("    local:")
		fmt.Println("      uri: file:///var/log/app.log")
		return nil
	}

	// Sort alias names for consistent output
	names := make([]string, 0, len(cfg.Sources))
	for name := range cfg.Sources {
		names = append(names, name)
	}
	sort.Strings(names)

	// Find max name length for alignment
	maxLen := 0
	for _, name := range names {
		if len(name) > maxLen {
			maxLen = len(name)
		}
	}

	for _, name := range names {
		s := cfg.Sources[name]
		aliasName := ui.LabelStyle.Render(fmt.Sprintf("@%-*s", maxLen, name))
		fmt.Fprintf(os.Stdout, "%s  %s", aliasName, s.URI)
		if s.Format != "" {
			fmt.Fprintf(os.Stdout, "  [format: %s]", s.Format)
		}
		fmt.Fprintln(os.Stdout)
	}

	if cfg.DefaultSource != "" {
		render.Newline()
		render.Info("Default source: @%s", cfg.DefaultSource)
	}

	return nil
}
