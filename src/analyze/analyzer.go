// Package analyze provides stateless log analysis for the distributed architecture.
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

	// Normalization patterns
	timestampPattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:?\d{2})?`)
	uuidPattern      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	hexPattern       = regexp.MustCompile(`\b0x[0-9a-fA-F]+\b`)
	numberPattern    = regexp.MustCompile(`\b\d+\b`)

	// High confidence indicators
	highConfidencePattern = regexp.MustCompile(`(?i)^.{0,50}\b(FATAL|ERROR|EXCEPTION|CRITICAL)\s*[\[:]`)

	// === BOOST PATTERNS (high signal) ===

	// Stack traces
	stackTraceJava   = regexp.MustCompile(`^\s+at\s+[\w.$]+\(`)                            // Java/Kotlin/Scala
	stackTracePython = regexp.MustCompile(`(?i)^Traceback \(most recent call`)            // Python
	pythonFileLine   = regexp.MustCompile(`^\s*File ".*", line \d+`)                       // Python file:line
	panicGo          = regexp.MustCompile(`^panic:`)                                       // Go panic
	stackTraceCpp    = regexp.MustCompile(`(?i)^(Backtrace:|Stack trace:|#\d+\s+0x[0-9a-f]+)`) // C/C++
	terminateCpp     = regexp.MustCompile(`(?i)^terminate called`)                         // C++ terminate

	// Exit/return codes
	exitCodePattern = regexp.MustCompile(`(?i)(exit(ed)?|return(ed)?|status).{0,20}(code|status)?\s*[:\s]+[1-9]\d*`)
	nonZeroExit     = regexp.MustCompile(`(?i)(non-?zero|failed|failure).{0,15}(exit|return|code)`)

	// Build tool specific - npm
	npmError = regexp.MustCompile(`^npm ERR!`)
	npmCodes = regexp.MustCompile(`\b(ENOENT|EACCES|ELIFECYCLE|ECONNREFUSED|ECONNRESET|E404|ERESOLVE)\b`)

	// Build tool specific - Maven/Gradle
	mavenFailure  = regexp.MustCompile(`^\[ERROR\]|BUILD FAILURE`)
	gradleFailure = regexp.MustCompile(`^FAILURE:|BUILD FAILED`)

	// Build tool specific - Docker
	dockerError = regexp.MustCompile(`(?i)(Error response from daemon|error during connect|Cannot connect to the Docker)`)

	// Kubernetes errors
	k8sErrors = regexp.MustCompile(`\b(ErrImagePull|ImagePullBackOff|CrashLoopBackOff|OOMKilled|NodeNotReady|RunContainerError)\b`)

	// Crashes & resource issues
	oomPattern     = regexp.MustCompile(`(?i)(OutOfMemory|out of memory|OOM|Cannot allocate memory|heap space|memory exhausted)`)
	segfaultPattern = regexp.MustCompile(`(?i)(Segmentation fault|SIGSEGV|SIGKILL|SIGABRT|core dumped|Aborted)`)
	timeoutPattern = regexp.MustCompile(`(?i)(timed?\s*out|deadline exceeded|context canceled|context deadline|ETIMEDOUT)`)

	// Compilation/import errors
	compileError = regexp.MustCompile(`(?i)(cannot find symbol|undefined reference|does not exist|not found|unresolved|linker error)`)
	importError  = regexp.MustCompile(`(?i)(ModuleNotFoundError|cannot find module|No module named|import.*failed|could not resolve)`)
	syntaxError  = regexp.MustCompile(`(?i)(SyntaxError|unexpected token|parse error|invalid syntax|unexpected end)`)

	// Permission/auth errors
	permissionError = regexp.MustCompile(`(?i)(Permission denied|Access denied|Unauthorized|403 Forbidden|401 Unauthorized|EACCES)`)

	// Connection failures
	connectionError = regexp.MustCompile(`(?i)(Connection refused|Connection reset|ECONNREFUSED|ECONNRESET|network unreachable|host unreachable)`)

	// Assertion failures
	assertionError = regexp.MustCompile(`(?i)(assertion failed|AssertionError|assert.*failed|ASSERT)`)

	// === PENALTY PATTERNS (false positives) ===

	// Success messages containing "error" word
	zeroErrorsPattern = regexp.MustCompile(`(?i)(^|[^\d])0 errors?\b|no errors?\b|errors?:\s*0\b`)

	// Test expectations (testing for errors, not actual errors)
	// Be careful not to match "expected X but got Y" which is an actual assertion failure message
	testExpectPattern = regexp.MustCompile(`(?i)(expect|should|assert|must)\s*[\.(].{0,20}(error|throw|fail|reject)`)

	// Caught/handled errors
	handledErrorPattern = regexp.MustCompile(`(?i)(caught|rescued|handled|recovered|catching|recovery|graceful)`)

	// Error in variable/function names (but not actual Error types like SyntaxError, TypeError, etc.)
	// Use word boundaries to avoid matching substrings like "AssertionError" containing "onError"
	errorVarPattern = regexp.MustCompile(`(error[A-Z_]|_error_|error_|\.error\(|\bgetError\b|\bsetError\b|\bisError\b|\bhasError\b|\blastError\b|\bonError\b|\bhandleError\b)`)

	// Success after retry
	retrySuccessPattern = regexp.MustCompile(`(?i)(succeeded|passed|success|ok).{0,20}(retry|attempt|retrying)`)

	// Comments
	commentPattern = regexp.MustCompile(`^\s*(//|#\s|/\*|\*\s|<!--)`)

	// Log level in quotes (part of format string, not actual error)
	quotedLevelPattern = regexp.MustCompile(`["'](ERROR|FATAL|WARN)["']`)

	// Documentation/help text
	helpTextPattern = regexp.MustCompile(`(?i)(usage:|--help|example:|see also:|documentation)`)
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

	// Check job outcome based on exit status
	// exit_status "0" = passed, non-zero = failed
	jobFailed := false
	jobPassed := false
	if exitStatus, ok := chunk.Metadata["exit_status"]; ok {
		if exitStatus != "0" {
			jobFailed = true
		} else {
			jobPassed = true
		}
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

		// Adjust confidence based on job outcome:
		// - Boost for failed jobs (errors more likely to be root cause)
		// - Penalize for passed jobs (errors are likely noise/teardown)
		if jobFailed {
			confidence = boostConfidenceForFailedJob(confidence)
		} else if jobPassed {
			confidence = penalizeConfidenceForPassedJob(confidence)
		}

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
	lower := strings.ToLower(line)

	// === BOOSTS ===

	// High confidence indicators (structured log prefix)
	if highConfidencePattern.MatchString(line) {
		score += 0.25
	}

	// Severity boost
	if severity == "FATAL" {
		score += 0.2
	} else if severity == "ERROR" {
		score += 0.1
	}

	// Stack traces (very high signal)
	if stackTraceJava.MatchString(line) || stackTracePython.MatchString(line) ||
		pythonFileLine.MatchString(line) || panicGo.MatchString(line) ||
		stackTraceCpp.MatchString(line) || terminateCpp.MatchString(line) {
		score += 0.30
	}

	// Build tool errors (definitive)
	if npmError.MatchString(line) || npmCodes.MatchString(line) ||
		mavenFailure.MatchString(line) || gradleFailure.MatchString(line) {
		score += 0.30
	}

	// Docker/K8s errors
	if dockerError.MatchString(line) || k8sErrors.MatchString(line) {
		score += 0.30
	}

	// Crashes and resource issues (very high signal)
	if oomPattern.MatchString(line) || segfaultPattern.MatchString(line) {
		score += 0.35
	}

	// Timeout errors
	if timeoutPattern.MatchString(line) {
		score += 0.20
	}

	// Exit code failures
	if exitCodePattern.MatchString(line) || nonZeroExit.MatchString(line) {
		score += 0.25
	}

	// Compilation/syntax/import errors
	if compileError.MatchString(line) || syntaxError.MatchString(line) || importError.MatchString(line) {
		score += 0.25
	}

	// Permission/auth errors
	if permissionError.MatchString(line) {
		score += 0.20
	}

	// Connection failures
	if connectionError.MatchString(line) {
		score += 0.20
	}

	// Assertion failures
	if assertionError.MatchString(line) {
		score += 0.25
	}

	// === PENALTIES ===

	// "0 errors" or "no errors" - success message (heavy penalty)
	if zeroErrorsPattern.MatchString(line) {
		score -= 0.50
	}

	// Test expectations (testing for errors, not actual errors)
	if testExpectPattern.MatchString(line) {
		score -= 0.40
	}

	// Caught/handled errors
	if handledErrorPattern.MatchString(line) {
		score -= 0.30
	}

	// Error in variable/function names
	if errorVarPattern.MatchString(line) {
		score -= 0.25
	}

	// Success after retry
	if retrySuccessPattern.MatchString(line) {
		score -= 0.40
	}

	// Comments
	if commentPattern.MatchString(line) {
		score -= 0.30
	}

	// Quoted log levels (format strings, not actual errors)
	if quotedLevelPattern.MatchString(line) {
		score -= 0.30
	}

	// Help/documentation text
	if helpTextPattern.MatchString(line) {
		score -= 0.25
	}

	// Test passed messages
	if strings.Contains(lower, "test") && strings.Contains(lower, "passed") {
		score -= 0.30
	}

	// Deprecation warnings (usually not actionable)
	if strings.Contains(lower, "deprecated") || strings.Contains(lower, "deprecation") {
		score -= 0.20
	}

	// Retry without failure context (might be transient)
	if strings.Contains(lower, "retry") && !strings.Contains(lower, "failed") && !strings.Contains(lower, "error") {
		score -= 0.15
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

// boostConfidenceForFailedJob boosts confidence scores for findings from failed jobs.
// Errors from failed jobs are much more likely to be the actual root cause than
// errors from passing jobs (which are often test output or transient issues).
//
// Uses an asymptotic "compress toward 1.0" formula that:
// - Preserves relative ordering (0.9 stays above 0.8 after boosting)
// - Never actually reaches 1.0 (asymptotic)
// - Gives bigger boosts to lower confidence scores
//
// Formula: boosted = 1 - (1 - base) * shrinkFactor
// With shrinkFactor=0.4:
//
//	0.5 -> 0.70, 0.7 -> 0.82, 0.8 -> 0.88, 0.9 -> 0.94
func boostConfidenceForFailedJob(baseConfidence float64) float64 {
	// Shrink the gap to 1.0 by this factor
	// Lower values = more aggressive boost
	shrinkFactor := 0.4

	boosted := 1.0 - (1.0-baseConfidence)*shrinkFactor

	return boosted
}

// penalizeConfidenceForPassedJob reduces confidence scores for findings from passed jobs.
// If a job passed despite showing errors, those errors are likely:
// - Test teardown noise (404s when nodes are already stopped)
// - Expected errors in error-handling tests (401 Unauthorized in auth tests)
// - Transient issues that self-resolved
// - Log output from tests verifying error conditions
//
// Uses multiplicative penalty to maintain relative ordering within passed job findings
// while ensuring they rank below boosted findings from failed jobs.
//
// With factor=0.6:
//
//	0.9 -> 0.54, 0.8 -> 0.48, 0.7 -> 0.42
func penalizeConfidenceForPassedJob(baseConfidence float64) float64 {
	// Multiply by this factor (0.6 = 40% reduction)
	factor := 0.6

	penalized := baseConfidence * factor

	return penalized
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
