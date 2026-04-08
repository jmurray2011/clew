package cmd

import (
	"fmt"
	"os"

	"github.com/jmurray2011/clew/internal/ui"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	env          string
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
	Short: "A fast, focused CLI for querying AWS CloudWatch Logs.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		app := NewApp()
		ctx := SetApp(cmd.Context(), app)
		cmd.SetContext(ctx)
	},
	Long: `clew - a fast, focused CLI for querying AWS CloudWatch Logs.

Log groups are discovered live from AWS — no aliases to configure.

The first argument is a log group specifier:
  /app/api/prod                Exact log group name
  "docs.*error"                Search pattern (regex)
  ?                            Interactive picker
  (omitted)                    Frecency prompt — shows your recent groups

Configuration (~/.clew/config.yaml) is just profile shortcuts:

  profiles:
    test: test_profile
    prod: prod_profile
  default: test

Use -e to select an environment:
  clew query -e prod /app/api/prod -s 2h -f "error"

Examples:
  clew query -e test "docs.*error" -s 1h -f "error"
  clew tail -e prod /app/api/prod -f "exception"
  clew streams -e test ? --stale 5m
  clew groups -e prod --prefix "/aws/lambda"`,
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
	rootCmd.PersistentFlags().StringVarP(&env, "env", "e", "", "Environment/profile (maps to profiles in config)")
	rootCmd.PersistentFlags().StringVarP(&region, "region", "r", "", "AWS region override")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: text, json, csv")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Suppress status messages")

	_ = viper.BindPFlag("region", rootCmd.PersistentFlags().Lookup("region"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))
	_ = viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	_ = rootCmd.RegisterFlagCompletionFunc("output", completeOutputFormat)
}

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
			configPath := home + "/.clew/config.yaml"
			if _, err := os.Stat(configPath); err == nil {
				viper.SetConfigFile(configPath)
			} else {
				viper.AddConfigPath(home)
				viper.AddConfigPath(home + "/.clew")
				viper.AddConfigPath(".")
				viper.SetConfigName(".clew")
				viper.SetConfigType("yaml")
			}
		}
	}

	viper.SetEnvPrefix("CLEW")
	viper.AutomaticEnv()

	viper.SetDefault("region", "us-east-1")
	viper.SetDefault("output", "text")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "Warning: error reading config file: %v\n", err)
		}
	}
}

func getRegion() string {
	if region != "" {
		return region
	}
	return viper.GetString("region")
}

func getOutputFormat() string {
	if outputFormat != "" {
		return outputFormat
	}
	return viper.GetString("output")
}
