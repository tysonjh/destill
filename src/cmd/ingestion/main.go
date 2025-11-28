// Package main provides the Ingestion Agent for the Destill log triage tool.
// This agent consumes requests from a topic and publishes raw log data.
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/contracts"
)

// IngestionAgent consumes requests and publishes raw log data via a MessageBroker.
type IngestionAgent struct {
	msgBroker contracts.MessageBroker
}

// NewIngestionAgent creates a new IngestionAgent with the given broker.
func NewIngestionAgent(msgBroker contracts.MessageBroker) *IngestionAgent {
	return &IngestionAgent{msgBroker: msgBroker}
}

// Run starts the ingestion agent's main loop.
// It subscribes to the destill_requests topic and processes incoming requests.
func (a *IngestionAgent) Run() error {
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
func (a *IngestionAgent) processRequest(message []byte) error {
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
	logChunk := contracts.LogChunk{
		ID:        fmt.Sprintf("chunk-%d", time.Now().UnixNano()),
		RequestID: request.RequestID,
		JobName:   request.JobName,
		Content:   fmt.Sprintf("Placeholder log content for %s", request.LogURL),
		Timestamp: time.Now().Format(time.RFC3339),
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

func main() {
	fmt.Println("Destill Ingestion Agent")
	fmt.Println("This agent should be started by the CLI orchestrator.")
	fmt.Println("Run 'destill analyze' to start the full pipeline.")
}
