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
		JobName   string `json:"job_name"`
		LogURL    string `json:"log_url"`
	}

	if err := json.Unmarshal(message, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	fmt.Printf("[IngestionAgent] Processing request %s for job %s\n", request.RequestID, request.JobName)

	// Create a LogChunk from the request
	// In a real implementation, this would fetch the actual log content from LogURL
	// and populate Metadata with HTTP headers, content type, etc.
	logChunk := contracts.LogChunk{
		ID:        fmt.Sprintf("chunk-%d", time.Now().UnixNano()),
		RequestID: request.RequestID,
		JobName:   request.JobName,
		Content:   fmt.Sprintf("Placeholder log content for %s", request.LogURL),
		Timestamp: time.Now().Format(time.RFC3339),
		Metadata:  make(map[string]string), // Will be populated with source metadata during log fetch
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
