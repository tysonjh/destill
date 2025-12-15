package config

import (
	"os"
	"testing"
)

func TestLoadFromEnv(t *testing.T) {
	// Save and restore original env var
	originalToken := os.Getenv("BUILDKITE_API_TOKEN")
	defer func() {
		if originalToken != "" {
			os.Setenv("BUILDKITE_API_TOKEN", originalToken)
		} else {
			os.Unsetenv("BUILDKITE_API_TOKEN")
		}
	}()

	t.Run("valid token", func(t *testing.T) {
		testToken := "test-token-12345"
		os.Setenv("BUILDKITE_API_TOKEN", testToken)

		cfg, err := LoadFromEnv()
		if err != nil {
			t.Fatalf("LoadFromEnv() unexpected error: %v", err)
		}

		if cfg.BuildkiteAPIToken != testToken {
			t.Errorf("LoadFromEnv() token = %v, want %v", cfg.BuildkiteAPIToken, testToken)
		}
	})

	t.Run("missing token", func(t *testing.T) {
		os.Unsetenv("BUILDKITE_API_TOKEN")

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("LoadFromEnv() expected error for missing token, got nil")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		os.Setenv("BUILDKITE_API_TOKEN", "")

		_, err := LoadFromEnv()
		if err == nil {
			t.Error("LoadFromEnv() expected error for empty token, got nil")
		}
	})
}
