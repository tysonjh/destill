// Package pipeline provides shared functionality for starting the ingestion and analysis pipeline.
// This package is used by both the CLI (local mode) and the MCP server.
package pipeline

import (
	"context"
	"fmt"
	"os"

	"destill-agent/src/analyze"
	"destill-agent/src/broker"
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
)

// Start starts the ingest and analyze agents as goroutines.
// It uses silent logging to prevent log pollution when running in TUI mode or MCP server mode.
// Errors are still logged to stderr even in silent mode.
func Start(msgBroker broker.Broker, ctx context.Context) {
	// Use silent logger to prevent log pollution in TUI mode
	log := logger.NewSilentLogger()

	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingest.NewAgent(msgBroker, log)
	go func() {
		if err := ingestionAgent.Run(ctx); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := analyze.NewAgent(msgBroker, log)
	go func() {
		if err := analysisAgent.Run(ctx); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Analysis agent error: %v\n", err)
		}
	}()
}
