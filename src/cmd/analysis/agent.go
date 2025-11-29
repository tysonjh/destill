// Package analysis provides the Analysis Agent for the Destill log triage tool.
// This agent is the intelligence engine that processes raw logs and produces TriageCards.
package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

// DefaultConfidenceScore is the default confidence score for placeholder analysis.
const DefaultConfidenceScore = 0.75

var (
	// Regex patterns for normalization
	timestampPatterns = []*regexp.Regexp{
		// ISO 8601 formats: 2025-11-28T10:30:45Z, 2025-11-28 10:30:45
		regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`),
		// Common log formats: [10:30:45], (10:30:45.123)
		regexp.MustCompile(`[\[\(]\d{2}:\d{2}:\d{2}(\.\d+)?[\]\)]`),
		// Unix timestamps: 1732800645
		regexp.MustCompile(`\b\d{10,13}\b`),
		// Date formats: 11/28/2025, 28-Nov-2025
		regexp.MustCompile(`\d{1,2}[-/]\d{1,2}[-/]\d{2,4}`),
		regexp.MustCompile(`\d{1,2}-[A-Za-z]{3}-\d{2,4}`),
	}

	// UUID pattern: 550e8400-e29b-41d4-a716-446655440000
	uuidPattern = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)

	// Process/Thread ID patterns: pid=1234, PID: 5678, thread-9876
	pidPattern = regexp.MustCompile(`(?i)\b(pid|tid|thread)[\s=:]*\d+\b`)

	// Memory address patterns: 0x7fff5fbff710, @0x1a2b3c4d
	memoryAddressPattern = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b|@0x[0-9a-fA-F]+`)

	// IP addresses (optional - can help with recurrence but may remove useful info)
	ipAddressPattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

	// Port numbers in URLs or connection strings
	portPattern = regexp.MustCompile(`:(\d{4,5})\b`)

	// Sequence numbers and incremental IDs: seq=123, id=456
	sequencePattern = regexp.MustCompile(`(?i)\b(seq|sequence|id|index|count)[\s=:]*\d+\b`)

	// Log line numbers (not source code line numbers)
	// Matches patterns like: ,335]: or ,784 - where the number is between timestamp and message
	// Does NOT match lineno:123 or line 456 which are useful for debugging
	logLineNumberPattern = regexp.MustCompile(`,\d+[]:-]`)

	// High-signal anchor pattern: severity keywords appearing near the start of the line
	// with a separator (e.g., "ERROR:", "FATAL |", "ERROR]")
	// Character class: ] at start, - at end to avoid escaping issues
	highSignalPattern = regexp.MustCompile(`(?i)^.{0,50}\b(?:FATAL|ERROR|PANIC|EXCEPTION|CRITICAL)\s*[]:|[-]`)

	// Penalty patterns: high-confidence false positive indicators
	// These reduce confidence scores to minimize false positive triage burden

	// Transient network failures that often resolve on retry
	connectionResetRetryPattern = regexp.MustCompile(`(?i)(conn(ection)?\s+(reset|refused|timeout).*retry|retry.*conn(ection)?\s+(reset|refused|timeout))`)
	addressInUsePattern         = regexp.MustCompile(`(?i)address\s+already\s+in\s+use`)

	// Success/informational messages that shouldn't be flagged as errors
	testPassedPattern         = regexp.MustCompile(`(?i)(tests?\s+passed|all\s+tests?\s+(passed|succeeded)|build\s+succeeded|successfully\s+completed)`)
	deprecationWarningPattern = regexp.MustCompile(`(?i)(deprecated|deprecation\s+warning)`)
)

// Agent subscribes to raw logs and performs analysis.
type Agent struct {
	msgBroker contracts.MessageBroker
	logger    logger.Logger
}

// NewAgent creates a new AnalysisAgent with the given broker and logger.
func NewAgent(msgBroker contracts.MessageBroker, log logger.Logger) *Agent {
	return &Agent{
		msgBroker: msgBroker,
		logger:    log,
	}
}

// Run starts the analysis agent's main loop.
// It subscribes to the ci_logs_raw topic and processes incoming log chunks.
func (a *Agent) Run() error {
	logChannel, err := a.msgBroker.Subscribe("ci_logs_raw")
	if err != nil {
		return fmt.Errorf("failed to subscribe to ci_logs_raw: %w", err)
	}

	a.logger.Info("[AnalysisAgent] Listening for logs on 'ci_logs_raw' topic...")

	for message := range logChannel {
		// Launch processing in a Go routine for concurrency
		go a.processLogChunk(message)
	}

	return nil
}

// processLogChunk handles an incoming raw log chunk.
// Each LogChunk contains the full log output from one CI job.
// We split it into individual lines and analyze each line for failures.
func (a *Agent) processLogChunk(message []byte) {
	// Deserialize the raw message into a LogChunk
	var logChunk contracts.LogChunk
	if err := json.Unmarshal(message, &logChunk); err != nil {
		a.logger.Error("[AnalysisAgent] Error unmarshaling log chunk: %v", err)
		return
	}

	a.logger.Info("[AnalysisAgent] Processing log chunk %s for job %s", logChunk.ID, logChunk.JobName)

	// Split the log content into individual lines
	lines := strings.Split(logChunk.Content, "\n")

	a.logger.Debug("[AnalysisAgent] Analyzing %d log lines from job %s", len(lines), logChunk.JobName)

	// Process each line independently
	for lineNum, line := range lines {
		// Skip empty or trivially short lines
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) < 10 {
			continue
		}

		// Determine severity from log content FIRST
		severity := a.detectSeverity(trimmedLine)

		// FILTER: Only process ERROR and FATAL severity logs
		// Skip INFO, WARN, and DEBUG messages
		if severity != "ERROR" && severity != "FATAL" {
			continue
		}

		// Normalize the log line
		normalizedMessage := a.normalizeLog(trimmedLine)

		// Skip hook execution messages and other build system noise
		if strings.Contains(normalizedMessage, "running agent environment hook") ||
			strings.Contains(normalizedMessage, "running global environment hook") ||
			strings.Contains(normalizedMessage, "running pre-command hook") ||
			strings.Contains(normalizedMessage, "running post-command hook") {
			continue
		}

		// Calculate message hash for recurrence tracking
		messageHash := a.calculateMessageHash(normalizedMessage)

		// Calculate confidence score
		confidenceScore := a.calculateConfidenceScore(trimmedLine)

		// FILTER: Only create triage cards for high-confidence failures
		// Minimum threshold of 0.80 to reduce noise significantly
		if confidenceScore < 0.80 {
			continue
		}

		// Create the TriageCard
		// Metadata will be populated with analysis-specific information
		metadata := make(map[string]string)
		// Copy any relevant metadata from the source LogChunk
		for k, v := range logChunk.Metadata {
			metadata[k] = v
		}
		// Add line-specific metadata
		metadata["line_number"] = fmt.Sprintf("%d", lineNum+1)

		triageCard := contracts.TriageCard{
			ID:              fmt.Sprintf("%s-line-%d", logChunk.ID, lineNum+1),
			Source:          "buildkite",
			Timestamp:       time.Now().Format(time.RFC3339),
			Severity:        severity,
			Message:         normalizedMessage,
			Metadata:        metadata,
			RequestID:       logChunk.RequestID,
			MessageHash:     messageHash,
			JobName:         logChunk.JobName,
			ConfidenceScore: confidenceScore,
		}

		// Marshal and publish to ci_failures_ranked topic
		data, err := json.Marshal(triageCard)
		if err != nil {
			a.logger.Error("[AnalysisAgent] Error marshaling triage card: %v", err)
			continue
		}

		if err := a.msgBroker.Publish("ci_failures_ranked", data); err != nil {
			a.logger.Error("[AnalysisAgent] Error publishing triage card: %v", err)
			continue
		}

		a.logger.Debug("[AnalysisAgent] Published triage card for line %d (severity: %s, confidence: %.2f)",
			lineNum+1, severity, confidenceScore)
	}
}

// normalizeLog performs comprehensive log normalization for recurrence tracking.
// This is the core intelligence that allows us to identify recurring failures.
func (a *Agent) normalizeLog(content string) string {
	normalized := content

	// Step 1: Remove UUIDs FIRST (before timestamps, as UUIDs contain dashes that look like dates)
	normalized = uuidPattern.ReplaceAllString(normalized, "[UUID]")

	// Step 2: Remove all timestamp formats
	for _, pattern := range timestampPatterns {
		normalized = pattern.ReplaceAllString(normalized, "[TIMESTAMP]")
	}

	// Step 3: Remove process/thread IDs
	normalized = pidPattern.ReplaceAllString(normalized, "[PID]")

	// Step 4: Remove memory addresses
	normalized = memoryAddressPattern.ReplaceAllString(normalized, "[ADDR]")

	// Step 5: Remove IP addresses (optional - may want to keep for network errors)
	normalized = ipAddressPattern.ReplaceAllString(normalized, "[IP]")

	// Step 6: Remove port numbers (often dynamic)
	normalized = portPattern.ReplaceAllString(normalized, ":[PORT]")

	// Step 7: Remove log file line numbers (e.g., ,335]: or ,784 -)
	// These are just positions in the log file, not useful for grouping
	// Preserves source code line numbers like lineno:123 which are valuable for debugging
	normalized = logLineNumberPattern.ReplaceAllString(normalized, "[LINE]")

	// Step 8: Remove sequence numbers and incremental IDs
	normalized = sequencePattern.ReplaceAllString(normalized, "[SEQ]")

	// Step 9: Normalize whitespace - replace multiple spaces/tabs/newlines with single space
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")

	// Step 10: Convert to lowercase for case-insensitive matching
	normalized = strings.ToLower(normalized)

	// Step 11: Trim leading/trailing whitespace
	normalized = strings.TrimSpace(normalized)

	return normalized
}

// calculateMessageHash generates a SHA256 hash of the normalized failure message.
// This hash is the key to recurrence tracking - identical normalized messages
// will always produce the same hash, allowing us to track how often a failure occurs.
func (a *Agent) calculateMessageHash(normalizedMessage string) string {
	hash := sha256.Sum256([]byte(normalizedMessage))
	return hex.EncodeToString(hash[:])
}

// detectSeverity analyzes log content to determine severity level.
// Uses a simplified approach: Permissive Detection (High Recall) + Confident Scoring (High Precision).
// If any error-like keyword is present, tag as ERROR. The confidence score handles quality differentiation.
func (a *Agent) detectSeverity(content string) string {
	lowerContent := strings.ToLower(content)

	// Combined error keywords - merge FATAL and ERROR for high recall
	// Quality differentiation is handled by confidence scoring
	errorKeywords := []string{
		"fatal", "panic", "segmentation fault", "core dumped", "out of memory",
		"error", "exception", "failed", "failure", "cannot", "unable to", "denied",
	}
	for _, keyword := range errorKeywords {
		if strings.Contains(lowerContent, keyword) {
			return "ERROR"
		}
	}

	// Warnings - potential issues
	warnKeywords := []string{"warn", "warning", "deprecated", "obsolete"}
	for _, keyword := range warnKeywords {
		if strings.Contains(lowerContent, keyword) {
			return "WARN"
		}
	}

	// Default to INFO if no severity indicators found
	return "INFO"
}

// calculateConfidenceScore determines the confidence of the analysis based on
// the presence of strong failure indicators and log structure.
func (a *Agent) calculateConfidenceScore(content string) float64 {
	score := 0.5 // Base score
	lowerContent := strings.ToLower(content)

	// High-signal anchor bonus: severity keywords near start of line with separator
	if highSignalPattern.MatchString(content) {
		score += 0.25
	}

	// Increase confidence for strong error indicators
	if strings.Contains(lowerContent, "exception") {
		score += 0.15
	}
	if strings.Contains(lowerContent, "stack trace") || strings.Contains(lowerContent, "traceback") {
		score += 0.15
	}
	if strings.Contains(lowerContent, "fatal") || strings.Contains(lowerContent, "panic") {
		score += 0.20
	}

	// Increase confidence for structured error information
	if strings.Contains(lowerContent, "line") && regexp.MustCompile(`\bline\s+\d+\b`).MatchString(lowerContent) {
		score += 0.15
	}
	if strings.Contains(lowerContent, "file") || strings.Contains(lowerContent, ".go") ||
		strings.Contains(lowerContent, ".py") || strings.Contains(lowerContent, ".js") {
		score += 0.10
	}

	// Decrease confidence for vague messages
	if len(content) < 20 {
		score -= 0.15
	}

	// PENALTIES: High-confidence false positive indicators
	// These patterns indicate likely non-actionable errors

	// Transient network failures that typically resolve on retry
	if connectionResetRetryPattern.MatchString(content) {
		score -= 0.30
	}
	if addressInUsePattern.MatchString(content) {
		score -= 0.25
	}

	// Success/informational messages mistakenly containing error keywords
	if testPassedPattern.MatchString(content) {
		score -= 0.35
	}
	if deprecationWarningPattern.MatchString(content) {
		score -= 0.20
	}

	// Cap the score between 0.0 and 1.0
	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}

	return score
}
