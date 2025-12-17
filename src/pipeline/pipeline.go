// Package pipeline provides shared functionality for starting the ingestion and analysis pipeline.
// This package is used by both the CLI (local mode) and the MCP server.
package pipeline

import (
	"context"
	"fmt"
	"os"

	"destill-agent/src/analyze"
	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
)

// Start starts the ingest and analyze agents as goroutines.
// Subscriptions are created synchronously to avoid race conditions, then processing
// loops run asynchronously. Returns error if subscriptions fail.
// It uses silent logging to prevent log pollution when running in TUI mode or MCP server mode.
// Errors are still logged to stderr even in silent mode.
func Start(msgBroker broker.Broker, ctx context.Context) error {
	// Use silent logger to prevent log pollution in TUI mode
	log := logger.NewSilentLogger()

	// Subscribe to topics synchronously BEFORE starting goroutines.
	// This ensures agents are ready to receive messages when Start returns.
	requestsCh, err := msgBroker.Subscribe(ctx, contracts.TopicRequests, "destill-ingest")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", contracts.TopicRequests, err)
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

	return nil
}
