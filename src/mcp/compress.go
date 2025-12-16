package mcp

import "regexp"

// timestampPattern matches leading timestamps in various formats:
// - 2024-05-21T10:00:05.123Z
// - 2024-05-21 10:00:05,123
// - 2024-05-21T10:00:05+00:00
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[.,]?\d*[Z]?([+-]\d{2}:?\d{2})?\s*`)

// stripTimestamps removes leading timestamps from a line.
func stripTimestamps(line string) string {
	return timestampPattern.ReplaceAllString(line, "")
}
