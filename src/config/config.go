// Package config provides configuration management for the Destill application.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds the application configuration.
type Config struct {
	// BuildkiteAPIToken is the API token for authenticating with Buildkite.
	BuildkiteAPIToken string

	// RedpandaBrokers is a comma-separated list of Redpanda broker addresses.
	// Empty means use in-memory broker (legacy mode).
	RedpandaBrokers []string

	// PostgresDSN is the Postgres connection string.
	// Required for distributed mode.
	PostgresDSN string
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	token := os.Getenv("BUILDKITE_API_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("BUILDKITE_API_TOKEN environment variable is required")
	}

	cfg := &Config{
		BuildkiteAPIToken: token,
		PostgresDSN:       os.Getenv("POSTGRES_DSN"),
	}

	// Parse Redpanda brokers (comma-separated)
	brokersEnv := os.Getenv("REDPANDA_BROKERS")
	if brokersEnv != "" {
		brokers := strings.Split(brokersEnv, ",")
		for i, broker := range brokers {
			brokers[i] = strings.TrimSpace(broker)
		}
		cfg.RedpandaBrokers = brokers
	}

	// Validate distributed mode configuration
	if len(cfg.RedpandaBrokers) > 0 && cfg.PostgresDSN == "" {
		return nil, fmt.Errorf("POSTGRES_DSN is required when REDPANDA_BROKERS is set (distributed mode)")
	}

	return cfg, nil
}
