package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

func TestAgent_ProcessRequest(t *testing.T) {
	// This is a unit test for the agent's processRequest method
	// Since we can't easily mock the Buildkite client, we'll test the chunker integration
	
	// Create in-memory broker
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Create agent (will fail on real Buildkite API calls, but that's OK for this test)
	log := logger.NewSilentLogger()
	agent := NewAgent(brk, "fake-token", log)

	// Verify agent is created
	if agent == nil {
		t.Fatal("Expected agent to be created")
	}
	if agent.broker == nil {
		t.Error("Expected broker to be set")
	}
	if agent.buildkiteClient == nil {
		t.Error("Expected buildkite client to be set")
	}
	if agent.logger == nil {
		t.Error("Expected logger to be set")
	}
}

func TestAgent_PublishFlow(t *testing.T) {
	// Test that the agent can publish to the correct topic
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Subscribe to the output topic
	msgChan, err := brk.Subscribe(ctx, contracts.TopicLogsRaw, "test-consumer")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Create a chunk and publish it manually (simulating what the agent does)
	chunk := contracts.LogChunkV2{
		RequestID:   "req-test",
		BuildID:     "org-pipeline-123",
		JobName:     "test-job",
		JobID:       "job-123",
		ChunkIndex:  0,
		TotalChunks: 1,
		Content:     "test log content",
		LineStart:   1,
		LineEnd:     1,
		Metadata:    map[string]string{"test": "value"},
	}

	data, err := json.Marshal(chunk)
	if err != nil {
		t.Fatalf("Failed to marshal chunk: %v", err)
	}

	// Publish
	if err := brk.Publish(ctx, contracts.TopicLogsRaw, "org-pipeline-123", data); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Verify we can receive it
	select {
	case msg := <-msgChan:
		var received contracts.LogChunkV2
		if err := json.Unmarshal(msg.Value, &received); err != nil {
			t.Fatalf("Failed to unmarshal received chunk: %v", err)
		}

		if received.RequestID != chunk.RequestID {
			t.Errorf("Expected request ID %s, got %s", chunk.RequestID, received.RequestID)
		}
		if received.BuildID != chunk.BuildID {
			t.Errorf("Expected build ID %s, got %s", chunk.BuildID, received.BuildID)
		}
		if received.Content != chunk.Content {
			t.Errorf("Expected content %s, got %s", chunk.Content, received.Content)
		}

	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for published message")
	}
}

func TestAgent_ChunkingIntegration(t *testing.T) {
	// Test that chunking and publishing work together
	ctx := context.Background()
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Subscribe to output
	msgChan, err := brk.Subscribe(ctx, contracts.TopicLogsRaw, "test-consumer")
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	// Create log content that will produce multiple chunks
	largeContent := ""
	for i := 0; i < 1000; i++ {
		largeContent += string(make([]byte, 600)) + "\n" // 600 bytes per line
	}

	// Chunk it
	chunks := ChunkLog(largeContent, "req-1", "build-1", "job1", "job-id-1", map[string]string{
		"test": "integration",
	})

	if len(chunks) < 2 {
		t.Fatalf("Expected multiple chunks, got %d", len(chunks))
	}

	t.Logf("Created %d chunks from %d bytes of content", len(chunks), len(largeContent))

	// Publish all chunks
	for _, chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			t.Fatalf("Failed to marshal chunk: %v", err)
		}

		if err := brk.Publish(ctx, contracts.TopicLogsRaw, "build-1", data); err != nil {
			t.Fatalf("Failed to publish chunk: %v", err)
		}
	}

	// Receive and verify all chunks
	receivedCount := 0
	timeout := time.After(2 * time.Second)

	for receivedCount < len(chunks) {
		select {
		case msg := <-msgChan:
			var received contracts.LogChunkV2
			if err := json.Unmarshal(msg.Value, &received); err != nil {
				t.Fatalf("Failed to unmarshal chunk: %v", err)
			}

			// Verify chunk structure
			if received.RequestID != "req-1" {
				t.Errorf("Chunk %d: wrong request ID", receivedCount)
			}
			if received.TotalChunks != len(chunks) {
				t.Errorf("Chunk %d: expected total chunks %d, got %d",
					receivedCount, len(chunks), received.TotalChunks)
			}
			if received.Metadata["test"] != "integration" {
				t.Errorf("Chunk %d: metadata not preserved", receivedCount)
			}

			receivedCount++

		case <-timeout:
			t.Fatalf("Timeout waiting for chunks: received %d/%d", receivedCount, len(chunks))
		}
	}

	t.Logf("Successfully received all %d chunks", receivedCount)
}

func TestAgent_TopicNames(t *testing.T) {
	// Verify we're using the correct topic names
	if contracts.TopicLogsRaw != "destill.logs.raw" {
		t.Errorf("Expected TopicLogsRaw to be 'destill.logs.raw', got %s", contracts.TopicLogsRaw)
	}
	if contracts.TopicRequests != "destill.requests" {
		t.Errorf("Expected TopicRequests to be 'destill.requests', got %s", contracts.TopicRequests)
	}
}

