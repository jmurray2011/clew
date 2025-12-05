package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize clew configuration",
	Long: `Create default configuration and history files.

Creates platform-appropriate config files:
  Linux/macOS: ~/.clew.yaml, ~/.clew_history.json
  Windows:     %USERPROFILE%\.clew.yaml, %USERPROFILE%\.clew_history.json

Examples:
  # Create default config (won't overwrite existing)
  clew init

  # Force overwrite existing config
  clew init --force`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Overwrite existing config file")
}

func runInit(cmd *cobra.Command, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	configPath := filepath.Join(home, ".clew.yaml")
	historyPath := filepath.Join(home, ".clew_history.json")

	// Generate OS-aware default config
	defaultConfig := generateDefaultConfig(home)

	// Create config file
	if err := createFileIfNotExists(configPath, defaultConfig, initForce); err != nil {
		return err
	}

	// Create empty history file
	if err := createFileIfNotExists(historyPath, "[]", initForce); err != nil {
		return err
	}

	fmt.Println("Initialized clew configuration:")
	fmt.Printf("  Config:  %s\n", configPath)
	fmt.Printf("  History: %s\n", historyPath)
	fmt.Printf("\nEdit %s to customize your settings.\n", configPath)

	return nil
}

func generateDefaultConfig(home string) string {
	// Use OS-appropriate path separators in examples
	var exampleLogGroup, historyFilePath string

	if runtime.GOOS == "windows" {
		exampleLogGroup = "/my/app/logs"
		historyFilePath = filepath.Join(home, ".clew_history.json")
	} else {
		exampleLogGroup = "/my/app/logs"
		historyFilePath = "~/.clew_history.json"
	}

	return fmt.Sprintf(`# clew configuration

# AWS settings
# profile: my-aws-profile
# region: us-east-1

# Default output format: text, json, csv
output: text

# Default log group when -g is omitted
# log_group: %s

# History settings
history_max: 50
# history_file: %s

# Aliases for frequently used log groups
# aliases:
#   app: /my/app/logs
#   waf: aws-waf-logs-MyALB

# Saved queries
# queries:
#   errors:
#     log_group: app
#     filter: "exception|error"
#     start: 2h
`, exampleLogGroup, historyFilePath)
}

func createFileIfNotExists(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("  %s already exists (use --force to overwrite)\n", path)
			return nil
		}
	}

	// Create parent directory if needed
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	fmt.Printf("  Created %s\n", path)
	return nil
}
