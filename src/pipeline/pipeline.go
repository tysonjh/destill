// Package pipeline defines the interface for processing build analysis requests.
package pipeline

import (
	"context"
	"destill-agent/src/contracts"
)

// Pipeline defines the interface for processing build analysis requests.
// It abstracts the difference between Legacy (in-memory) and Agentic (distributed) modes.
type Pipeline interface {
	// Submit submits a build URL for analysis, returns request ID
	Submit(ctx context.Context, buildURL string) (requestID string, err error)

	// Status returns the current status of a request
	Status(ctx context.Context, requestID string) (*contracts.RequestStatus, error)

	// Stream returns a channel of findings for a request (for TUI streaming)
	// Returns nil for agentic mode (use Store.GetFindings instead)
	Stream(ctx context.Context, requestID string) (<-chan contracts.TriageCard, error)

	// Close shuts down the pipeline
	Close() error
}

// Config holds pipeline configuration.
type Config struct {
	// Broker configuration
	RedpandaBrokers []string // Empty = use in-memory (legacy mode)

	// Postgres configuration (for agentic mode)
	PostgresDSN string

	// Buildkite configuration
	BuildkiteToken string
}

// Mode represents the pipeline execution mode.
type Mode int

const (
	// LegacyMode uses in-memory broker and streaming TUI
	LegacyMode Mode = iota
	// AgenticMode uses Redpanda + Postgres with distributed agents
	AgenticMode
)

// DetectMode determines the pipeline mode based on configuration.
func DetectMode(cfg *Config) Mode {
	if len(cfg.RedpandaBrokers) > 0 {
		return AgenticMode
	}
	return LegacyMode
}

