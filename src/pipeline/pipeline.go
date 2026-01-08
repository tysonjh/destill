// Package pipeline provides shared functionality for starting the ingestion and analysis pipeline.
// This package is used by both the CLI (local mode) and the MCP server.
package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

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

// AnalysisResult contains the results of analyzing a build.
type AnalysisResult struct {
	Cards       []contracts.TriageCard
	TestResults []contracts.TestResult
	Metadata    *contracts.BuildMetadata
}

// AnalyzeBuild runs the full analysis pipeline for a single build URL.
// This is a synchronous operation that starts the pipeline, submits the request,
// collects results, and returns them. Used by both MCP server and sync command.
func AnalyzeBuild(ctx context.Context, buildURL string) (*AnalysisResult, error) {
	// Create in-memory broker and start pipeline
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Subscribe to topics BEFORE starting pipeline to avoid race conditions
	findingsCh, err := msgBroker.Subscribe(ctx, contracts.TopicAnalysisFindings, "pipeline-findings")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to findings: %w", err)
	}
	testsCh, err := msgBroker.Subscribe(ctx, contracts.TopicTestResults, "pipeline-tests")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to test results: %w", err)
	}
	metadataCh, err := msgBroker.Subscribe(ctx, contracts.TopicBuildMetadata, "pipeline-metadata")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to build metadata: %w", err)
	}

	if err := Start(msgBroker, pipelineCtx); err != nil {
		return nil, fmt.Errorf("failed to start pipeline: %w", err)
	}

	// Submit analysis request
	requestID := generateRequestID()
	req := contracts.AnalysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	msgBroker.Publish(ctx, contracts.TopicRequests, requestID, reqData)

	// Collect results with timeout
	result, err := collectResults(ctx, findingsCh, testsCh, metadataCh)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// collectResults collects findings, test results, and build metadata from channels.
func collectResults(ctx context.Context, findingsCh, testsCh, metadataCh <-chan broker.Message) (*AnalysisResult, error) {
	result := &AnalysisResult{}
	timeout := time.After(120 * time.Second)
	lastActivity := time.Now()

	for {
		select {
		case msg := <-findingsCh:
			var card contracts.TriageCard
			if err := json.Unmarshal(msg.Value, &card); err == nil {
				result.Cards = append(result.Cards, card)
				lastActivity = time.Now()
			}
		case msg := <-testsCh:
			var tr contracts.TestResult
			if err := json.Unmarshal(msg.Value, &tr); err == nil {
				result.TestResults = append(result.TestResults, tr)
				lastActivity = time.Now()
			}
		case msg := <-metadataCh:
			var meta contracts.BuildMetadata
			if err := json.Unmarshal(msg.Value, &meta); err == nil {
				result.Metadata = &meta
				lastActivity = time.Now()
			}
		case <-timeout:
			return result, nil
		case <-ctx.Done():
			return result, ctx.Err()
		default:
			// If we have cards and no activity for 10 seconds, we're done
			if time.Since(lastActivity) > 10*time.Second && len(result.Cards) > 0 {
				return result, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	timestamp := time.Now().UTC().Format("20060102T150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return fmt.Sprintf("req-%s-%s", timestamp, hex.EncodeToString(randomBytes))
}
