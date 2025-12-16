// Package sanitize provides utilities for cleaning log output for LLM consumption.
// It removes ANSI escape codes and CI-specific markers (like Buildkite timestamps)
// to produce clean, readable text suitable for MCP tool responses.
//
// This package is specifically for MCP output sanitization. For TUI rendering,
// use the tui package which has its own ANSI handling via charmbracelet/x/ansi.
package sanitize

import "regexp"

var (
	// ANSI escape codes: \x1b[...m (SGR sequences)
	ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

	// Buildkite timestamp markers: \x1b_bk;t=...\x07
	buildkiteTimestamp = regexp.MustCompile(`\x1b_bk;t=[0-9]+\x07`)
)

// StripANSI removes ANSI escape codes and Buildkite timestamp markers.
func StripANSI(s string) string {
	s = buildkiteTimestamp.ReplaceAllString(s, "")
	s = ansiPattern.ReplaceAllString(s, "")
	return s
}
