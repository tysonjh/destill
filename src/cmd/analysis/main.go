// Package main provides the Analysis Agent for the Destill log triage tool.
// This agent is the intelligence engine that processes raw logs and produces TriageCards.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"destill-agent/src/contracts"
)

// DefaultConfidenceScore is the default confidence score for placeholder analysis.
const DefaultConfidenceScore = 0.75

// AnalysisAgent subscribes to raw logs and performs analysis.
type AnalysisAgent struct {
	msgBroker contracts.MessageBroker
}

// NewAnalysisAgent creates a new AnalysisAgent with the given broker.
func NewAnalysisAgent(msgBroker contracts.MessageBroker) *AnalysisAgent {
	return &AnalysisAgent{msgBroker: msgBroker}
}

// Run starts the analysis agent's main loop.
// It subscribes to the ci_logs_raw topic and processes incoming log chunks.
func (a *AnalysisAgent) Run() error {
	logChannel, err := a.msgBroker.Subscribe("ci_logs_raw")
	if err != nil {
		return fmt.Errorf("failed to subscribe to ci_logs_raw: %w", err)
	}

	fmt.Println("[AnalysisAgent] Listening for logs on 'ci_logs_raw' topic...")

	for message := range logChannel {
		// Launch processing in a Go routine for concurrency
		go a.processLogChunk(message)
	}

	return nil
}

// processLogChunk handles an incoming raw log chunk.
// This is where the full analysis pipeline will be implemented.
func (a *AnalysisAgent) processLogChunk(message []byte) {
	// Deserialize the raw message into a LogChunk
	var logChunk contracts.LogChunk
	if err := json.Unmarshal(message, &logChunk); err != nil {
		fmt.Printf("[AnalysisAgent] Error unmarshaling log chunk: %v\n", err)
		return
	}

	fmt.Printf("[AnalysisAgent] Processing log chunk %s for job %s\n", logChunk.ID, logChunk.JobName)

	// Placeholder: Log normalization
	normalizedMessage := a.normalizeLog(logChunk.Content)

	// Placeholder: Calculate message hash for recurrence tracking
	messageHash := a.calculateMessageHash(normalizedMessage)

	// Placeholder: Determine severity from log content
	severity := a.detectSeverity(logChunk.Content)

	// Placeholder: Calculate confidence score
	confidenceScore := a.calculateConfidenceScore(logChunk.Content)

	// Create the TriageCard
	triageCard := contracts.TriageCard{
		ID:              fmt.Sprintf("triage-%d", time.Now().UnixNano()),
		Source:          "buildkite",
		Timestamp:       time.Now().Format(time.RFC3339),
		Severity:        severity,
		Message:         normalizedMessage,
		Metadata:        make(map[string]string),
		RequestID:       logChunk.RequestID,
		MessageHash:     messageHash,
		JobName:         logChunk.JobName,
		ConfidenceScore: confidenceScore,
	}

	// Marshal and publish to ci_failures_ranked topic
	data, err := json.Marshal(triageCard)
	if err != nil {
		fmt.Printf("[AnalysisAgent] Error marshaling triage card: %v\n", err)
		return
	}

	if err := a.msgBroker.Publish("ci_failures_ranked", data); err != nil {
		fmt.Printf("[AnalysisAgent] Error publishing triage card: %v\n", err)
		return
	}

	fmt.Printf("[AnalysisAgent] Published triage card to 'ci_failures_ranked' (hash: %s)\n", messageHash[:8])
}

// normalizeLog performs log normalization.
// Placeholder implementation - will be enhanced with actual normalization logic.
func (a *AnalysisAgent) normalizeLog(content string) string {
	// Basic normalization: trim whitespace and convert to lowercase for comparison
	normalized := strings.TrimSpace(content)
	return normalized
}

// calculateMessageHash generates a unique hash of the normalized failure message.
// Used for recurrence tracking across builds.
func (a *AnalysisAgent) calculateMessageHash(normalizedMessage string) string {
	hash := sha256.Sum256([]byte(normalizedMessage))
	return hex.EncodeToString(hash[:])
}

// detectSeverity analyzes log content to determine severity level.
// Placeholder implementation - will be enhanced with ML/pattern matching.
func (a *AnalysisAgent) detectSeverity(content string) string {
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "error") || strings.Contains(lowerContent, "fatal") {
		return "ERROR"
	}
	if strings.Contains(lowerContent, "warn") {
		return "WARN"
	}
	return "INFO"
}

// calculateConfidenceScore determines the confidence of the analysis.
// Placeholder implementation - returns a fixed score for now.
func (a *AnalysisAgent) calculateConfidenceScore(content string) float64 {
	// Placeholder: return default confidence score
	return DefaultConfidenceScore
}

func main() {
	fmt.Println("Destill Analysis Agent")
	fmt.Println("This agent should be started by the CLI orchestrator.")
	fmt.Println("Run 'destill analyze' to start the full pipeline.")
}
