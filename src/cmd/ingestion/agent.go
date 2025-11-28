// Package ingestion provides the Ingestion Agent for the Destill log triage tool.
// This agent consumes requests from a topic and publishes raw log data.
package ingestion

import (
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/contracts"
)

// Agent consumes requests and publishes raw log data via a MessageBroker.
type Agent struct {
	msgBroker contracts.MessageBroker
}

// NewAgent creates a new IngestionAgent with the given broker.
func NewAgent(msgBroker contracts.MessageBroker) *Agent {
	return &Agent{msgBroker: msgBroker}
}

// Run starts the ingestion agent's main loop.
// It subscribes to the destill_requests topic and processes incoming requests.
func (a *Agent) Run() error {
	requestChannel, err := a.msgBroker.Subscribe("destill_requests")
	if err != nil {
		return fmt.Errorf("failed to subscribe to destill_requests: %w", err)
	}

	fmt.Println("[IngestionAgent] Listening for requests on 'destill_requests' topic...")

	for message := range requestChannel {
		if err := a.processRequest(message); err != nil {
			fmt.Printf("[IngestionAgent] Error processing request: %v\n", err)
		}
	}

	return nil
}

// processRequest handles an incoming request message.
func (a *Agent) processRequest(message []byte) error {
	// Parse the incoming request
	var request struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}

	if err := json.Unmarshal(message, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	fmt.Printf("[IngestionAgent] Processing build request %s\n", request.RequestID)
	fmt.Printf("[IngestionAgent] Build URL: %s\n", request.BuildURL)

	// In a real implementation, this would:
	// 1. Fetch the build metadata from the Buildkite API
	// 2. Discover all jobs in the build
	// 3. For each job, fetch and publish its log content

	// For now, create a placeholder LogChunk representing the build
	// In production, this would be multiple LogChunks (one per job)
	logChunk := contracts.LogChunk{
		ID:        fmt.Sprintf("chunk-%d", time.Now().UnixNano()),
		RequestID: request.RequestID,
		JobName:   "placeholder-job", // Will be populated per-job when fetching from Buildkite
		Content:   fmt.Sprintf("Placeholder log content for build %s", request.BuildURL),
		Timestamp: time.Now().Format(time.RFC3339),
		Metadata: map[string]string{
			"build_url": request.BuildURL,
		},
	}

	// Marshal and publish to ci_logs_raw topic
	data, err := json.Marshal(logChunk)
	if err != nil {
		return fmt.Errorf("failed to marshal log chunk: %w", err)
	}

	if err := a.msgBroker.Publish("ci_logs_raw", data); err != nil {
		return fmt.Errorf("failed to publish to ci_logs_raw: %w", err)
	}

	fmt.Printf("[IngestionAgent] Published log chunk to 'ci_logs_raw'\n")
	return nil
}
