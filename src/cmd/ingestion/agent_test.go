// Package ingestion provides the Ingestion Agent for the Destill log triage tool.
package ingestion

import (
	"encoding/json"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

// TestIngestionAgentMarshalsLogChunk verifies the Ingestion Agent correctly marshals its output.
func TestIngestionAgentMarshalsLogChunk(t *testing.T) {
	// Create broker and subscribe to output topic
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	outputChan, err := msgBroker.Subscribe("ci_logs_raw")
	if err != nil {
		t.Fatalf("Failed to subscribe to ci_logs_raw: %v", err)
	}

	// Create and start agent with a placeholder API token
	// Note: This test will fail if it tries to make real API calls
	agent := NewAgent(msgBroker, "test-token-placeholder")

	// Start agent in goroutine
	go func() {
		_ = agent.Run()
	}()

	// Give agent time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Create and publish a mock request
	request := struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}{
		RequestID: "test-request-123",
		BuildURL:  "https://buildkite.com/org/pipeline/builds/4091",
	}

	requestData, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	if err := msgBroker.Publish("destill_requests", requestData); err != nil {
		t.Fatalf("Failed to publish request: %v", err)
	}

	// Wait for output
	select {
	case output := <-outputChan:
		// Verify output can be unmarshaled as LogChunk
		var logChunk contracts.LogChunk
		if err := json.Unmarshal(output, &logChunk); err != nil {
			t.Fatalf("Failed to unmarshal output as LogChunk: %v", err)
		}

		// Verify fields are populated correctly
		if logChunk.RequestID != request.RequestID {
			t.Errorf("RequestID mismatch: expected %q, got %q", request.RequestID, logChunk.RequestID)
		}
		if logChunk.ID == "" {
			t.Error("LogChunk ID should not be empty")
		}
		if logChunk.Timestamp == "" {
			t.Error("LogChunk Timestamp should not be empty")
		}
		if logChunk.Metadata == nil {
			t.Error("LogChunk Metadata should not be nil")
		}
		if logChunk.Metadata["build_url"] != request.BuildURL {
			t.Errorf("Metadata build_url mismatch: expected %q, got %q", request.BuildURL, logChunk.Metadata["build_url"])
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for LogChunk output")
	}
}

// TestIngestionAgentHandlesInvalidRequest verifies agent handles malformed requests gracefully.
func TestIngestionAgentHandlesInvalidRequest(t *testing.T) {
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	agent := NewAgent(msgBroker, "test-token-placeholder")

	// Process invalid JSON - should not panic
	err := agent.processRequest([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

// TestLogChunkSerialization verifies LogChunk can be serialized and deserialized correctly.
func TestLogChunkSerialization(t *testing.T) {
	original := contracts.LogChunk{
		ID:        "chunk-123",
		RequestID: "req-456",
		JobName:   "build-job",
		Content:   "Log content here",
		Timestamp: "2025-11-28T12:00:00Z",
		Metadata: map[string]string{
			"build_url": "https://example.com/build/123",
			"source":    "buildkite",
		},
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal LogChunk: %v", err)
	}

	// Unmarshal
	var restored contracts.LogChunk
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal LogChunk: %v", err)
	}

	// Verify fields
	if restored.ID != original.ID {
		t.Errorf("ID mismatch: expected %q, got %q", original.ID, restored.ID)
	}
	if restored.RequestID != original.RequestID {
		t.Errorf("RequestID mismatch: expected %q, got %q", original.RequestID, restored.RequestID)
	}
	if restored.JobName != original.JobName {
		t.Errorf("JobName mismatch: expected %q, got %q", original.JobName, restored.JobName)
	}
	if restored.Content != original.Content {
		t.Errorf("Content mismatch: expected %q, got %q", original.Content, restored.Content)
	}
	if restored.Metadata["build_url"] != original.Metadata["build_url"] {
		t.Errorf("Metadata build_url mismatch")
	}
}
