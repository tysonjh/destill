package analyze

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

func TestAgent_Creation(t *testing.T) {
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	log := logger.NewSilentLogger()
	agent := NewAgent(brk, log)

	if agent == nil {
		t.Fatal("Expected agent to be created")
	}
	if agent.broker == nil {
		t.Error("Expected broker to be set")
	}
	if agent.logger == nil {
		t.Error("Expected logger to be set")
	}
}

func TestAgent_ProcessChunk(t *testing.T) {
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Subscribe to output topic
	findingsChan, err := brk.Subscribe(ctx, contracts.TopicAnalysisFindings, "test-consumer")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Create agent
	log := logger.NewSilentLogger()
	agent := NewAgent(brk, log)

	// Create a chunk with errors
	chunk := contracts.LogChunkV2{
		RequestID:   "req-test",
		BuildID:     "build-test",
		JobName:     "test-job",
		JobID:       "job-123",
		ChunkIndex:  0,
		TotalChunks: 1,
		Content:     "INFO: Starting\nERROR: Connection failed\nFATAL: System crash",
		LineStart:   1,
		LineEnd:     3,
		Metadata:    map[string]string{"build_url": "https://example.com"},
	}

	// Marshal chunk
	chunkData, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("Failed to marshal chunk: %v", err)
	}

	// Create broker message
	msg := broker.Message{
		Topic: contracts.TopicLogsRaw,
		Key:   "build-test",
		Value: chunkData,
	}

	// Process chunk
	if err := agent.processChunk(ctx, msg); err != nil {
		t.Fatalf("processChunk failed: %v", err)
	}

	// Verify findings were published
	findingsReceived := 0
	timeout := time.After(2 * time.Second)

	for findingsReceived < 2 { // Expecting ERROR and FATAL
		select {
		case findingMsg := <-findingsChan:
			var card contracts.TriageCardV2
			if err := json.Unmarshal(findingMsg.Value, &card); err != nil {
				t.Fatalf("Failed to unmarshal finding: %v", err)
			}

			// Verify card structure
			if card.RequestID != "req-test" {
				t.Errorf("Expected request ID req-test, got %s", card.RequestID)
			}
			if card.JobName != "test-job" {
				t.Errorf("Expected job name test-job, got %s", card.JobName)
			}
			if card.Severity != "ERROR" && card.Severity != "FATAL" {
				t.Errorf("Expected ERROR or FATAL, got %s", card.Severity)
			}
			if card.ConfidenceScore <= 0 {
				t.Errorf("Expected positive confidence score, got %.2f", card.ConfidenceScore)
			}

			findingsReceived++

		case <-timeout:
			t.Fatalf("Timeout waiting for findings: received %d/2", findingsReceived)
		}
	}

	t.Logf("Successfully received %d findings", findingsReceived)
}

func TestAgent_EmptyChunk(t *testing.T) {
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	log := logger.NewSilentLogger()
	agent := NewAgent(brk, log)

	// Create empty chunk
	chunk := contracts.LogChunkV2{
		RequestID: "req-empty",
		Content:   "",
	}

	chunkData, _ := json.Marshal(chunk)
	msg := broker.Message{
		Topic: contracts.TopicLogsRaw,
		Value: chunkData,
	}

	// Should not error on empty chunk
	if err := agent.processChunk(ctx, msg); err != nil {
		t.Errorf("Expected no error for empty chunk, got %v", err)
	}
}

func TestAgent_ChunkWithoutErrors(t *testing.T) {
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Subscribe to output
	findingsChan, err := brk.Subscribe(ctx, contracts.TopicAnalysisFindings, "test-consumer")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	log := logger.NewSilentLogger()
	agent := NewAgent(brk, log)

	// Create chunk with only INFO logs
	chunk := contracts.LogChunkV2{
		RequestID: "req-info",
		Content:   "INFO: Starting process\nINFO: Processing\nINFO: Complete",
	}

	chunkData, _ := json.Marshal(chunk)
	msg := broker.Message{
		Topic: contracts.TopicLogsRaw,
		Value: chunkData,
	}

	// Process chunk
	if err := agent.processChunk(ctx, msg); err != nil {
		t.Fatalf("processChunk failed: %v", err)
	}

	// Verify no findings were published
	select {
	case <-findingsChan:
		t.Error("Expected no findings for INFO-only chunk")
	case <-time.After(500 * time.Millisecond):
		// Good - no findings
	}
}

func TestAgent_IntegrationFlow(t *testing.T) {
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Subscribe to findings
	findingsChan, err := brk.Subscribe(ctx, contracts.TopicAnalysisFindings, "test-consumer")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Create agent
	log := logger.NewSilentLogger()
	agent := NewAgent(brk, log)

	// Simulate multiple chunks from same build
	chunks := []contracts.LogChunkV2{
		{
			RequestID:   "req-multi",
			BuildID:     "build-multi",
			JobName:     "job1",
			ChunkIndex:  0,
			TotalChunks: 2,
			Content:     "INFO: Starting\nERROR: Failed step 1\nINFO: Retrying",
		},
		{
			RequestID:   "req-multi",
			BuildID:     "build-multi",
			JobName:     "job1",
			ChunkIndex:  1,
			TotalChunks: 2,
			Content:     "INFO: Retry successful\nFATAL: Out of memory\nINFO: Exiting",
		},
	}

	// Process all chunks
	expectedFindings := 2 // One ERROR, one FATAL
	for _, chunk := range chunks {
		chunkData, _ := json.Marshal(chunk)
		msg := broker.Message{
			Topic: contracts.TopicLogsRaw,
			Value: chunkData,
		}

		if err := agent.processChunk(ctx, msg); err != nil {
			t.Fatalf("processChunk failed: %v", err)
		}
	}

	// Collect findings
	findingsReceived := 0
	timeout := time.After(2 * time.Second)

	for findingsReceived < expectedFindings {
		select {
		case findingMsg := <-findingsChan:
			var card contracts.TriageCardV2
			if err := json.Unmarshal(findingMsg.Value, &card); err != nil {
				t.Fatalf("Failed to unmarshal finding: %v", err)
			}

			// All findings should have same request ID
			if card.RequestID != "req-multi" {
				t.Errorf("Expected request ID req-multi, got %s", card.RequestID)
			}

			findingsReceived++

		case <-timeout:
			t.Fatalf("Timeout waiting for findings: received %d/%d",
				findingsReceived, expectedFindings)
		}
	}

	t.Logf("Successfully processed %d chunks and received %d findings",
		len(chunks), findingsReceived)
}
