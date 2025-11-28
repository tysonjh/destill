// Package pipeline provides end-to-end integration tests for the Destill pipeline.
package pipeline

import (
	"encoding/json"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/cmd/analysis"
	"destill-agent/src/cmd/ingestion"
	"destill-agent/src/contracts"
)

// TestEndToEndPipelineFlow tests the entire flow from request to ranked card.
func TestEndToEndPipelineFlow(t *testing.T) {
	// Create shared broker
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	// Subscribe to final output topic BEFORE starting agents
	outputChan, err := msgBroker.Subscribe("ci_failures_ranked")
	if err != nil {
		t.Fatalf("Failed to subscribe to ci_failures_ranked: %v", err)
	}

	// Start Ingestion Agent
	ingestionAgent := ingestion.NewAgent(msgBroker)
	go func() {
		_ = ingestionAgent.Run()
	}()

	// Start Analysis Agent
	analysisAgent := analysis.NewAgent(msgBroker)
	go func() {
		_ = analysisAgent.Run()
	}()

	// Give agents time to subscribe
	time.Sleep(100 * time.Millisecond)

	// Create a mock build request
	requestID := "integration-test-request-123"
	request := struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}{
		RequestID: requestID,
		BuildURL:  "https://buildkite.com/org/pipeline/builds/4091",
	}

	// Marshal and publish the request
	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	if err := msgBroker.Publish("destill_requests", requestData); err != nil {
		t.Fatalf("Failed to publish request: %v", err)
	}

	// Wait for the final TriageCard to appear
	select {
	case output := <-outputChan:
		// Verify the output is a valid TriageCard
		var triageCard contracts.TriageCard
		if err := json.Unmarshal(output, &triageCard); err != nil {
			t.Fatalf("Failed to unmarshal TriageCard: %v", err)
		}

		// Verify the RequestID matches the initial request
		if triageCard.RequestID != requestID {
			t.Errorf("RequestID mismatch: expected %q, got %q", requestID, triageCard.RequestID)
		}

		// Verify critical fields are populated
		if triageCard.ID == "" {
			t.Error("TriageCard ID should not be empty")
		}
		if triageCard.Source == "" {
			t.Error("TriageCard Source should not be empty")
		}
		if triageCard.MessageHash == "" {
			t.Error("TriageCard MessageHash should not be empty")
		}
		if triageCard.ConfidenceScore <= 0 {
			t.Error("TriageCard ConfidenceScore should be greater than 0")
		}

		t.Logf("Successfully received TriageCard: ID=%s, RequestID=%s, Severity=%s",
			triageCard.ID, triageCard.RequestID, triageCard.Severity)

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for TriageCard - pipeline may be broken")
	}
}

// TestPipelineMultipleRequests verifies the pipeline handles multiple concurrent requests.
func TestPipelineMultipleRequests(t *testing.T) {
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	// Subscribe to final output
	outputChan, err := msgBroker.Subscribe("ci_failures_ranked")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Start agents
	ingestionAgent := ingestion.NewAgent(msgBroker)
	go func() { _ = ingestionAgent.Run() }()

	analysisAgent := analysis.NewAgent(msgBroker)
	go func() { _ = analysisAgent.Run() }()

	time.Sleep(100 * time.Millisecond)

	// Send 3 requests
	requestIDs := []string{"req-1", "req-2", "req-3"}
	for _, reqID := range requestIDs {
		request := struct {
			RequestID string `json:"request_id"`
			BuildURL  string `json:"build_url"`
		}{
			RequestID: reqID,
			BuildURL:  "https://buildkite.com/org/pipeline/builds/" + reqID,
		}
		data, _ := json.Marshal(request)
		msgBroker.Publish("destill_requests", data)
	}

	// Collect all outputs
	receivedIDs := make(map[string]bool)
	timeout := time.After(5 * time.Second)

	for len(receivedIDs) < 3 {
		select {
		case output := <-outputChan:
			var card contracts.TriageCard
			if err := json.Unmarshal(output, &card); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
				continue
			}
			receivedIDs[card.RequestID] = true
			t.Logf("Received TriageCard for RequestID: %s", card.RequestID)

		case <-timeout:
			t.Errorf("Timeout: received only %d/3 TriageCards", len(receivedIDs))
			return
		}
	}

	// Verify all requests were processed
	for _, reqID := range requestIDs {
		if !receivedIDs[reqID] {
			t.Errorf("Did not receive TriageCard for RequestID: %s", reqID)
		}
	}
}
