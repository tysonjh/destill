// Package main provides the standalone ingest agent binary.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"destill-agent/src/broker"
	_ "destill-agent/src/buildkite" // Import for provider registration
	"destill-agent/src/config"
	_ "destill-agent/src/githubactions" // Import for provider registration
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
)

func main() {
	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	// Verify we're in distributed mode
	if len(cfg.RedpandaBrokers) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: REDPANDA_BROKERS environment variable is required for ingest agent")
		fmt.Fprintln(os.Stderr, "Example: export REDPANDA_BROKERS=localhost:19092")
		os.Exit(1)
	}

	// Create logger
	log := logger.NewConsoleLogger()

	log.Info("Starting Destill Ingest Agent")
	log.Info("Redpanda brokers: %v", cfg.RedpandaBrokers)

	// Create Redpanda broker
	brk, err := broker.NewRedpandaBroker(cfg.RedpandaBrokers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create broker: %v\n", err)
		os.Exit(1)
	}
	defer brk.Close()

	// Create ingest agent (no longer needs token - providers get it from env)
	agent := ingest.NewAgent(brk, log)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Info("Shutdown signal received, stopping agent...")
		cancel()
	}()

	// Run agent
	log.Info("Ingest agent started, waiting for requests...")
	if err := agent.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
		os.Exit(1)
	}

	log.Info("Ingest agent stopped")
}
