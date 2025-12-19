// Package pipeline provides shared functionality for starting the ingestion and analysis pipeline.
// This package is used by both the CLI (local mode) and the MCP server.
package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"destill-agent/src/analyze"
	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
	"destill-agent/src/store"
)

// Start starts the ingest, analyze, and artifact agents as goroutines.
// Subscriptions are created synchronously to avoid race conditions, then processing
// loops run asynchronously. Returns error if subscriptions fail.
// It uses silent logging to prevent log pollution when running in TUI mode or MCP server mode.
// Errors are still logged to stderr even in silent mode.
func Start(msgBroker broker.Broker, ctx context.Context) error {
	// Debug: Write to log file if debug mode enabled
	if os.Getenv("DESTILL_DEBUG_ARTIFACTS") != "" {
		f, err := os.OpenFile("/tmp/destill-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			fmt.Fprintf(f, "Pipeline.Start() called - agents starting\n")
			f.Close()
		}
	}

	// Use silent logger to prevent log pollution in TUI mode
	log := logger.NewSilentLogger()

	// Subscribe to topics synchronously BEFORE starting goroutines.
	// This ensures agents are ready to receive messages when Start returns.
	requestsCh, err := msgBroker.Subscribe(ctx, contracts.TopicRequests, "destill-ingest")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", contracts.TopicRequests, err)
	}

	// Artifact agent also subscribes to requests (separate subscription)
	artifactRequestsCh, err := msgBroker.Subscribe(ctx, contracts.TopicRequests, "destill-artifacts")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s for artifacts: %w", contracts.TopicRequests, err)
	}

	logsRawCh, err := msgBroker.Subscribe(ctx, contracts.TopicLogsRaw, "destill-analyze")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", contracts.TopicLogsRaw, err)
	}

	// Start Ingestion Agent processing loop as a goroutine
	ingestionAgent := ingest.NewAgent(msgBroker, log)
	go func() {
		if err := ingestionAgent.RunWithChannel(ctx, requestsCh); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent processing loop as a goroutine
	analysisAgent := analyze.NewAgent(msgBroker, log)
	go func() {
		if err := analysisAgent.RunWithChannel(ctx, logsRawCh); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Analysis agent error: %v\n", err)
		}
	}()

	// Start Artifact Agent for JUnit XML processing
	// Initialize test history database (optional - nil if it fails)
	var history *store.TestHistory
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dbPath := filepath.Join(homeDir, ".destill", "history.db")
		history, _ = store.NewTestHistory(dbPath) // Ignore errors - history is optional
	}

	artifactAgent := ingest.NewArtifactAgent(msgBroker, history, log)
	go func() {
		if err := artifactAgent.RunWithChannel(ctx, artifactRequestsCh); err != nil && err != context.Canceled {
			fmt.Fprintf(os.Stderr, "[Pipeline] Artifact agent error: %v\n", err)
		}
	}()

	return nil
}
