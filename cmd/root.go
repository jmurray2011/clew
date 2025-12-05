package cmd

import (
	"fmt"
	"os"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	_ "github.com/jmurray2011/clew/internal/local" // Register file:// source
	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	profile      string
	region       string
	outputFormat string
	cfgFile      string
	verbose      bool
	noColor      bool
	quiet        bool

	// render is the global renderer for all output
	render *ui.Renderer
)

var rootCmd = &cobra.Command{
	Use:   "clew",
	Short: "Follow the thread through your logs",
	Long: `clew - a ball of thread; from Greek mythology, the thread Ariadne gave
Theseus to escape the Minotaur's labyrinth. Follow the clew through your logs.

A CLI tool for querying logs from multiple sources.
Supports CloudWatch Logs, local files, and more.

Source URIs:
  cloudwatch:///log-group?profile=x&region=y   AWS CloudWatch Logs
  file:///path/to/file.log                     Local file
  /var/log/app.log                             Local file (shorthand)
  @alias-name                                  Config alias

Configuration:
  Create ~/.clew/config.yaml to define source aliases:

    sources:
      prod-api:
        uri: cloudwatch:///app/api/prod?profile=prod&region=us-east-1
      staging:
        uri: cloudwatch:///app/api/staging?profile=staging
      local:
        uri: file:///var/log/app.log
        format: java

    default_source: prod-api

    output:
      format: text      # text, json, csv
      timestamps: local # local, utc

Examples:
  # Query CloudWatch Logs
  clew query "cloudwatch:///app/logs?profile=prod" -s 2h -f "error"

  # Query local file
  clew query /var/log/app.log -f "exception"

  # Use a config alias
  clew query @prod-api -s 1h -f "timeout"

  # Tail logs in real-time
  clew tail @prod-api -f "error"

  # List log streams
  clew streams @prod-api`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// SetVersion sets the version string for the root command
func SetVersion(v string) {
	rootCmd.Version = v
}

func init() {
	cobra.OnInitialize(initConfig, initRenderer)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ~/.clew/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&profile, "profile", "p", "", "Default AWS profile (can be overridden in URI)")
	rootCmd.PersistentFlags().StringVarP(&region, "region", "r", "", "Default AWS region (can be overridden in URI)")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: text, json, csv")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output for debugging")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Suppress status messages")

	// Bind flags to viper
	_ = viper.BindPFlag("profile", rootCmd.PersistentFlags().Lookup("profile"))
	_ = viper.BindPFlag("region", rootCmd.PersistentFlags().Lookup("region"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initRenderer initializes the global renderer with current settings.
func initRenderer() {
	render = ui.NewRendererWithOptions(
		ui.WithNoColor(noColor || os.Getenv("NO_COLOR") != ""),
		ui.WithQuiet(quiet),
	)
}

// IsVerbose returns true if verbose mode is enabled
func IsVerbose() bool {
	return verbose || viper.GetBool("verbose")
}

// Debugf prints a debug message if verbose mode is enabled
func Debugf(format string, args ...interface{}) {
	if IsVerbose() {
		render.Debug(format, args...)
	}
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			viper.AddConfigPath(home)
			// Also check ~/.clew/ directory
			viper.AddConfigPath(home + "/.clew")
		}
		viper.AddConfigPath(".")
		viper.SetConfigName(".clew")
		viper.SetConfigType("yaml")
	}

	// Environment variables
	viper.SetEnvPrefix("CLEW")
	viper.AutomaticEnv()

	// Defaults
	viper.SetDefault("region", "us-east-1")
	viper.SetDefault("output", "text")
	viper.SetDefault("history_max", 50)
	// history_file defaults to ~/.clew_history.json (handled in history.go)

	// Read config file (ignore if not found, warn on other errors)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config file: %v\n", err)
		}
	}
}

// getProfile returns the AWS profile from flags or config.
func getProfile() string {
	if profile != "" {
		return profile
	}
	return viper.GetString("profile")
}

// accountIDCache caches AWS account IDs by profile to avoid repeated STS calls.
var accountIDCache = make(map[string]string)

// getAccountID returns the AWS account ID for the current profile.
// Results are cached to avoid repeated API calls during a session.
func getAccountID() string {
	profile := getProfile()

	// Check cache first
	if id, ok := accountIDCache[profile]; ok {
		return id
	}

	// Fetch from AWS
	id, err := cloudwatch.GetAccountID(profile, getRegion())
	if err != nil {
		Debugf("Failed to get account ID: %v", err)
		return ""
	}

	// Cache for future use
	accountIDCache[profile] = id
	return id
}

// getRegion returns the AWS region from flags or config.
func getRegion() string {
	if region != "" {
		return region
	}
	return viper.GetString("region")
}

// getOutputFormat returns the output format from flags or config.
func getOutputFormat() string {
	if outputFormat != "" {
		return outputFormat
	}
	return viper.GetString("output")
}

// getDefaultLogGroup returns the default log group from config.
// Deprecated: Use source URIs or @aliases instead.
func getDefaultLogGroup() string {
	return viper.GetString("log_group")
}

// resolveLogGroup resolves a log group name, checking aliases first.
// Deprecated: Use source URIs or @aliases instead.
func resolveLogGroup(input string) string {
	if input == "" {
		return ""
	}

	// Check if it's an alias
	aliases := viper.GetStringMapString("aliases")
	if fullPath, ok := aliases[input]; ok {
		return fullPath
	}

	return input
}
