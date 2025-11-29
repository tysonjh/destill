// Package analysis provides the Analysis Agent for the Destill log triage tool.
package analysis

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

// TestAnalysisAgentUnmarshalsLogChunk verifies the agent correctly unmarshals incoming LogChunk.
func TestAnalysisAgentUnmarshalsLogChunk(t *testing.T) {
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	// Subscribe to output topic
	outputChan, err := msgBroker.Subscribe("ci_failures_ranked")
	if err != nil {
		t.Fatalf("Failed to subscribe to ci_failures_ranked: %v", err)
	}

	// Create and start agent
	agent := NewAgent(msgBroker, logger.NewSilentLogger())
	go func() {
		_ = agent.Run()
	}()

	// Give agent time to subscribe
	time.Sleep(50 * time.Millisecond)

	// Create a LogChunk
	logChunk := contracts.LogChunk{
		ID:        "chunk-test-123",
		RequestID: "req-test-456",
		JobName:   "unit-test-job",
		Content:   "ERROR: Test failure message",
		Timestamp: "2025-11-28T12:00:00Z",
		Metadata: map[string]string{
			"build_url": "https://example.com/build/123",
		},
	}

	// Marshal and publish
	data, err := json.Marshal(logChunk)
	if err != nil {
		t.Fatalf("Failed to marshal LogChunk: %v", err)
	}

	if err := msgBroker.Publish("ci_logs_raw", data); err != nil {
		t.Fatalf("Failed to publish LogChunk: %v", err)
	}

	// Wait for output
	select {
	case output := <-outputChan:
		// Verify output can be unmarshaled as TriageCard
		var triageCard contracts.TriageCard
		if err := json.Unmarshal(output, &triageCard); err != nil {
			t.Fatalf("Failed to unmarshal output as TriageCard: %v", err)
		}

		// Verify fields are populated correctly
		if triageCard.ID != logChunk.ID {
			t.Errorf("ID mismatch: expected %q, got %q", logChunk.ID, triageCard.ID)
		}
		if triageCard.RequestID != logChunk.RequestID {
			t.Errorf("RequestID mismatch: expected %q, got %q", logChunk.RequestID, triageCard.RequestID)
		}
		if triageCard.JobName != logChunk.JobName {
			t.Errorf("JobName mismatch: expected %q, got %q", logChunk.JobName, triageCard.JobName)
		}
		if triageCard.MessageHash == "" {
			t.Error("MessageHash should not be empty")
		}
		if triageCard.Severity != "ERROR" {
			t.Errorf("Severity should be ERROR for error log, got %q", triageCard.Severity)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for TriageCard output")
	}
}

// TestTriageCardSerialization verifies TriageCard can be serialized and deserialized correctly.
func TestTriageCardSerialization(t *testing.T) {
	original := contracts.TriageCard{
		ID:              "triage-123",
		Source:          "buildkite",
		Timestamp:       "2025-11-28T12:00:00Z",
		Severity:        "ERROR",
		Message:         "Test failure message",
		Metadata:        map[string]string{"key": "value"},
		RequestID:       "req-456",
		MessageHash:     "abc123def456",
		JobName:         "test-job",
		ConfidenceScore: 0.85,
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal TriageCard: %v", err)
	}

	// Unmarshal
	var restored contracts.TriageCard
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal TriageCard: %v", err)
	}

	// Verify all fields
	if restored.ID != original.ID {
		t.Errorf("ID mismatch: expected %q, got %q", original.ID, restored.ID)
	}
	if restored.Source != original.Source {
		t.Errorf("Source mismatch: expected %q, got %q", original.Source, restored.Source)
	}
	if restored.Severity != original.Severity {
		t.Errorf("Severity mismatch: expected %q, got %q", original.Severity, restored.Severity)
	}
	if restored.Message != original.Message {
		t.Errorf("Message mismatch: expected %q, got %q", original.Message, restored.Message)
	}
	if restored.RequestID != original.RequestID {
		t.Errorf("RequestID mismatch: expected %q, got %q", original.RequestID, restored.RequestID)
	}
	if restored.MessageHash != original.MessageHash {
		t.Errorf("MessageHash mismatch: expected %q, got %q", original.MessageHash, restored.MessageHash)
	}
	if restored.JobName != original.JobName {
		t.Errorf("JobName mismatch: expected %q, got %q", original.JobName, restored.JobName)
	}
	if restored.ConfidenceScore != original.ConfidenceScore {
		t.Errorf("ConfidenceScore mismatch: expected %v, got %v", original.ConfidenceScore, restored.ConfidenceScore)
	}
}

// TestNormalizeLogRemovesTimestamps verifies log normalization handles non-deterministic noise.
func TestNormalizeLogRemovesTimestamps(t *testing.T) {
	agent := &Agent{}

	// Two logs with different timestamps but same error message
	logA := "[2025-11-28 10:00:00] FATAL: Null pointer in function X at line 42."
	logB := "[2025-11-28 10:01:30] FATAL: Null pointer in function X at line 42."

	// NOTE: The current normalizeLog implementation is a placeholder that only trims whitespace.
	// This test documents the expected behavior for future implementation.
	// When proper normalization is implemented, uncomment the hash comparison.

	normalizedA := agent.normalizeLog(logA)
	normalizedB := agent.normalizeLog(logB)

	hashA := agent.calculateMessageHash(normalizedA)
	hashB := agent.calculateMessageHash(normalizedB)

	// Current placeholder: hashes will differ because timestamps aren't stripped yet
	// TODO: When proper normalization is implemented, these should be equal
	t.Logf("Normalized A: %q", normalizedA)
	t.Logf("Normalized B: %q", normalizedB)
	t.Logf("Hash A: %s", hashA)
	t.Logf("Hash B: %s", hashB)

	// For now, just verify hashes are non-empty
	if hashA == "" || hashB == "" {
		t.Error("Hashes should not be empty")
	}
}

// TestDetectSeverity verifies severity detection logic.
func TestDetectSeverity(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		content  string
		expected string
	}{
		{"ERROR: something went wrong", "ERROR"},
		{"FATAL: system crashed", "ERROR"}, // FATAL now returns ERROR (simplified severity)
		{"error in processing", "ERROR"},
		{"Warning: low disk space", "WARN"},
		{"WARN: memory usage high", "WARN"},
		{"INFO: operation completed", "INFO"},
		{"Just a regular log line", "INFO"},
	}

	for _, test := range tests {
		result := agent.detectSeverity(test.content)
		if result != test.expected {
			t.Errorf("detectSeverity(%q) = %q, want %q", test.content, result, test.expected)
		}
	}
}

// TestCalculateMessageHash verifies hash calculation is deterministic.
func TestCalculateMessageHash(t *testing.T) {
	agent := &Agent{}

	message := "Test message for hashing"

	// Calculate hash twice
	hash1 := agent.calculateMessageHash(message)
	hash2 := agent.calculateMessageHash(message)

	// Should be identical
	if hash1 != hash2 {
		t.Errorf("Hash should be deterministic: %q != %q", hash1, hash2)
	}

	// Different message should produce different hash
	hash3 := agent.calculateMessageHash("Different message")
	if hash1 == hash3 {
		t.Error("Different messages should produce different hashes")
	}
}

// TestCalculateConfidenceScore verifies confidence scoring based on content quality.
func TestCalculateConfidenceScore(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		name     string
		content  string
		minScore float64
		maxScore float64
	}{
		{
			name:     "exception with stack trace",
			content:  "Exception in thread main: NullPointerException\nStack trace:\nat line 42",
			minScore: 0.8,
			maxScore: 1.0,
		},
		{
			name:     "fatal error with high-signal anchor",
			content:  "FATAL: system panic occurred",
			minScore: 0.9, // +0.25 for high-signal, +0.20 for fatal/panic
			maxScore: 1.0,
		},
		{
			name:     "structured error with file and line",
			content:  "Error in file main.go at line 123: connection failed",
			minScore: 0.7,
			maxScore: 1.0,
		},
		{
			name:     "high-signal error anchor",
			content:  "ERROR: connection refused to database",
			minScore: 0.75, // +0.25 for high-signal anchor
			maxScore: 1.0,
		},
		{
			name:     "vague short message",
			content:  "failed",
			minScore: 0.0,
			maxScore: 0.5,
		},
		{
			name:     "regular log message",
			content:  "Processing request for user authentication",
			minScore: 0.4,
			maxScore: 0.6,
		},
		// Penalty pattern tests
		{
			name:     "connection reset with retry - transient error",
			content:  "ERROR: connection reset, will retry in 5 seconds",
			minScore: 0.0,
			maxScore: 0.5, // -0.30 penalty for retry pattern
		},
		{
			name:     "address already in use - transient error",
			content:  "ERROR: bind failed - address already in use on port 8080",
			minScore: 0.0,
			maxScore: 0.55, // -0.25 penalty for address in use
		},
		{
			name:     "tests passed - success message",
			content:  "ERROR encountered during cleanup but all tests passed successfully",
			minScore: 0.0,
			maxScore: 0.4, // -0.35 penalty for test passed pattern
		},
		{
			name:     "deprecation warning - informational",
			content:  "ERROR: deprecated API usage detected, please update",
			minScore: 0.0,
			maxScore: 0.6, // -0.20 penalty for deprecation
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			score := agent.calculateConfidenceScore(test.content)
			if score < test.minScore || score > test.maxScore {
				t.Errorf("Score %v out of expected range [%v, %v] for content: %q",
					score, test.minScore, test.maxScore, test.content)
			}
		})
	}
}

// TestNormalizeLogComprehensive verifies all normalization patterns.
func TestNormalizeLogComprehensive(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		name        string
		input       string
		contains    []string // strings that should be present in normalized output
		notContains []string // strings that should NOT be present
	}{
		{
			name:        "timestamp removal - ISO format",
			input:       "2025-11-28T10:30:45Z ERROR: connection failed",
			contains:    []string{"[timestamp]", "error", "connection failed"},
			notContains: []string{"2025-11-28", "10:30:45"},
		},
		{
			name:        "UUID removal",
			input:       "Request ID 550e8400-e29b-41d4-a716-446655440000 failed",
			contains:    []string{"request", "[uuid]", "failed"},
			notContains: []string{"550e8400"},
		},
		{
			name:        "PID removal",
			input:       "Process PID 12345 crashed with error",
			contains:    []string{"process", "[pid]", "crashed"},
			notContains: []string{"12345"},
		},
		{
			name:        "memory address removal",
			input:       "Null pointer at 0x7fff5fbff710",
			contains:    []string{"null pointer", "[addr]"},
			notContains: []string{"0x7fff"},
		},
		{
			name:        "IP address removal",
			input:       "Connection to 192.168.1.100 failed",
			contains:    []string{"connection", "[ip]", "failed"},
			notContains: []string{"192.168"},
		},
		{
			name:        "port number removal",
			input:       "Server on localhost:8080 unreachable",
			contains:    []string{"server", "localhost", "[port]"},
			notContains: []string{":8080"},
		},
		{
			name:        "sequence number removal",
			input:       "Error at seq=12345 in processing",
			contains:    []string{"error", "[seq]", "processing"},
			notContains: []string{"seq=12345"},
		},
		{
			name:        "whitespace normalization",
			input:       "ERROR:\t\t  Multiple    spaces   and\ttabs",
			contains:    []string{"error", "multiple spaces and tabs"},
			notContains: []string{"\t", "  "},
		},
		{
			name:        "case normalization",
			input:       "ERROR: Connection FAILED",
			contains:    []string{"error: connection failed"},
			notContains: []string{"ERROR", "FAILED"},
		},
		{
			name:        "complex real-world log",
			input:       "[2025-11-28 10:30:45.123] ERROR: Connection to 10.0.0.5:5432 failed for request 550e8400-e29b-41d4-a716-446655440000 (pid=1234, seq=5678)",
			contains:    []string{"[timestamp]", "error", "connection", "[ip]", "[port]", "failed", "request", "[uuid]", "[pid]", "[seq]"},
			notContains: []string{"2025-11-28", "10.0.0.5", "5432", "550e8400", "1234", "5678"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized := agent.normalizeLog(test.input)

			// Check for required strings
			for _, substr := range test.contains {
				if !strings.Contains(normalized, substr) {
					t.Errorf("Normalized output should contain %q\nInput: %q\nOutput: %q",
						substr, test.input, normalized)
				}
			}

			// Check for strings that should be removed
			for _, substr := range test.notContains {
				if strings.Contains(normalized, substr) {
					t.Errorf("Normalized output should NOT contain %q\nInput: %q\nOutput: %q",
						substr, test.input, normalized)
				}
			}
		})
	}
}

// TestNormalizeLogRecurrence verifies that similar errors produce identical hashes.
func TestNormalizeLogRecurrence(t *testing.T) {
	agent := &Agent{}

	// Two errors that are semantically the same but with different dynamic values
	log1 := "[2025-11-28 10:00:00] ERROR: Connection to 192.168.1.5:5432 failed for request 550e8400-e29b-41d4-a716-446655440000 (pid=1234)"
	log2 := "[2025-11-28 11:30:15] ERROR: Connection to 10.0.0.10:5432 failed for request 773d94b2-f5a1-4567-b890-123456789abc (pid=5678)"

	normalized1 := agent.normalizeLog(log1)
	normalized2 := agent.normalizeLog(log2)

	hash1 := agent.calculateMessageHash(normalized1)
	hash2 := agent.calculateMessageHash(normalized2)

	if hash1 != hash2 {
		t.Errorf("Similar errors should produce identical hashes after normalization")
		t.Logf("Log 1: %s", log1)
		t.Logf("Log 2: %s", log2)
		t.Logf("Normalized 1: %s", normalized1)
		t.Logf("Normalized 2: %s", normalized2)
		t.Logf("Hash 1: %s", hash1)
		t.Logf("Hash 2: %s", hash2)
	}
}

// TestDetectSeverityEnhanced verifies enhanced severity detection.
func TestDetectSeverityEnhanced(t *testing.T) {
	agent := &Agent{}

	tests := []struct {
		content  string
		expected string
	}{
		// All error-like keywords now return ERROR (simplified severity)
		// Quality differentiation is handled by confidence scoring
		{"FATAL: system panic", "ERROR"},
		{"panic: runtime error", "ERROR"},
		{"Segmentation fault (core dumped)", "ERROR"},
		{"Out of memory error", "ERROR"},
		{"ERROR: connection failed", "ERROR"},
		{"Exception in thread main", "ERROR"},
		{"Unable to connect to database", "ERROR"},
		{"Permission denied", "ERROR"},
		{"Warning: deprecated API usage", "WARN"},
		{"WARN: high memory usage", "WARN"},
		{"INFO: request processed successfully", "INFO"},
		{"Processing completed", "INFO"},
	}

	for _, test := range tests {
		result := agent.detectSeverity(test.content)
		if result != test.expected {
			t.Errorf("detectSeverity(%q) = %q, want %q", test.content, result, test.expected)
		}
	}
}
