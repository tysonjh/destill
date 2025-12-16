package mcp

import (
	"regexp"
	"strings"
)

// timestampPattern matches leading timestamps in various formats:
// - 2024-05-21T10:00:05.123Z
// - 2024-05-21 10:00:05,123
// - 2024-05-21T10:00:05+00:00
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[.,]?\d*[Z]?([+-]\d{2}:?\d{2})?\s*`)

// stripTimestamps removes leading timestamps from a line.
func stripTimestamps(line string) string {
	return timestampPattern.ReplaceAllString(line, "")
}

// hashPattern matches hex strings of 12+ characters (container IDs, git SHAs, etc.)
var hashPattern = regexp.MustCompile(`\b[a-f0-9]{12,}\b`)

// maskHashes replaces long hex strings with <HASH>.
func maskHashes(line string) string {
	return hashPattern.ReplaceAllString(line, "<HASH>")
}

// longPathPattern matches absolute paths with 3+ directories.
// Captures the filename (and optional line number) at the end.
var longPathPattern = regexp.MustCompile(`/(?:[^/\s]+/){3,}([^/\s:]+(?::\d+)?)`)

// compressPath shortens long file paths to .../filename.
func compressPath(line string) string {
	return longPathPattern.ReplaceAllString(line, ".../$1")
}

// minPrefixLength is the minimum prefix length worth removing.
// Shorter prefixes don't save enough tokens to justify the "..." replacement.
const minPrefixLength = 20

// findCommonPrefix finds the longest common prefix across lines.
// Returns empty string if prefix is too short or lines are empty.
func findCommonPrefix(lines []string) string {
	if len(lines) < 2 {
		return ""
	}

	prefix := lines[0]
	for _, line := range lines[1:] {
		for len(prefix) > 0 && (len(line) < len(prefix) || line[:len(prefix)] != prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		if len(prefix) == 0 {
			break
		}
	}

	if len(prefix) < minPrefixLength {
		return ""
	}

	return prefix
}

// removeCommonPrefix replaces common prefix with "... " across lines.
func removeCommonPrefix(lines []string) []string {
	prefix := findCommonPrefix(lines)
	if prefix == "" {
		return lines
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "... " + line[len(prefix):]
	}
	return result
}

// whitespacePattern matches multiple consecutive whitespace characters.
var whitespacePattern = regexp.MustCompile(`\s+`)

// normalizeWhitespace collapses multiple spaces/tabs and trims.
func normalizeWhitespace(line string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(line, " "))
}
