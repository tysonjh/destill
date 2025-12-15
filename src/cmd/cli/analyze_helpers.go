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

// localModeContext holds resources for local mode execution
type localModeContext struct {
	broker broker.Broker
	ctx    context.Context
}

// analysisRequest represents a request to analyze a build
type analysisRequest struct {
	RequestID string
	BuildURL  string
	Data      []byte // JSON-marshaled payload
}

// validateBuildURL validates the build URL format
func validateBuildURL(buildURL string) error {
	if _, err := provider.ParseURL(buildURL); err != nil {
		return provider.WrapError(err)
	}
	return nil
}

// setupLocalMode initializes broker, context, and agents for local mode
func setupLocalMode() (*localModeContext, func(), error) {
	// Initialize in-memory broker
	msgBroker := broker.NewInMemoryBroker()

	// Create context for agent lifecycle
	agentCtx, agentCancel := context.WithCancel(context.Background())

	// Start the ingest and analyze agents as goroutines
	startStreamPipeline(msgBroker, agentCtx)

	// Cleanup function
	cleanup := func() {
		agentCancel()
		msgBroker.Close()
	}

	return &localModeContext{
		broker: msgBroker,
		ctx:    agentCtx,
	}, cleanup, nil
}

// generateRequestID creates a unique request identifier
// Format: req-YYYYMMDDTHHmmss-XXXXXXXX (ISO timestamp + 8 hex random chars)
func generateRequestID() string {
	// Compact ISO format (sorts correctly)
	timestamp := time.Now().UTC().Format("20060102T150405")

	// Random suffix (4 bytes = 8 hex chars = 32 bits of randomness)
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomSuffix := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("req-%s-%s", timestamp, randomSuffix)
}

// createAnalysisRequest creates and marshals an analysis request
func createAnalysisRequest(buildURL string) (analysisRequest, error) {
	requestID := generateRequestID()

	payload := struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}{
		RequestID: requestID,
		BuildURL:  buildURL,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return analysisRequest{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	return analysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
		Data:      data,
	}, nil
}

// runJSONMode executes analysis and outputs JSON results
func runJSONMode(local *localModeContext, request analysisRequest) error {
	ctx := context.Background()
	return collectAndOutputJSONWithRequest(ctx, local.broker, request.RequestID, request.Data)
}

// runTUIMode executes analysis with interactive TUI
func runTUIMode(local *localModeContext, request analysisRequest, cacheFile string) error {
	ctx := context.Background()

	// Publish request to broker
	if err := local.broker.Publish(ctx, contracts.TopicRequests, request.RequestID, request.Data); err != nil {
		return fmt.Errorf("failed to publish request: %w", err)
	}

	// Load cached cards if specified
	initialCards, err := loadCachedCards(cacheFile)
	if err != nil {
		// Non-fatal - just log and continue without cache
		fmt.Fprintf(os.Stderr, "Warning: failed to load cache: %v\n", err)
		initialCards = []contracts.TriageCard{}
	}

	// Sort cards by priority
	if len(initialCards) > 0 {
		sortCardsByPriority(initialCards)
		fmt.Printf("📂 Loaded %d cards from cache: %s\n", len(initialCards), cacheFile)
	}

	// Launch TUI
	return launchTUI(local.broker, initialCards)
}

// loadCachedCards reads and unmarshals cached triage cards
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

	return cards, nil
}

// sortCardsByPriority sorts cards by confidence score (desc) and recurrence count (desc)
func sortCardsByPriority(cards []contracts.TriageCard) {
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
			return cards[i].ConfidenceScore > cards[j].ConfidenceScore
		}
		countI := getRecurrenceCount(cards[i].Metadata)
		countJ := getRecurrenceCount(cards[j].Metadata)
		return countI > countJ
	})
}

// launchTUI starts the terminal UI with optional streaming broker
func launchTUI(msgBroker broker.Broker, initialCards []contracts.TriageCard) error {
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

