// Package analyze provides stateless log analysis for the agentic architecture.
package analyze

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"destill-agent/src/contracts"
)

const (
	// PreContextLines is the number of lines to extract before an error (from chunk)
	PreContextLines = 15

	// PostContextLines is the number of lines to extract after an error (from chunk)
	PostContextLines = 30
)

var (
	// Severity detection patterns
	fatalPattern   = regexp.MustCompile(`(?i)\b(FATAL|PANIC|CRITICAL)\b`)
	errorPattern   = regexp.MustCompile(`(?i)\b(ERROR|ERR|EXCEPTION|FAILURE|FAILED)\b`)
	warningPattern = regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`)

	// Normalization patterns (similar to existing analyzer)
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`)
	uuidPattern      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	hexPattern       = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)
	numberPattern    = regexp.MustCompile(`\b\d+\b`)

	// High confidence indicators
	highConfidencePattern = regexp.MustCompile(`(?i)^.{0,50}\b(FATAL|ERROR|EXCEPTION|CRITICAL)\s*[\[:]`)
)

// Finding represents an error found in a log chunk.
type Finding struct {
	LineNumber      int
	RawMessage      string
	NormalizedMsg   string
	Severity        string
	ConfidenceScore float64
	PreContext      []string
	PostContext     []string
	ContextNote     string
}

// AnalyzeChunk processes a single log chunk and returns findings.
// This is stateless - it only looks within the provided chunk.
func AnalyzeChunk(chunk contracts.LogChunk) []Finding {
	// Split content into lines
	lines := strings.Split(chunk.Content, "\n")
	if len(lines) == 0 {
		return nil
	}

	var findings []Finding

	// Process each line
	for i, line := range lines {
		// Skip empty or very short lines
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 10 {
			continue
		}

		// Detect severity
		severity := detectSeverity(trimmed)

		// Only process ERROR and FATAL
		if severity != "ERROR" && severity != "FATAL" {
			continue
		}

		// Calculate confidence
		confidence := calculateConfidence(trimmed, severity)

		// Skip low confidence findings
		if confidence < 0.5 {
			continue
		}

		// Normalize message
		normalized := normalizeMessage(trimmed)

		// Extract context from within this chunk only
		preContext, postContext, contextNote := extractContext(lines, i)

		finding := Finding{
			LineNumber:      chunk.LineStart + i,
			RawMessage:      line,
			NormalizedMsg:   normalized,
			Severity:        severity,
			ConfidenceScore: confidence,
			PreContext:      preContext,
			PostContext:     postContext,
			ContextNote:     contextNote,
		}

		findings = append(findings, finding)
	}

	return findings
}

// detectSeverity determines the severity level of a log line.
func detectSeverity(line string) string {
	if fatalPattern.MatchString(line) {
		return "FATAL"
	}
	if errorPattern.MatchString(line) {
		return "ERROR"
	}
	if warningPattern.MatchString(line) {
		return "WARN"
	}
	return "INFO"
}

// calculateConfidence calculates a confidence score for a finding.
func calculateConfidence(line string, severity string) float64 {
	score := 0.5 // Base score

	// High confidence indicators
	if highConfidencePattern.MatchString(line) {
		score += 0.3
	}

	// Severity boost
	if severity == "FATAL" {
		score += 0.2
	} else if severity == "ERROR" {
		score += 0.1
	}

	// Penalty for common false positives
	lower := strings.ToLower(line)
	if strings.Contains(lower, "test") && strings.Contains(lower, "passed") {
		score -= 0.3
	}
	if strings.Contains(lower, "deprecated") || strings.Contains(lower, "deprecation") {
		score -= 0.2
	}
	if strings.Contains(lower, "retry") {
		score -= 0.1
	}

	// Cap between 0 and 1
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}

// normalizeMessage normalizes a log message for deduplication.
func normalizeMessage(msg string) string {
	normalized := msg

	// Replace timestamps
	normalized = timestampPattern.ReplaceAllString(normalized, "[TIMESTAMP]")

	// Replace UUIDs
	normalized = uuidPattern.ReplaceAllString(normalized, "[UUID]")

	// Replace hex addresses
	normalized = hexPattern.ReplaceAllString(normalized, "[HEX]")

	// Replace numbers (but keep error codes intact)
	normalized = numberPattern.ReplaceAllString(normalized, "[NUM]")

	return strings.TrimSpace(normalized)
}

// extractContext extracts surrounding lines from within the chunk.
// Returns pre-context, post-context, and a note about truncation.
func extractContext(lines []string, lineIndex int) ([]string, []string, string) {
	var preContext []string
	var postContext []string
	var note string

	// Extract pre-context
	preStart := lineIndex - PreContextLines
	if preStart < 0 {
		note = "truncated at chunk start"
		preStart = 0
	}

	for i := preStart; i < lineIndex; i++ {
		if i >= 0 && i < len(lines) {
			preContext = append(preContext, lines[i])
		}
	}

	// Extract post-context
	postEnd := lineIndex + PostContextLines + 1
	if postEnd > len(lines) {
		if note != "" {
			note = "truncated at chunk boundaries"
		} else {
			note = "truncated at chunk end"
		}
		postEnd = len(lines)
	}

	for i := lineIndex + 1; i < postEnd; i++ {
		if i >= 0 && i < len(lines) {
			postContext = append(postContext, lines[i])
		}
	}

	return preContext, postContext, note
}

// CalculateMessageHash creates a hash of the normalized message for deduplication.
func CalculateMessageHash(normalized string) string {
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}

// ConvertToTriageCard converts a Finding to a TriageCard.
func ConvertToTriageCard(finding Finding, chunk contracts.LogChunk, requestID string) contracts.TriageCard {
	messageHash := CalculateMessageHash(finding.NormalizedMsg)

	card := contracts.TriageCard{
		ID:              fmt.Sprintf("%s-%s-%d", chunk.JobID, messageHash[:8], finding.LineNumber),
		RequestID:       requestID,
		MessageHash:     messageHash,
		Source:          "buildkite",
		JobName:         chunk.JobName,
		BuildURL:        chunk.Metadata["build_url"],
		Severity:        finding.Severity,
		RawMessage:      finding.RawMessage,
		NormalizedMsg:   finding.NormalizedMsg,
		ConfidenceScore: finding.ConfidenceScore,
		PreContext:      finding.PreContext,
		PostContext:     finding.PostContext,
		ContextNote:     finding.ContextNote,
		ChunkIndex:      chunk.ChunkIndex,
		LineInChunk:     finding.LineNumber - chunk.LineStart,
		Metadata:        copyMetadata(chunk.Metadata),
		Timestamp:       fmt.Sprintf("%d", 0), // Will be set by agent
	}

	return card
}

// copyMetadata creates a copy of metadata map.
func copyMetadata(original map[string]string) map[string]string {
	if original == nil {
		return make(map[string]string)
	}
	copy := make(map[string]string, len(original))
	for k, v := range original {
		copy[k] = v
	}
	return copy
}
