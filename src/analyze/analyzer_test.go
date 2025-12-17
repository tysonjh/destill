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

func TestCalculateConfidence_BoostPatterns(t *testing.T) {
	// Test that high-signal patterns get boosted confidence
	tests := []struct {
		name     string
		line     string
		severity string
		minScore float64
	}{
		// Stack traces
		{"Java stack trace", "	at com.example.MyClass.method(MyClass.java:42)", "ERROR", 0.75},
		{"Python traceback", "Traceback (most recent call last):", "ERROR", 0.75},
		{"Python file line", `  File "/app/main.py", line 42, in func`, "ERROR", 0.75},
		{"Go panic", "panic: runtime error: invalid memory address", "FATAL", 0.85},
		{"C++ backtrace", "Backtrace:", "ERROR", 0.75},
		{"C++ stack frame", "#0  0x00007f3c in myfunction", "ERROR", 0.75},
		{"C++ terminate", "terminate called after throwing", "ERROR", 0.75},

		// Build tool errors
		{"npm error", "npm ERR! code ENOENT", "ERROR", 0.80},
		{"npm ELIFECYCLE", "ERROR: ELIFECYCLE exit code 1", "ERROR", 0.75},
		{"Maven failure", "[ERROR] BUILD FAILURE", "ERROR", 0.80},
		{"Gradle failure", "FAILURE: Build failed with an exception", "ERROR", 0.80},

		// Docker/K8s
		{"Docker error", "Error response from daemon: pull access denied", "ERROR", 0.75},
		{"K8s CrashLoop", "ERROR: CrashLoopBackOff for pod nginx", "ERROR", 0.80},
		{"K8s OOMKilled", "ERROR: Container killed OOMKilled", "ERROR", 0.80},

		// Crashes
		{"OOM", "ERROR: OutOfMemoryError: Java heap space", "ERROR", 0.85},
		{"Segfault", "ERROR: Segmentation fault (core dumped)", "ERROR", 0.85},
		{"SIGKILL", "Process killed by SIGKILL", "ERROR", 0.85},

		// Exit codes
		{"Exit code", "ERROR: Process exited with code 1", "ERROR", 0.75},
		{"Non-zero exit", "FATAL: non-zero exit code returned", "FATAL", 0.85},

		// Compilation
		{"Compile error", "ERROR: cannot find symbol: class Foo", "ERROR", 0.75},
		{"Syntax error", "SyntaxError: unexpected token '<'", "ERROR", 0.70},
		{"Import error", "ModuleNotFoundError: No module named 'foo'", "ERROR", 0.70},

		// Other high-signal
		{"Permission denied", "ERROR: Permission denied accessing /etc/passwd", "ERROR", 0.70},
		{"Connection refused", "ERROR: Connection refused to localhost:5432", "ERROR", 0.70},
		{"Timeout", "ERROR: Operation timed out after 30s", "ERROR", 0.70},
		{"Assertion", "AssertionError: expected true but got false", "ERROR", 0.80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateConfidence(tt.line, tt.severity)
			if score < tt.minScore {
				t.Errorf("calculateConfidence(%q) = %.2f, expected >= %.2f (should be boosted)",
					tt.line, score, tt.minScore)
			}
		})
	}
}

func TestCalculateConfidence_PenaltyPatterns(t *testing.T) {
	// Test that false-positive patterns get penalized
	tests := []struct {
		name     string
		line     string
		severity string
		maxScore float64
	}{
		// Success messages with "error" word
		{"Zero errors", "Build completed with 0 errors", "ERROR", 0.30},
		{"No errors", "Compilation finished: no errors found", "ERROR", 0.30},
		{"Errors: 0", "Test results: errors: 0, warnings: 2", "ERROR", 0.30},

		// Test expectations
		{"Expect error", "expect(fn).toThrow(Error)", "ERROR", 0.40},
		{"Should fail", "it should.fail when given invalid input", "ERROR", 0.40},
		{"Assert throws", "assert.throws(() => fn(), Error)", "ERROR", 0.45},

		// Handled errors
		{"Caught error", "ERROR caught and handled gracefully", "ERROR", 0.45},
		{"Recovered", "ERROR recovered from panic successfully", "ERROR", 0.45},

		// Variable names
		{"errorHandler", "const errorHandler = new ErrorHandler()", "ERROR", 0.50},
		{"getError", "result.getError() returned null", "ERROR", 0.50},
		{"isError", "if (response.isError()) return", "ERROR", 0.50},

		// Retry success
		{"Succeeded after retry", "Operation succeeded after retry attempt 3", "ERROR", 0.40},
		{"Passed on retry", "Test passed on retry", "ERROR", 0.40},

		// Comments
		{"Code comment", "// ERROR: This should never happen", "ERROR", 0.60},
		{"Hash comment", "# ERROR handling code below", "ERROR", 0.45},

		// Quoted levels (format strings)
		{"Quoted ERROR", `log.SetLevel("ERROR")`, "ERROR", 0.45},
		{"Quoted FATAL", `logger.level = 'FATAL'`, "FATAL", 0.55},

		// Help text
		{"Usage text", "Usage: command [OPTIONS] ERROR_FILE", "ERROR", 0.50},
		{"Documentation", "See documentation for error handling", "ERROR", 0.50},

		// Combined penalties
		{"Test passed", "All 42 tests passed, 0 errors", "ERROR", 0.15},
		{"Deprecated", "WARN: deprecated function 'oldMethod'", "WARN", 0.30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateConfidence(tt.line, tt.severity)
			if score > tt.maxScore {
				t.Errorf("calculateConfidence(%q) = %.2f, expected <= %.2f (should be penalized)",
					tt.line, score, tt.maxScore)
			}
		})
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
	chunk := contracts.LogChunk{
		Content: "",
	}

	findings := AnalyzeChunk(chunk)
	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for empty chunk, got %d", len(findings))
	}
}

func TestAnalyzeChunk_NoErrors(t *testing.T) {
	chunk := contracts.LogChunk{
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

	chunk := contracts.LogChunk{
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

	chunk := contracts.LogChunk{
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

	chunk := contracts.LogChunk{
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
