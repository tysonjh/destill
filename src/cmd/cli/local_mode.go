package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/pipeline"
	"destill-agent/src/provider"
	"destill-agent/src/store"
	"destill-agent/src/tui"
)

const (
	// RequestIDFormat documents the format of generated request IDs
	// Format: req-YYYYMMDDTHHmmss-XXXXXXXX (ISO timestamp + 8 hex random chars)
	// Example: req-20240115T143022-a3f8c91d
	RequestIDFormat = "req-YYYYMMDDTHHmmss-XXXXXXXX"
)

// ========================================
// LocalMode - Infrastructure & Lifecycle
// ========================================

// LocalMode encapsulates the in-memory broker and agent lifecycle for local execution.
// It provides a clean interface for starting agents, submitting analysis requests,
// and managing the complete lifecycle of the local mode infrastructure.
type LocalMode struct {
	broker broker.Broker
	ctx    context.Context
	cancel context.CancelFunc
}

// NewLocalMode creates and initializes a new local mode instance.
// Agents are started immediately and begin listening for requests.
// Returns error if pipeline initialization fails.
func NewLocalMode() (*LocalMode, error) {
	msgBroker := broker.NewInMemoryBroker()
	ctx, cancel := context.WithCancel(context.Background())

	// Start ingest and analyze agents as goroutines.
	// Subscriptions happen synchronously to avoid race conditions.
	if err := pipeline.Start(msgBroker, ctx); err != nil {
		cancel()
		msgBroker.Close()
		return nil, fmt.Errorf("failed to start pipeline: %w", err)
	}

	return &LocalMode{
		broker: msgBroker,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

// SubmitAnalysis publishes an analysis request to the broker.
// The ingest and analyze agents will process this request asynchronously.
func (lm *LocalMode) SubmitAnalysis(buildURL string) (string, error) {
	// Debug: log when submitting
	if os.Getenv("DESTILL_DEBUG_ARTIFACTS") != "" {
		f, _ := os.OpenFile("/tmp/destill-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, "SubmitAnalysis called for: %s\n", buildURL)
			f.Close()
		}
	}

	requestID, data, err := buildAnalysisRequest(buildURL)
	if err != nil {
		return "", err
	}

	if err := lm.broker.Publish(lm.ctx, contracts.TopicRequests, requestID, data); err != nil {
		return "", fmt.Errorf("failed to publish request: %w", err)
	}

	// Debug: log after publishing
	if os.Getenv("DESTILL_DEBUG_ARTIFACTS") != "" {
		f, _ := os.OpenFile("/tmp/destill-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			fmt.Fprintf(f, "Published request %s to broker\n", requestID)
			f.Close()
		}
	}

	return requestID, nil
}

// Broker returns the underlying message broker for subscription purposes.
func (lm *LocalMode) Broker() broker.Broker {
	return lm.broker
}

// Close gracefully shuts down the agents and closes the broker.
func (lm *LocalMode) Close() {
	lm.cancel()
	lm.broker.Close()
}

// ========================================
// Display Layer - Presentation
// ========================================

// displayTUI launches the interactive terminal UI with pre-subscribed channels.
// If channels is nil and initialCards is provided (from cache), displays them immediately.
// If channels is provided, streams live updates from the broker as analysis progresses.
// After the TUI exits, collected data is persisted to SQLite for novelty/flaky detection.
func displayTUI(channels *tui.BrokerChannels, initialCards []contracts.TriageCard) error {
	// Show appropriate startup message
	if channels == nil || channels.CardChan == nil {
		fmt.Println("🚀 Launching TUI with cached data...")
	} else {
		fmt.Println("🚀 Launching TUI (cards will stream in as they're analyzed)...")
	}

	// Brief pause to ensure log output completes before TUI starts
	time.Sleep(100 * time.Millisecond)

	// Check for debug mode - if enabled, tell user where logs go
	if os.Getenv("DESTILL_DEBUG_ARTIFACTS") != "" {
		fmt.Println("[DEBUG] Artifact debug logging enabled - writing to /tmp/destill-debug.log")
	}

	result, err := tui.StartWithChannels(channels, initialCards)
	if err != nil {
		return err
	}

	// Persist collected data to SQLite (best-effort, don't fail if it errors)
	recordAnalysisResult(result)

	return nil
}

// recordAnalysisResult persists the analysis result to SQLite for novelty/flaky detection.
func recordAnalysisResult(result *tui.AnalysisResult) {
	if result == nil || (len(result.Cards) == 0 && len(result.TestResults) == 0) {
		return
	}
	pipelineID := result.PipelineID()
	buildNumber := result.BuildNumber()
	if pipelineID == "" || buildNumber == 0 {
		return
	}

	history, err := store.NewTestHistory("")
	if err != nil {
		return
	}
	defer history.Close()

	ctx := context.Background()
	_ = history.RecordBuildData(ctx, pipelineID, buildNumber, result.Metadata, result.TestResults, result.Cards)
}

// displayJSON collects findings from the broker and outputs them as JSON.
// The analysis request must already be submitted before calling this function.
func displayJSON(msgBroker broker.Broker) error {
	ctx := context.Background()
	return collectAndOutputJSON(ctx, msgBroker)
}

// ========================================
// Helper Functions
// ========================================

// validateBuildURL validates the build URL format and required API token
func validateBuildURL(buildURL string) error {
	ref, err := provider.ParseURL(buildURL)
	if err != nil {
		return provider.WrapError(err)
	}
	// Validate token upfront to fail fast before starting the pipeline
	if err := provider.ValidateToken(ref); err != nil {
		return provider.WrapError(err)
	}
	return nil
}

// generateRequestID creates a unique request identifier
// Format: req-YYYYMMDDTHHmmss-XXXXXXXX (ISO timestamp + 8 hex random chars)
func generateRequestID() string {
	// Compact ISO format (sorts correctly)
	timestamp := time.Now().UTC().Format("20060102T150405")

	// Random suffix (4 bytes = 8 hex chars = 32 bits of randomness)
	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-only ID if random generation fails
		// This should be extremely rare (only on broken systems)
		return fmt.Sprintf("req-%s-00000000", timestamp)
	}
	randomSuffix := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("req-%s-%s", timestamp, randomSuffix)
}

// buildAnalysisRequest creates a new analysis request with a unique ID.
// Returns the request ID and marshaled JSON data ready for publishing.
func buildAnalysisRequest(buildURL string) (requestID string, data []byte, err error) {
	requestID = generateRequestID()

	payload := contracts.AnalysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
	}

	data, err = json.Marshal(payload)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	return requestID, data, nil
}

// loadCachedCards reads, unmarshals, and sorts cached triage cards from a file.
// Returns an empty slice if cacheFile is empty (not an error).
func loadCachedCards(cacheFile string) ([]contracts.TriageCard, error) {
	if cacheFile == "" {
		return []contracts.TriageCard{}, nil
	}

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cards []contracts.TriageCard
	if err := json.Unmarshal(data, &cards); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	// Sort by priority for consistent display
	sortCardsByPriority(cards)

	return cards, nil
}

// sortCardsByPriority sorts cards by confidence score (desc) and recurrence count (desc)
func sortCardsByPriority(cards []contracts.TriageCard) {
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
			return cards[i].ConfidenceScore > cards[j].ConfidenceScore
		}
		return cards[i].GetRecurrenceCount() > cards[j].GetRecurrenceCount()
	})
}
