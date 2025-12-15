// Package analyze provides the Analysis Agent for the distributed architecture.
// This agent consumes log chunks from Redpanda and publishes findings.
package analyze

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

// Agent consumes log chunks and publishes analysis findings.
type Agent struct {
	broker broker.Broker
	logger logger.Logger
}

// NewAgent creates a new analyze agent.
func NewAgent(brk broker.Broker, log logger.Logger) *Agent {
	return &Agent{
		broker: brk,
		logger: log,
	}
}

// Run starts the agent's main loop.
// It subscribes to destill.logs.raw and processes incoming chunks.
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("[AnalyzeAgent] Starting...")

	// Subscribe to log chunks topic
	msgChan, err := a.broker.Subscribe(ctx, contracts.TopicLogsRaw, "destill-analyze")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", contracts.TopicLogsRaw, err)
	}

	a.logger.Info("[AnalyzeAgent] Listening for log chunks on '%s' topic...", contracts.TopicLogsRaw)

	// Process messages
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				a.logger.Info("[AnalyzeAgent] Message channel closed, shutting down")
				return nil
			}

			if err := a.processChunk(ctx, msg); err != nil {
				a.logger.Error("[AnalyzeAgent] Error processing chunk: %v", err)
			}

		case <-ctx.Done():
			a.logger.Info("[AnalyzeAgent] Context cancelled, shutting down")
			return ctx.Err()
		}
	}
}

// processChunk analyzes a single log chunk and publishes findings.
func (a *Agent) processChunk(ctx context.Context, msg broker.Message) error {
	// Parse log chunk
	var chunk contracts.LogChunk
	if err := json.Unmarshal(msg.Value, &chunk); err != nil {
		return fmt.Errorf("failed to unmarshal chunk: %w", err)
	}

	a.logger.Debug("[AnalyzeAgent] Processing chunk %d/%d for job '%s'",
		chunk.ChunkIndex+1, chunk.TotalChunks, chunk.JobName)

	// Analyze chunk (stateless)
	findings := AnalyzeChunk(chunk)

	if len(findings) == 0 {
		a.logger.Debug("[AnalyzeAgent] No findings in chunk %d/%d",
			chunk.ChunkIndex+1, chunk.TotalChunks)
		return nil
	}

	a.logger.Info("[AnalyzeAgent] Found %d issues in chunk %d/%d of job '%s'",
		len(findings), chunk.ChunkIndex+1, chunk.TotalChunks, chunk.JobName)

	// Convert and publish each finding
	for _, finding := range findings {
		card := ConvertToTriageCard(finding, chunk, chunk.RequestID)
		card.Timestamp = time.Now().Format(time.RFC3339)

		data, err := json.Marshal(card)
		if err != nil {
			a.logger.Error("[AnalyzeAgent] Failed to marshal finding: %v", err)
			continue
		}

		// Publish to destill.analysis.findings with requestID as key for grouping
		if err := a.broker.Publish(ctx, contracts.TopicAnalysisFindings, chunk.RequestID, data); err != nil {
			a.logger.Error("[AnalyzeAgent] Failed to publish finding: %v", err)
			continue
		}

		a.logger.Debug("[AnalyzeAgent] Published finding: %s (confidence: %.2f)",
			finding.Severity, finding.ConfidenceScore)
	}

	return nil
}
