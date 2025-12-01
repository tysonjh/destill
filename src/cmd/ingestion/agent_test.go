// Package ingestion provides the Ingestion Agent for the Destill log triage tool.
package ingestion

import (
	"encoding/json"
	"testing"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

// TestIngestionAgentHandlesInvalidRequest verifies agent handles malformed requests gracefully.
func TestIngestionAgentHandlesInvalidRequest(t *testing.T) {
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	agent := NewAgent(msgBroker, "test-token-placeholder", logger.NewSilentLogger())

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
