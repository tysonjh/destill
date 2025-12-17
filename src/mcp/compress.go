package mcp

import "destill-agent/src/patterns"

// CompressLine applies presentation-level compression to a single line.
// Preserves diagnostic details like line numbers while reducing tokens.
//
// Transforms applied:
//   - Strip leading timestamps
//   - Shorten long paths (.../filename:line)
//   - Mask long hex strings (<HASH>)
//   - Normalize whitespace
func CompressLine(line string) string {
	return patterns.Normalize(line, patterns.MaskPresentation)
}

// CompressContextLines applies compression to a slice of context lines.
// Applies per-line compression and removes common prefixes.
func CompressContextLines(lines []string) []string {
	return patterns.NormalizeLines(lines, patterns.MaskPresentation)
}
