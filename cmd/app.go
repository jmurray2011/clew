package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/config"
	"github.com/jmurray2011/clew/internal/discovery"
	"github.com/jmurray2011/clew/internal/frecency"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

type appContextKey struct{}

// App holds the application dependencies.
type App struct {
	Config         AppConfig
	Render         *ui.Renderer
	Cfg            *config.Config
	AccountIDCache map[string]string
}

// AppConfig holds CLI-level configuration values.
type AppConfig struct {
	Env          string // -e flag value
	Region       string
	OutputFormat string
	Verbose      bool
	NoColor      bool
	Quiet        bool
}

// NewApp creates a new App with default configuration from viper.
func NewApp() *App {
	cfg := AppConfig{
		Env:          env,
		Region:       getRegion(),
		OutputFormat: getOutputFormat(),
		Verbose:      IsVerbose(),
		NoColor:      noColor,
		Quiet:        quiet,
	}

	fileCfg, err := config.Load()
	if err != nil {
		if render != nil {
			render.Debug("Failed to load config: %v", err)
		}
		fileCfg = &config.Config{Profiles: make(map[string]string)}
	}

	return &App{
		Config:         cfg,
		Render:         render,
		Cfg:            fileCfg,
		AccountIDCache: make(map[string]string),
	}
}

// GetApp retrieves the App from the command context.
func GetApp(cmd *cobra.Command) *App {
	if app, ok := cmd.Context().Value(appContextKey{}).(*App); ok {
		return app
	}
	return NewApp()
}

// SetApp stores the App in the context for a command.
func SetApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appContextKey{}, app)
}

// Debugf prints a debug message if verbose mode is enabled.
func (a *App) Debugf(format string, args ...interface{}) {
	if a.Config.Verbose || viper.GetBool("verbose") {
		a.Render.Debug(format, args...)
	}
}

// GetProfile resolves the AWS profile from -e flag → config profiles map → pass-through.
func (a *App) GetProfile() string {
	return a.Cfg.ResolveProfile(a.Config.Env)
}

// GetRegion returns the region from -r flag, config, or viper default.
func (a *App) GetRegion() string {
	if a.Config.Region != "" {
		return a.Config.Region
	}
	if a.Cfg.Region != "" {
		return a.Cfg.Region
	}
	return viper.GetString("region")
}

// GetOutputFormat returns the output format.
func (a *App) GetOutputFormat() string {
	if a.Config.OutputFormat != "" {
		return a.Config.OutputFormat
	}
	return viper.GetString("output")
}

// ResolveLogGroup resolves the positional source argument to a concrete log group name.
// Flow: exact match → search pattern → frecency prompt.
// If sourceArg is "?", shows the interactive picker.
// If sourceArg is "", shows frecency suggestions.
func (a *App) ResolveLogGroup(ctx context.Context, sourceArg string) (string, error) {
	profile := a.GetProfile()
	region := a.GetRegion()

	// Interactive picker
	if sourceArg == "?" {
		a.Render.Status("Loading log groups...")
		groups, err := discovery.ListGroups(ctx, profile, region)
		if err != nil {
			return "", fmt.Errorf("failed to list log groups: %w", err)
		}
		return discovery.PickGroup(groups, "Log groups")
	}

	// No source specified — frecency prompt
	if sourceArg == "" {
		return a.frecencyPrompt(profile)
	}

	// Try exact match first
	exact, err := discovery.IsExactGroup(ctx, profile, region, sourceArg)
	if err != nil {
		// If discovery fails (e.g., no creds), treat as literal
		a.Debugf("Discovery failed, treating as literal: %v", err)
		return sourceArg, nil
	}
	if exact {
		return sourceArg, nil
	}

	// Not exact — treat as search pattern
	a.Render.Status("Searching for log groups matching %q...", sourceArg)
	matches, err := discovery.SearchGroups(ctx, profile, region, sourceArg)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("no log groups matching %q (use 'clew groups -e %s' to list available groups)", sourceArg, a.Config.Env)
	}

	if len(matches) == 1 {
		a.Render.Info("Matched: %s", matches[0])
		return matches[0], nil
	}

	// Multiple matches — interactive pick
	return discovery.PickGroup(matches, fmt.Sprintf("Log groups matching %q", sourceArg))
}

// frecencyPrompt shows recent log groups and prompts for selection.
func (a *App) frecencyPrompt(profile string) (string, error) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "", fmt.Errorf("log group required (cannot prompt in non-interactive context)")
	}

	suggestions := frecency.Suggest(profile, 10)
	if len(suggestions) == 0 {
		return "", fmt.Errorf("no log group specified and no history for profile %q\n\nUsage:\n  clew query <log-group> -s 1h -f \"error\"\n  clew query ? -s 1h -f \"error\"  (interactive picker)", profile)
	}

	groups := make([]string, len(suggestions))
	labels := make([]string, len(suggestions))
	for i, s := range suggestions {
		groups[i] = s.LogGroup
		labels[i] = fmt.Sprintf("%-50s (%s, %d queries)", s.LogGroup, frecency.FormatSuggestion(s), s.Count)
	}

	return discovery.PickGroup(groups, fmt.Sprintf("Recent log groups for %s", profile))
}

// OpenSource resolves a log group and opens a CloudWatch source.
// Also records the query in frecency history.
func (a *App) OpenSource(ctx context.Context, sourceArg string) (*cloudwatch.Source, string, error) {
	logGroup, err := a.ResolveLogGroup(ctx, sourceArg)
	if err != nil {
		return nil, "", err
	}

	profile := a.GetProfile()
	region := a.GetRegion()

	src, err := cloudwatch.NewSource(logGroup, profile, region)
	if err != nil {
		return nil, "", err
	}

	// Record in frecency (fire and forget)
	_ = frecency.Record(profile, logGroup)

	return src, logGroup, nil
}

// GetAccountID returns the AWS account ID for the current profile.
func (a *App) GetAccountID() string {
	profile := a.GetProfile()
	if id, ok := a.AccountIDCache[profile]; ok {
		return id
	}

	id, err := cloudwatch.GetAccountID(profile, a.GetRegion())
	if err != nil {
		a.Debugf("Failed to get account ID: %v", err)
		return ""
	}

	a.AccountIDCache[profile] = id
	return id
}
