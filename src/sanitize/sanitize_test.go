package sanitize

import "testing"

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "color codes",
			input:    "\x1b[31mERROR\x1b[0m: something failed",
			expected: "ERROR: something failed",
		},
		{
			name:     "no ANSI",
			input:    "plain text message",
			expected: "plain text message",
		},
		{
			name:     "multiple codes",
			input:    "\x1b[1m\x1b[31mbold red\x1b[0m normal",
			expected: "bold red normal",
		},
		{
			name:     "buildkite timestamp marker",
			input:    "\x1b_bk;t=1765886936038\x07[ERROR] message",
			expected: "[ERROR] message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestClean(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "full cleanup",
			input:    "\x1b_bk;t=123\x07\x1b[31mERROR\x1b[0m: message\r\n",
			expected: "ERROR: message",
		},
		{
			name:     "carriage returns",
			input:    "line1\r\nline2\r",
			expected: "line1\nline2",
		},
		{
			name:     "already clean",
			input:    "clean message",
			expected: "clean message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Clean(tt.input)
			if result != tt.expected {
				t.Errorf("Clean(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
