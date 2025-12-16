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

func TestMaskHashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "container ID",
			input:    "Container abc123def456789 failed to start",
			expected: "Container <HASH> failed to start",
		},
		{
			name:     "git SHA",
			input:    "Commit 1a2b3c4d5e6f7890abcdef1234567890abcdef12 broke tests",
			expected: "Commit <HASH> broke tests",
		},
		{
			name:     "multiple hashes",
			input:    "Image abc123def456789:latest on host def456abc789012",
			expected: "Image <HASH>:latest on host <HASH>",
		},
		{
			name:     "short hex preserved",
			input:    "Error code 0x1234 returned",
			expected: "Error code 0x1234 returned",
		},
		{
			name:     "no hashes",
			input:    "Connection failed to server",
			expected: "Connection failed to server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskHashes(tt.input)
			if result != tt.expected {
				t.Errorf("maskHashes(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCompressPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long absolute path",
			input:    "/var/lib/jenkins/workspace/pipeline-123/src/test/java/com/app/AuthTest.java:45",
			expected: ".../AuthTest.java:45",
		},
		{
			name:     "path with line reference",
			input:    "File /home/user/project/src/main/Service.go:123 - error",
			expected: "File .../Service.go:123 - error",
		},
		{
			name:     "short path preserved",
			input:    "src/main.go:10 - warning",
			expected: "src/main.go:10 - warning",
		},
		{
			name:     "no path",
			input:    "Connection refused",
			expected: "Connection refused",
		},
		{
			name:     "multiple paths",
			input:    "/a/b/c/file1.go:1 imports /d/e/f/file2.go:2",
			expected: ".../file1.go:1 imports .../file2.go:2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compressPath(tt.input)
			if result != tt.expected {
				t.Errorf("compressPath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFindCommonPrefix(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name: "Java logger prefix",
			lines: []string{
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Starting",
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Pulling",
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Failed",
			},
			expected: "[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] ",
		},
		{
			name: "no common prefix",
			lines: []string{
				"Starting container",
				"Pulling image",
				"Container failed",
			},
			expected: "",
		},
		{
			name: "short common prefix ignored",
			lines: []string{
				"[INFO] Starting",
				"[INFO] Stopping",
			},
			expected: "", // Too short (< 20 chars) to be worth removing
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: "",
		},
		{
			name:     "single line",
			lines:    []string{"[INFO] Single line"},
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

func TestRemoveCommonPrefix(t *testing.T) {
	lines := []string{
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Starting container",
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Pulling image",
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Container failed",
	}

	result := removeCommonPrefix(lines)

	expected := []string{
		"... Starting container",
		"... Pulling image",
		"... Container failed",
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

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "Error    in     module",
			expected: "Error in module",
		},
		{
			name:     "tabs to spaces",
			input:    "Error\tin\tmodule",
			expected: "Error in module",
		},
		{
			name:     "leading/trailing spaces",
			input:    "   Error in module   ",
			expected: "Error in module",
		},
		{
			name:     "mixed whitespace",
			input:    "  Error  \t  in \t module  ",
			expected: "Error in module",
		},
		{
			name:     "already normalized",
			input:    "Error in module",
			expected: "Error in module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWhitespace(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeWhitespace(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
