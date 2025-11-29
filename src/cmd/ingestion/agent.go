// Package ingestion provides the Ingestion Agent for the Destill log triage tool.
// This agent consumes requests from a topic and publishes raw log data.
package ingestion

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"destill-agent/src/buildkite"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

const (
	// ChunkSizeLines is the target number of lines per chunk (approx 10 MB for typical logs)
	ChunkSizeLines = 50000
	// ContextOverlapLines is how many lines to overlap between chunks to preserve context
	ContextOverlapLines = 20
)

// Agent consumes requests and publishes raw log data via a MessageBroker.
type Agent struct {
	msgBroker       contracts.MessageBroker
	buildkiteClient *buildkite.Client
	logger          logger.Logger
}

// NewAgent creates a new IngestionAgent with the given broker, Buildkite API token, and logger.
func NewAgent(msgBroker contracts.MessageBroker, buildkiteAPIToken string, log logger.Logger) *Agent {
	return &Agent{
		msgBroker:       msgBroker,
		buildkiteClient: buildkite.NewClient(buildkiteAPIToken),
		logger:          log,
	}
}

// Run starts the ingestion agent's main loop.
// It subscribes to the destill_requests topic and processes incoming requests.
func (a *Agent) Run() error {
	requestChannel, err := a.msgBroker.Subscribe("destill_requests")
	if err != nil {
		return fmt.Errorf("failed to subscribe to destill_requests: %w", err)
	}

	a.logger.Info("[IngestionAgent] Listening for requests on 'destill_requests' topic...")

	for message := range requestChannel {
		if err := a.processRequest(message); err != nil {
			a.logger.Error("[IngestionAgent] Error processing request: %v", err)
		}
	}

	return nil
}

// processRequest handles an incoming request message.
func (a *Agent) processRequest(message []byte) error {
	// Parse the incoming request
	var request struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}

	if err := json.Unmarshal(message, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	a.logger.Info("[IngestionAgent] Processing build request %s", request.RequestID)
	a.logger.Info("[IngestionAgent] Build URL: %s", request.BuildURL)

	// Extract build information from the URL
	org, pipeline, buildNumber, err := buildkite.ParseBuildURL(request.BuildURL)
	if err != nil {
		return fmt.Errorf("failed to parse build URL: %w", err)
	}

	a.logger.Info("[IngestionAgent] Fetching build metadata for %s/%s #%d", org, pipeline, buildNumber)

	// Fetch build metadata from Buildkite API
	build, err := a.buildkiteClient.GetBuild(org, pipeline, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch build metadata: %w", err)
	}

	a.logger.Info("[IngestionAgent] Found %d jobs in build #%d (state: %s)", len(build.Jobs), buildNumber, build.State)

	// Process each job in the build
	for _, job := range build.Jobs {
		// Skip non-script jobs (e.g., waiter, trigger jobs)
		if job.Type != "script" {
			a.logger.Debug("[IngestionAgent] Skipping non-script job: %s (type: %s)", job.Name, job.Type)
			continue
		}

		a.logger.Info("[IngestionAgent] Fetching logs for job: %s (id: %s, state: %s)", job.Name, job.ID, job.State)

		// Fetch the raw log content for this job
		logContent, err := a.buildkiteClient.GetJobLog(org, pipeline, buildNumber, job.ID)
		if err != nil {
			// Log the error but continue processing other jobs
			a.logger.Error("[IngestionAgent] Warning: failed to fetch log for job %s: %v", job.Name, err)
			continue
		}

		// Process log in chunks to handle large logs efficiently
		chunks := a.chunkLogContent(logContent)
		a.logger.Info("[IngestionAgent] Split log into %d chunks for job %s", len(chunks), job.Name)

		// Publish each chunk separately
		for chunkIdx, chunkContent := range chunks {
			logChunk := contracts.LogChunk{
				ID:        fmt.Sprintf("chunk-%s-%s-%d", request.RequestID, job.ID, chunkIdx),
				RequestID: request.RequestID,
				JobName:   job.Name,
				Content:   chunkContent,
				Timestamp: time.Now().Format(time.RFC3339),
				Metadata: map[string]string{
					"build_url":    request.BuildURL,
					"org":          org,
					"pipeline":     pipeline,
					"build_number": fmt.Sprintf("%d", buildNumber),
					"job_id":       job.ID,
					"job_state":    job.State,
					"job_type":     job.Type,
					"chunk_index":  fmt.Sprintf("%d", chunkIdx),
					"total_chunks": fmt.Sprintf("%d", len(chunks)),
				},
			}

			// Add exit status if available
			if job.ExitStatus != nil {
				logChunk.Metadata["exit_status"] = fmt.Sprintf("%d", *job.ExitStatus)
			}

			// Marshal and publish to ci_logs_raw topic
			data, err := json.Marshal(logChunk)
			if err != nil {
				a.logger.Error("[IngestionAgent] Error marshaling log chunk %d for job %s: %v", chunkIdx, job.Name, err)
				continue
			}

			if err := a.msgBroker.Publish("ci_logs_raw", data); err != nil {
				a.logger.Error("[IngestionAgent] Error publishing log chunk %d for job %s: %v", chunkIdx, job.Name, err)
				continue
			}

			a.logger.Debug("[IngestionAgent] Published chunk %d/%d for job '%s' (%d bytes)",
				chunkIdx+1, len(chunks), job.Name, len(chunkContent))
		}
	}

	a.logger.Info("[IngestionAgent] Completed processing build request %s", request.RequestID)
	return nil
}

// chunkLogContent splits a large log into smaller chunks with intelligent boundary detection.
// It avoids splitting in the middle of error contexts by finding safe split points.
func (a *Agent) chunkLogContent(logContent string) []string {
	scanner := bufio.NewScanner(strings.NewReader(logContent))
	var lines []string

	// Read all lines
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	// If log is small enough, return as single chunk
	if len(lines) <= ChunkSizeLines {
		return []string{logContent}
	}

	var chunks []string
	startIdx := 0

	for startIdx < len(lines) {
		// Calculate chunk end (target size + overlap for context)
		endIdx := startIdx + ChunkSizeLines
		if endIdx >= len(lines) {
			// Last chunk - take everything remaining
			chunks = append(chunks, strings.Join(lines[startIdx:], "\n"))
			break
		}

		// Find a safe split point: look for non-error lines near the end
		splitIdx := a.findSafeSplitPoint(lines, endIdx)

		// Create chunk with overlap for context preservation
		chunks = append(chunks, strings.Join(lines[startIdx:splitIdx], "\n"))

		// Move start pointer, keeping overlap for context
		startIdx = splitIdx - ContextOverlapLines
		if startIdx < 0 {
			startIdx = 0
		}
	}

	return chunks
}

// findSafeSplitPoint finds a safe place to split the log, preferring non-error lines.
// This minimizes the chance of splitting in the middle of an error's context.
func (a *Agent) findSafeSplitPoint(lines []string, preferredIdx int) int {
	// Search window: 100 lines before preferred index
	searchStart := preferredIdx - 100
	if searchStart < 0 {
		searchStart = 0
	}

	// Look backward from preferred index for a safe line
	for i := preferredIdx; i >= searchStart; i-- {
		line := strings.ToLower(strings.TrimSpace(lines[i]))

		// Skip empty lines
		if len(line) == 0 {
			continue
		}

		// Safe indicators: INFO, DEBUG, or lines without error keywords
		if strings.Contains(line, "[info]") ||
			strings.Contains(line, "[debug]") ||
			strings.Contains(line, "info:") ||
			strings.Contains(line, "debug:") ||
			(!strings.Contains(line, "error") &&
				!strings.Contains(line, "fail") &&
				!strings.Contains(line, "fatal") &&
				!strings.Contains(line, "exception")) {
			return i
		}
	}

	// No safe point found, use preferred index
	return preferredIdx
}
