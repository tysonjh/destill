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
	"destill-agent/src/provider"
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
func NewLocalMode() *LocalMode {
	msgBroker := broker.NewInMemoryBroker()
	ctx, cancel := context.WithCancel(context.Background())

	// Start ingest and analyze agents as goroutines
	startStreamPipeline(msgBroker, ctx)

	return &LocalMode{
		broker: msgBroker,
		ctx:    ctx,
		cancel: cancel,
	}
}

// SubmitAnalysis publishes an analysis request to the broker.
// The ingest and analyze agents will process this request asynchronously.
func (lm *LocalMode) SubmitAnalysis(buildURL string) (string, error) {
	requestID, data, err := buildAnalysisRequest(buildURL)
	if err != nil {
		return "", err
	}

	if err := lm.broker.Publish(lm.ctx, contracts.TopicRequests, requestID, data); err != nil {
		return "", fmt.Errorf("failed to publish request: %w", err)
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

// displayTUI launches the interactive terminal UI.
// If initialCards is provided (from cache), displays them immediately without streaming.
// If initialCards is empty, streams live updates from the broker as analysis progresses.
func displayTUI(msgBroker broker.Broker, initialCards []contracts.TriageCard) error {
	// Show appropriate startup message
	if len(initialCards) == 0 {
		fmt.Println("🚀 Launching TUI (cards will stream in as they're analyzed)...")
	} else {
		fmt.Println("🚀 Launching TUI with cached data...")
	}

	// Brief pause to ensure log output completes before TUI starts
	time.Sleep(100 * time.Millisecond)

	// Determine if broker should stream (only if no cache)
	var brokerForTUI broker.Broker
	if len(initialCards) == 0 {
		brokerForTUI = msgBroker
	}

	return tui.StartWithBroker(brokerForTUI, initialCards)
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

// validateBuildURL validates the build URL format
func validateBuildURL(buildURL string) error {
	if _, err := provider.ParseURL(buildURL); err != nil {
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
