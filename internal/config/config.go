// Package config handles clew configuration loading.
// The config is minimal — just profile shortcuts and a default.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the clew configuration file.
type Config struct {
	Profiles map[string]string `yaml:"profiles"` // short name → AWS profile name
	Default  string            `yaml:"default"`  // default profile short name
	Region   string            `yaml:"region"`   // default region (optional)
}

// Path returns the path to the clew config file.
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".clew", "config.yaml")
}

// DataDir returns the path to the clew data directory (~/.clew/).
func DataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".clew")
}

// Load loads the configuration from ~/.clew/config.yaml.
// Returns an empty config (not an error) if the file doesn't exist.
func Load() (*Config, error) {
	cfg := &Config{
		Profiles: make(map[string]string),
	}

	path := Path()
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

// ResolveProfile maps an -e flag value to an AWS profile name.
// If envName is empty, uses the default. If the name isn't in the profiles map,
// it's treated as a literal AWS profile name (pass-through).
func (c *Config) ResolveProfile(envName string) string {
	if envName == "" {
		envName = c.Default
	}
	if envName == "" {
		return ""
	}
	if awsProfile, ok := c.Profiles[envName]; ok {
		return awsProfile
	}
	// Pass-through: treat as literal AWS profile name
	return envName
}

// Save writes the configuration to ~/.clew/config.yaml.
func Save(cfg *Config) error {
	path := Path()
	if path == "" {
		return fmt.Errorf("could not determine config path")
	}

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
