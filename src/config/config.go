// Package config provides configuration management for the Destill application.
package config

import (
	"fmt"
	"os"
)

// Config holds the application configuration.
type Config struct {
	// BuildkiteAPIToken is the API token for authenticating with Buildkite.
	BuildkiteAPIToken string
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	token := os.Getenv("BUILDKITE_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BUILDKITE_API_TOKEN environment variable is required")
	}

	return &Config{
		BuildkiteAPIToken: token,
	}, nil
}

// MustLoadFromEnv loads configuration from environment variables and panics on error.
// This is useful for initialization in main() where configuration errors should be fatal.
func MustLoadFromEnv() *Config {
	cfg, err := LoadFromEnv()
	if err != nil {
		panic(fmt.Sprintf("failed to load configuration: %v", err))
	}
	return cfg
}
