// Package patterns provides unified log pattern normalization for both
// recurrence detection (grouping similar errors) and presentation (token reduction).
//
// The same underlying patterns are used with different masking levels:
//   - MaskRecurrence: Aggressive normalization for grouping (masks line numbers)
//   - MaskPresentation: Conservative normalization for display (preserves line numbers)
package patterns

import (
	"regexp"
	"strings"
)

// MaskingLevel controls how aggressively log lines are normalized.
type MaskingLevel int

const (
	// MaskPresentation preserves diagnostic details like line numbers.
	// Use for: MCP responses, UI display, debugging output.
	// Example: /var/lib/.../file.go:42 → .../file.go:42
	MaskPresentation MaskingLevel = iota

	// MaskRecurrence aggressively normalizes for grouping identical errors.
	// Use for: Deduplication, recurrence counting, pattern matching.
	// Example: Error on line 42 → Error on line [NUM]
	MaskRecurrence
)

// Shared regex patterns - compiled once at package init.
var (
	// timestampPattern matches ISO8601 and common log timestamps.
	// Matches: 2024-05-21T10:00:05.123Z, 2024-05-21 10:00:05,123, etc.
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}([.,]\d+)?(Z|[+-]\d{2}:?\d{2})?`)

	// uuidPattern matches standard UUIDs.
	// Matches: 550e8400-e29b-41d4-a716-446655440000
	uuidPattern = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)

	// longHashPattern matches long hex strings (container IDs, git SHAs, etc.)
	// Matches: abc123def456789 (12+ hex chars, not prefixed with 0x)
	longHashPattern = regexp.MustCompile(`\b[a-f0-9]{12,}\b`)

	// hexAddressPattern matches hex addresses (0x prefixed).
	// Matches: 0x7fff5fbff8c0
	hexAddressPattern = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)

	// numberPattern matches standalone numbers.
	// Matches: 42, 1234, etc. (word boundary prevents matching within hashes)
	numberPattern = regexp.MustCompile(`\b\d+\b`)

	// longPathPattern matches absolute paths with 3+ directories.
	// Captures filename and optional line number for preservation.
	longPathPattern = regexp.MustCompile(`/(?:[^/\s]+/){3,}([^/\s:]+(?::\d+)?)`)

	// whitespacePattern matches multiple consecutive whitespace.
	whitespacePattern = regexp.MustCompile(`\s+`)
)

// minPrefixLength is the minimum common prefix length worth removing.
const minPrefixLength = 20

// Normalize applies pattern normalization to a single line.
// The masking level determines how aggressively patterns are replaced.
func Normalize(line string, level MaskingLevel) string {
	// Core transforms - always apply
	line = stripTimestamps(line, level)
	line = maskUUIDs(line, level)
	line = maskHexAddresses(line, level)

	// Level-specific transforms
	switch level {
	case MaskPresentation:
		line = compressPath(line)
		line = maskLongHashes(line)
	case MaskRecurrence:
		line = maskAllPaths(line)
		line = maskLongHashes(line)
		line = maskNumbers(line)
	}

	// Final cleanup
	line = normalizeWhitespace(line)

	return line
}

// NormalizeLines applies normalization to multiple lines and optionally
// removes common prefixes (presentation mode only).
func NormalizeLines(lines []string, level MaskingLevel) []string {
	if len(lines) == 0 {
		return lines
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = Normalize(line, level)
	}

	// Only remove common prefixes in presentation mode
	if level == MaskPresentation {
		result = removeCommonPrefix(result)
	}

	return result
}

// --- Core transforms (always applied) ---

// stripTimestamps removes or replaces timestamps based on level.
func stripTimestamps(line string, level MaskingLevel) string {
	switch level {
	case MaskPresentation:
		// Strip leading timestamps entirely for cleaner display
		if loc := timestampPattern.FindStringIndex(line); loc != nil && loc[0] < 5 {
			line = strings.TrimSpace(line[loc[1]:])
		}
		return line
	case MaskRecurrence:
		// Replace all timestamps with placeholder for grouping
		return timestampPattern.ReplaceAllString(line, "[TIMESTAMP]")
	}
	return line
}

// maskUUIDs replaces UUIDs based on level.
func maskUUIDs(line string, level MaskingLevel) string {
	switch level {
	case MaskPresentation:
		return uuidPattern.ReplaceAllString(line, "<UUID>")
	case MaskRecurrence:
		return uuidPattern.ReplaceAllString(line, "[UUID]")
	}
	return line
}

// maskHexAddresses replaces 0x-prefixed hex addresses.
func maskHexAddresses(line string, level MaskingLevel) string {
	switch level {
	case MaskPresentation:
		return hexAddressPattern.ReplaceAllString(line, "<HEX>")
	case MaskRecurrence:
		return hexAddressPattern.ReplaceAllString(line, "[HEX]")
	}
	return line
}

// --- Presentation-only transforms ---

// compressPath shortens long paths while preserving filename and line number.
// /var/lib/jenkins/workspace/src/main.go:42 → .../main.go:42
func compressPath(line string) string {
	return longPathPattern.ReplaceAllString(line, ".../$1")
}

// maskLongHashes replaces long hex strings (container IDs, git SHAs).
func maskLongHashes(line string) string {
	return longHashPattern.ReplaceAllString(line, "<HASH>")
}

// --- Recurrence-only transforms ---

// maskAllPaths replaces entire paths with placeholder.
func maskAllPaths(line string) string {
	return longPathPattern.ReplaceAllString(line, "[PATH]")
}

// maskNumbers replaces all standalone numbers with placeholder.
// This masks line numbers for grouping identical error patterns.
func maskNumbers(line string) string {
	return numberPattern.ReplaceAllString(line, "[NUM]")
}

// --- Shared cleanup ---

// normalizeWhitespace collapses multiple spaces and trims.
func normalizeWhitespace(line string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(line, " "))
}

// removeCommonPrefix finds and removes common prefix across lines.
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

// findCommonPrefix finds the longest common prefix across lines.
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
