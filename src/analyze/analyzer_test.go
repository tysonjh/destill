package analyze

import (
	"fmt"
	"strings"
	"testing"

	"destill-agent/src/contracts"
)

func TestDetectSeverity(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"FATAL: System crashed", "FATAL"},
		{"PANIC: Out of memory", "FATAL"},
		{"ERROR: Connection failed", "ERROR"},
		{"Exception in thread main", "ERROR"},
		{"WARN: Deprecated function", "WARN"},
		{"INFO: Starting server", "INFO"},
		{"Debug: processing item", "INFO"},
	}

	for _, tt := range tests {
		result := detectSeverity(tt.line)
		if result != tt.expected {
			t.Errorf("detectSeverity(%q) = %q, expected %q", tt.line, result, tt.expected)
		}
	}
}

func TestCalculateConfidence(t *testing.T) {
	tests := []struct {
		line     string
		severity string
		minScore float64
		maxScore float64
	}{
		{"ERROR: Something went wrong", "ERROR", 0.6, 1.0},
		{"FATAL: Critical failure", "FATAL", 0.7, 1.0},
		{"Test passed successfully", "ERROR", 0.0, 0.5},
		{"Deprecated function warning", "WARN", 0.0, 0.5},
		{"Connection error, will retry", "ERROR", 0.4, 0.7},
	}

	for _, tt := range tests {
		score := calculateConfidence(tt.line, tt.severity)
		if score < tt.minScore || score > tt.maxScore {
			t.Errorf("calculateConfidence(%q, %q) = %.2f, expected between %.2f and %.2f",
				tt.line, tt.severity, score, tt.minScore, tt.maxScore)
		}
	}
}

func TestNormalizeMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			"Error at 2025-11-28T10:30:45Z",
			"Error at [TIMESTAMP]",
		},
		{
			"UUID: 550e8400-e29b-41d4-a716-446655440000",
			"UUID: [UUID]",
		},
		{
			"Memory address: 0x7fff5fbff710",
			"Memory address: [HEX]",
		},
		{
			"Error code 500 on line 123",
			"Error code [NUM] on line [NUM]",
		},
	}

	for _, tt := range tests {
		result := normalizeMessage(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeMessage(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestExtractContext(t *testing.T) {
	lines := []string{
		"line 0",
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"ERROR: Something went wrong", // line 5
		"line 6",
		"line 7",
		"line 8",
		"line 9",
	}

	// Test normal case (not at boundaries)
	pre, post, note := extractContext(lines, 5)

	if len(pre) != 5 {
		t.Errorf("Expected 5 pre-context lines, got %d", len(pre))
	}
	if len(post) != 4 { // Only 4 lines after in this small sample
		t.Errorf("Expected 4 post-context lines, got %d", len(post))
	}
	// Note should indicate truncation at end since post-context is incomplete
	if note == "" {
		t.Error("Expected truncation note")
	}

	// Test at chunk start
	pre, _, note = extractContext(lines, 0)
	if len(pre) != 0 {
		t.Errorf("Expected 0 pre-context lines at start, got %d", len(pre))
	}
	if note == "" {
		t.Error("Expected truncation note at start")
	}

	// Test at chunk end
	_, post, note = extractContext(lines, len(lines)-1)
	if len(post) != 0 {
		t.Errorf("Expected 0 post-context lines at end, got %d", len(post))
	}
	if note == "" {
		t.Error("Expected truncation note at end")
	}
}

func TestAnalyzeChunk_Empty(t *testing.T) {
	chunk := contracts.LogChunkV2{
		Content: "",
	}

	findings := AnalyzeChunk(chunk)
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for empty chunk, got %d", len(findings))
	}
}

func TestAnalyzeChunk_NoErrors(t *testing.T) {
	chunk := contracts.LogChunkV2{
		Content: "INFO: Starting process\nINFO: Running task\nINFO: Completed successfully",
	}

	findings := AnalyzeChunk(chunk)
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for info-only chunk, got %d", len(findings))
	}
}

func TestAnalyzeChunk_WithErrors(t *testing.T) {
	content := `INFO: Starting process
INFO: Connecting to database
ERROR: Connection timeout after 30 seconds
FATAL: Unable to start application
INFO: Cleanup started`

	chunk := contracts.LogChunkV2{
		RequestID:   "req-1",
		BuildID:     "build-1",
		JobName:     "test-job",
		JobID:       "job-1",
		ChunkIndex:  0,
		TotalChunks: 1,
		Content:     content,
		LineStart:   1,
		LineEnd:     5,
		Metadata:    map[string]string{"build_url": "https://example.com"},
	}

	findings := AnalyzeChunk(chunk)

	// Should find both ERROR and FATAL
	if len(findings) < 2 {
		t.Fatalf("Expected at least 2 findings, got %d", len(findings))
	}

	// Verify first finding (ERROR)
	if findings[0].Severity != "ERROR" && findings[1].Severity != "ERROR" {
		t.Error("Expected to find ERROR severity")
	}

	// Verify second finding (FATAL)
	if findings[0].Severity != "FATAL" && findings[1].Severity != "FATAL" {
		t.Error("Expected to find FATAL severity")
	}

	// Verify context extraction
	for i, f := range findings {
		if len(f.PreContext) == 0 {
			t.Errorf("Finding %d: expected pre-context", i)
		}
		if f.ConfidenceScore <= 0 {
			t.Errorf("Finding %d: confidence score should be > 0, got %.2f", i, f.ConfidenceScore)
		}
	}
}

func TestAnalyzeChunk_LargeChunk(t *testing.T) {
	// Create a large chunk with multiple errors
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("INFO: Processing item %d", i))
		if i%10 == 0 && i > 0 {
			lines = append(lines, fmt.Sprintf("ERROR: Failed to process item %d", i))
		}
	}

	chunk := contracts.LogChunkV2{
		Content:   strings.Join(lines, "\n"),
		LineStart: 1,
	}

	findings := AnalyzeChunk(chunk)

	// Should find multiple errors
	if len(findings) < 5 {
		t.Errorf("Expected at least 5 findings in large chunk, got %d", len(findings))
	}

	// Verify all findings have reasonable data
	for i, f := range findings {
		if f.RawMessage == "" {
			t.Errorf("Finding %d: raw message is empty", i)
		}
		if f.NormalizedMsg == "" {
			t.Errorf("Finding %d: normalized message is empty", i)
		}
		if f.LineNumber < 1 {
			t.Errorf("Finding %d: invalid line number %d", i, f.LineNumber)
		}
	}
}

func TestConvertToTriageCard(t *testing.T) {
	finding := Finding{
		LineNumber:      10,
		RawMessage:      "ERROR: Connection failed",
		NormalizedMsg:   "ERROR: Connection failed",
		Severity:        "ERROR",
		ConfidenceScore: 0.85,
		PreContext:      []string{"line 1", "line 2"},
		PostContext:     []string{"line 3", "line 4"},
		ContextNote:     "complete",
	}

	chunk := contracts.LogChunkV2{
		RequestID:  "req-123",
		BuildID:    "build-456",
		JobName:    "test-job",
		JobID:      "job-789",
		ChunkIndex: 0,
		LineStart:  1,
		Metadata:   map[string]string{"build_url": "https://example.com"},
	}

	card := ConvertToTriageCard(finding, chunk, "req-123")

	// Verify card fields
	if card.RequestID != "req-123" {
		t.Errorf("Expected request ID req-123, got %s", card.RequestID)
	}
	if card.JobName != "test-job" {
		t.Errorf("Expected job name test-job, got %s", card.JobName)
	}
	if card.Severity != "ERROR" {
		t.Errorf("Expected severity ERROR, got %s", card.Severity)
	}
	if card.ConfidenceScore != 0.85 {
		t.Errorf("Expected confidence 0.85, got %.2f", card.ConfidenceScore)
	}
	if len(card.PreContext) != 2 {
		t.Errorf("Expected 2 pre-context lines, got %d", len(card.PreContext))
	}
	if len(card.PostContext) != 2 {
		t.Errorf("Expected 2 post-context lines, got %d", len(card.PostContext))
	}
	if card.MessageHash == "" {
		t.Error("Expected message hash to be set")
	}
}

func TestCalculateMessageHash(t *testing.T) {
	msg1 := "ERROR: Connection failed"
	msg2 := "ERROR: Connection failed"
	msg3 := "ERROR: Different error"

	hash1 := CalculateMessageHash(msg1)
	hash2 := CalculateMessageHash(msg2)
	hash3 := CalculateMessageHash(msg3)

	// Same messages should produce same hash
	if hash1 != hash2 {
		t.Error("Expected same hash for identical messages")
	}

	// Different messages should produce different hashes
	if hash1 == hash3 {
		t.Error("Expected different hash for different messages")
	}

	// Hash should be hex string
	if len(hash1) != 64 { // SHA256 produces 64 hex characters
		t.Errorf("Expected hash length 64, got %d", len(hash1))
	}
}
