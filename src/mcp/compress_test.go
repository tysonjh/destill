package mcp

import (
	"testing"
)

// Tests for the public compression API.
// Internal compression logic is tested in the patterns package.

func TestCompressLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "combined compression",
			input:    "2024-05-21T10:00:05.123Z /var/lib/jenkins/workspace/pipeline/src/test/AuthTest.java:45 - Container abc123def456789 failed",
			expected: ".../AuthTest.java:45 - Container <HASH> failed",
		},
		{
			name:     "timestamp stripped",
			input:    "2024-05-21T10:00:05.123Z [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "path compressed preserving line number",
			input:    "/var/lib/jenkins/workspace/pipeline-123/src/test/java/com/app/AuthTest.java:45",
			expected: ".../AuthTest.java:45",
		},
		{
			name:     "hash masked",
			input:    "Container abc123def456789 failed",
			expected: "Container <HASH> failed",
		},
		{
			name:     "whitespace normalized",
			input:    "Error    in     module",
			expected: "Error in module",
		},
		{
			name:     "uuid masked",
			input:    "Request 550e8400-e29b-41d4-a716-446655440000 failed",
			expected: "Request <UUID> failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompressLine(tt.input)
			if result != tt.expected {
				t.Errorf("CompressLine(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompressContextLines(t *testing.T) {
	lines := []string{
		"2024-05-21T10:00:01.000Z [INFO] [com.mycompany.runner.Executor] Starting test",
		"2024-05-21T10:00:02.000Z [INFO] [com.mycompany.runner.Executor] Running test",
		"2024-05-21T10:00:03.000Z [INFO] [com.mycompany.runner.Executor] Test failed",
	}

	result := CompressContextLines(lines)

	// Should remove timestamps and common prefix
	expected := []string{
		"... Starting test",
		"... Running test",
		"... Test failed",
	}

	if len(result) != len(expected) {
		t.Fatalf("len = %d, expected %d", len(result), len(expected))
	}

	for i, line := range result {
		if line != expected[i] {
			t.Errorf("line[%d] = %q, expected %q", i, line, expected[i])
		}
	}
}

func TestCompressContextLines_Empty(t *testing.T) {
	result := CompressContextLines([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestCompressContextLines_SingleLine(t *testing.T) {
	lines := []string{"2024-05-21T10:00:00Z Single line with timestamp"}
	result := CompressContextLines(lines)

	// Single line - no prefix removal, just per-line compression
	if len(result) != 1 {
		t.Fatalf("expected 1 line, got %d", len(result))
	}
	if result[0] != "Single line with timestamp" {
		t.Errorf("expected timestamp stripped, got %q", result[0])
	}
}
