package cmd

import (
	"context"

	"github.com/jmurray2011/clew/internal/cases"
	"github.com/jmurray2011/clew/internal/cloudwatch"
	"github.com/jmurray2011/clew/internal/ui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// appContextKey is the context key for the App instance.
type appContextKey struct{}

// Config holds all configuration values that were previously global.
type Config struct {
	Profile      string
	Region       string
	OutputFormat string
	Verbose      bool
	NoColor      bool
	Quiet        bool
}

// App holds the application dependencies that can be injected for testing.
type App struct {
	Config      Config
	Render      *ui.Renderer
	CaseManager *cases.Manager
	// AccountIDCache caches AWS account IDs by profile
	AccountIDCache map[string]string
}

// NewApp creates a new App with default configuration from viper.
func NewApp() *App {
	cfg := Config{
		Profile:      getProfile(),
		Region:       getRegion(),
		OutputFormat: getOutputFormat(),
		Verbose:      IsVerbose(),
		NoColor:      noColor,
		Quiet:        quiet,
	}

	mgr, err := cases.NewManager()
	if err != nil {
		// Log but don't fail - case management is optional
		if render != nil {
			render.Debug("Failed to initialize case manager: %v", err)
		}
	}

	return &App{
		Config:         cfg,
		Render:         render,
		CaseManager:    mgr,
		AccountIDCache: make(map[string]string),
	}
}

// NewAppWithConfig creates a new App with the given configuration.
// This is primarily used for testing.
func NewAppWithConfig(cfg Config, renderer *ui.Renderer, mgr *cases.Manager) *App {
	return &App{
		Config:         cfg,
		Render:         renderer,
		CaseManager:    mgr,
		AccountIDCache: make(map[string]string),
	}
}

// GetApp retrieves the App from the command context.
// If no App is set, it creates a new default one.
func GetApp(cmd *cobra.Command) *App {
	if app, ok := cmd.Context().Value(appContextKey{}).(*App); ok {
		return app
	}
	// Fallback: create default app (maintains backward compatibility)
	return NewApp()
}

// SetApp stores the App in the context for a command.
func SetApp(ctx context.Context, app *App) context.Context {
	return context.WithValue(ctx, appContextKey{}, app)
}

// Debugf prints a debug message if verbose mode is enabled.
// This is a method on App to allow per-instance verbose control.
func (a *App) Debugf(format string, args ...interface{}) {
	if a.Config.Verbose || viper.GetBool("verbose") {
		a.Render.Debug(format, args...)
	}
}

// GetProfile returns the profile from Config or viper.
func (a *App) GetProfile() string {
	if a.Config.Profile != "" {
		return a.Config.Profile
	}
	return viper.GetString("profile")
}

// GetRegion returns the region from Config or viper.
func (a *App) GetRegion() string {
	if a.Config.Region != "" {
		return a.Config.Region
	}
	return viper.GetString("region")
}

// GetOutputFormat returns the output format from Config or viper.
func (a *App) GetOutputFormat() string {
	if a.Config.OutputFormat != "" {
		return a.Config.OutputFormat
	}
	return viper.GetString("output")
}

// GetAccountID returns the AWS account ID for the current profile.
// Results are cached to avoid repeated API calls during a session.
func (a *App) GetAccountID() string {
	profile := a.GetProfile()

	// Check cache first
	if id, ok := a.AccountIDCache[profile]; ok {
		return id
	}

	// Fetch from AWS
	id, err := cloudwatch.GetAccountID(profile, a.GetRegion())
	if err != nil {
		a.Debugf("Failed to get account ID: %v", err)
		return ""
	}

	// Cache for future use
	a.AccountIDCache[profile] = id
	return id
}
