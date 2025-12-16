package patterns

import (
	"testing"
)

func TestNormalize_Presentation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "timestamp stripped from start",
			input:    "2024-05-21T10:00:05.123Z [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "long path compressed preserving line number",
			input:    "/var/lib/jenkins/workspace/pipeline-123/src/test/java/com/app/AuthTest.java:45 - failed",
			expected: ".../AuthTest.java:45 - failed",
		},
		{
			name:     "long hash masked",
			input:    "Container abc123def456789 failed to start",
			expected: "Container <HASH> failed to start",
		},
		{
			name:     "UUID masked",
			input:    "Request 550e8400-e29b-41d4-a716-446655440000 failed",
			expected: "Request <UUID> failed",
		},
		{
			name:     "hex address masked",
			input:    "Pointer at 0x7fff5fbff8c0 is nil",
			expected: "Pointer at <HEX> is nil",
		},
		{
			name:     "numbers preserved in presentation",
			input:    "Error code 42 on line 100",
			expected: "Error code 42 on line 100",
		},
		{
			name:     "whitespace normalized",
			input:    "Error    in     module",
			expected: "Error in module",
		},
		{
			name:     "combined transforms",
			input:    "2024-05-21T10:00:05Z /var/lib/long/path/to/file.go:42 - Container abc123def456789 crashed",
			expected: ".../file.go:42 - Container <HASH> crashed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input, MaskPresentation)
			if result != tt.expected {
				t.Errorf("Normalize(MaskPresentation)\n  input:    %q\n  got:      %q\n  expected: %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalize_Recurrence(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "timestamp replaced with placeholder",
			input:    "2024-05-21T10:00:05.123Z Connection failed",
			expected: "[TIMESTAMP] Connection failed",
		},
		{
			name:     "path replaced entirely",
			input:    "/var/lib/jenkins/workspace/src/main.go:42 - error",
			expected: "[PATH] - error",
		},
		{
			name:     "all numbers masked including line numbers",
			input:    "Error code 42 on line 100",
			expected: "Error code [NUM] on line [NUM]",
		},
		{
			name:     "UUID replaced with placeholder",
			input:    "Request 550e8400-e29b-41d4-a716-446655440000 failed",
			expected: "Request [UUID] failed",
		},
		{
			name:     "hex address replaced",
			input:    "Pointer at 0x7fff5fbff8c0 is nil",
			expected: "Pointer at [HEX] is nil",
		},
		{
			name:     "long hash masked",
			input:    "Container abc123def456789 failed",
			expected: "Container <HASH> failed",
		},
		{
			name:     "combined transforms for grouping",
			input:    "2024-05-21T10:00:05Z Error on line 42: /var/lib/path/file.go",
			expected: "[TIMESTAMP] Error on line [NUM]: [PATH]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Normalize(tt.input, MaskRecurrence)
			if result != tt.expected {
				t.Errorf("Normalize(MaskRecurrence)\n  input:    %q\n  got:      %q\n  expected: %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeLines_Presentation(t *testing.T) {
	lines := []string{
		"2024-05-21T10:00:01.000Z [INFO] [com.mycompany.runner.Executor] Starting test",
		"2024-05-21T10:00:02.000Z [INFO] [com.mycompany.runner.Executor] Running test",
		"2024-05-21T10:00:03.000Z [INFO] [com.mycompany.runner.Executor] Test failed",
	}

	result := NormalizeLines(lines, MaskPresentation)

	// Should strip timestamps and remove common prefix
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

func TestNormalizeLines_Recurrence(t *testing.T) {
	lines := []string{
		"2024-05-21T10:00:01.000Z Error on line 42",
		"2024-05-21T10:00:02.000Z Error on line 50",
	}

	result := NormalizeLines(lines, MaskRecurrence)

	// Should replace timestamps and numbers, no prefix removal
	expected := []string{
		"[TIMESTAMP] Error on line [NUM]",
		"[TIMESTAMP] Error on line [NUM]",
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

func TestNormalizeLines_Empty(t *testing.T) {
	result := NormalizeLines([]string{}, MaskPresentation)
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

func TestFindCommonPrefix(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name: "long common prefix",
			lines: []string{
				"[INFO] [com.mycompany.runner.Executor] Starting",
				"[INFO] [com.mycompany.runner.Executor] Running",
				"[INFO] [com.mycompany.runner.Executor] Stopped",
			},
			expected: "[INFO] [com.mycompany.runner.Executor] ",
		},
		{
			name: "short prefix ignored",
			lines: []string{
				"[INFO] Start",
				"[INFO] Stop",
			},
			expected: "", // Less than minPrefixLength
		},
		{
			name: "no common prefix",
			lines: []string{
				"Starting server",
				"Connecting to database",
			},
			expected: "",
		},
		{
			name:     "single line",
			lines:    []string{"Only one line"},
			expected: "",
		},
		{
			name:     "empty",
			lines:    []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCommonPrefix(tt.lines)
			if result != tt.expected {
				t.Errorf("findCommonPrefix() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestLevelDifference demonstrates the key difference between levels:
// Presentation preserves line numbers, Recurrence masks them.
func TestLevelDifference_LineNumbers(t *testing.T) {
	input := "Error at main.go:42"

	presentation := Normalize(input, MaskPresentation)
	recurrence := Normalize(input, MaskRecurrence)

	// Presentation should preserve the line number
	if presentation != "Error at main.go:42" {
		t.Errorf("Presentation should preserve line number, got: %q", presentation)
	}

	// Recurrence should mask the line number
	if recurrence != "Error at main.go:[NUM]" {
		t.Errorf("Recurrence should mask line number, got: %q", recurrence)
	}
}

// TestLevelDifference_Paths demonstrates path handling difference.
func TestLevelDifference_Paths(t *testing.T) {
	input := "/var/lib/jenkins/workspace/src/main.go:42 crashed"

	presentation := Normalize(input, MaskPresentation)
	recurrence := Normalize(input, MaskRecurrence)

	// Presentation should shorten path but preserve filename:line
	if presentation != ".../main.go:42 crashed" {
		t.Errorf("Presentation should compress path, got: %q", presentation)
	}

	// Recurrence should replace entire path and mask numbers
	if recurrence != "[PATH] crashed" {
		t.Errorf("Recurrence should replace path entirely, got: %q", recurrence)
	}
}
