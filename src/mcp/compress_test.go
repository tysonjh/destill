package mcp

import "testing"

func TestStripTimestamps(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ISO timestamp with T separator",
			input:    "2024-05-21T10:00:05.123Z [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "ISO timestamp with space separator",
			input:    "2024-05-21 10:00:05,123 [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "timestamp with timezone offset",
			input:    "2024-05-21T10:00:05+00:00 [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "no timestamp",
			input:    "[ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "timestamp mid-line preserved",
			input:    "Error at 2024-05-21T10:00:05Z in module",
			expected: "Error at 2024-05-21T10:00:05Z in module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTimestamps(tt.input)
			if result != tt.expected {
				t.Errorf("stripTimestamps(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
