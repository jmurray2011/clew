package source

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the clew configuration file.
type Config struct {
	Sources       map[string]SourceAlias `yaml:"sources"`
	DefaultSource string                 `yaml:"default_source"`
	Output        OutputConfig           `yaml:"output"`
}

// SourceAlias defines a named source alias.
type SourceAlias struct {
	URI    string `yaml:"uri"`
	Format string `yaml:"format,omitempty"` // Optional format hint for local files
}

// OutputConfig defines output preferences.
type OutputConfig struct {
	Format     string `yaml:"format"`     // text, json, csv
	Timestamps string `yaml:"timestamps"` // local, utc
	Color      string `yaml:"color"`      // auto, always, never
}

// ConfigPath returns the path to the clew config file.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".clew", "config.yaml")
}

// LoadConfig loads the configuration from ~/.clew/config.yaml.
// Returns an empty config (not an error) if the file doesn't exist.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		Sources: make(map[string]SourceAlias),
		Output: OutputConfig{
			Format:     "text",
			Timestamps: "local",
			Color:      "auto",
		},
	}

	path := ConfigPath()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// SaveConfig saves the configuration to ~/.clew/config.yaml.
func SaveConfig(cfg *Config) error {
	path := ConfigPath()
	if path == "" {
		return os.ErrNotExist
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
