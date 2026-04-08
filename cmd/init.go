package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmurray2011/clew/internal/config"

	"github.com/spf13/cobra"
)

var (
	initForce bool
	initFrom  string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config",
	Long: `Initialize clew configuration.

Creates ~/.clew/config.yaml with a profile-only template.
Use --from to bootstrap from a team config.

Examples:
  clew init
  clew init --from https://github.com/myteam/dotfiles/raw/main/clew/config.yaml
  clew init --from ./team-config.yaml`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config file")
	initCmd.Flags().StringVar(&initFrom, "from", "", "URL or local path to copy config from")
}

func runInit(cmd *cobra.Command, args []string) error {
	app := GetApp(cmd)
	configPath := config.Path()
	if configPath == "" {
		return fmt.Errorf("could not determine home directory")
	}

	if !initForce {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", configPath)
		}
	}

	var content string
	if initFrom != "" {
		data, err := fetchConfig(initFrom)
		if err != nil {
			return fmt.Errorf("failed to fetch config: %w", err)
		}
		content = data
	} else {
		content = defaultConfigTemplate()
	}

	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	app.Render.Success("Created %s", configPath)

	cfg, err := config.Load()
	if err == nil {
		printConfigSummary(app, cfg)
	}

	return nil
}

func fetchConfig(from string) (string, error) {
	if strings.HasPrefix(from, "http://") || strings.HasPrefix(from, "https://") {
		resp, err := http.Get(from)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}

	data, err := os.ReadFile(from)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func printConfigSummary(app *App, cfg *config.Config) {
	app.Render.Newline()

	if len(cfg.Profiles) > 0 {
		app.Render.Info("  Profiles:")
		for short, awsProfile := range cfg.Profiles {
			marker := ""
			if short == cfg.Default {
				marker = " (default)"
			}
			app.Render.Info("    %-12s → %s%s", short, awsProfile, marker)
		}
	}

	if cfg.Region != "" {
		app.Render.Info("  Region:   %s", cfg.Region)
	}

	app.Render.Newline()
	if cfg.Default != "" {
		app.Render.Info("  Try: clew groups -e %s", cfg.Default)
	} else {
		app.Render.Info("  Try: clew groups -e <profile>")
	}
}

func defaultConfigTemplate() string {
	return `# clew configuration
# https://github.com/jmurray2011/clew
#
# Map short names to AWS profiles. Use -e to select:
#   clew query -e prod /app/api/prod -s 1h -f "error"

profiles:
  # test: test_power
  # prod: prod_power
  # demo: demo_power

# Default profile when -e is omitted
# default: test

# Default region (can override with -r)
# region: us-east-1
`
}
