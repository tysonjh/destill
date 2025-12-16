// Package ingest provides the Ingestion Agent for the distributed architecture.
// This agent consumes requests from Redpanda and publishes log chunks.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/broker"
	_ "destill-agent/src/buildkite" // Import for provider registration
	"destill-agent/src/contracts"
	_ "destill-agent/src/githubactions" // Import for provider registration
	"destill-agent/src/logger"
	"destill-agent/src/provider"
)

// Agent consumes analysis requests and publishes log chunks.
type Agent struct {
	broker broker.Broker
	logger logger.Logger
}

// NewAgent creates a new ingest agent.
func NewAgent(brk broker.Broker, log logger.Logger) *Agent {
	return &Agent{
		broker: brk,
		logger: log,
	}
}

// Run starts the agent's main loop.
// It subscribes to destill.requests and processes incoming build analysis requests.
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("[IngestAgent] Starting...")

	// Subscribe to requests topic
	msgChan, err := a.broker.Subscribe(ctx, contracts.TopicRequests, "destill-ingest")
	if err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", contracts.TopicRequests, err)
	}

	a.logger.Info("[IngestAgent] Listening for requests on '%s' topic...", contracts.TopicRequests)

	// Process messages
	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				a.logger.Info("[IngestAgent] Message channel closed, shutting down")
				return nil
			}

			if err := a.processRequest(ctx, msg); err != nil {
				a.logger.Error("[IngestAgent] Error processing request: %v", err)
			}

		case <-ctx.Done():
			a.logger.Info("[IngestAgent] Context cancelled, shutting down")
			return ctx.Err()
		}
	}
}

// processRequest handles an incoming analysis request.
func (a *Agent) processRequest(ctx context.Context, msg broker.Message) error {
	// Parse request
	var request contracts.AnalysisRequest
	if err := json.Unmarshal(msg.Value, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	a.logger.Info("[IngestAgent] Processing request %s", request.RequestID)
	a.logger.Info("[IngestAgent] Build URL: %s", request.BuildURL)

	// Parse URL to detect provider
	ref, err := provider.ParseURL(request.BuildURL)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to parse build URL: %v", err)
		return fmt.Errorf("failed to parse build URL: %w", err)
	}

	a.logger.Info("[IngestAgent] Detected provider: %s", ref.Provider)

	// Get provider implementation
	prov, err := provider.GetProvider(ref)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to get provider: %v", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Send progress: Downloading build metadata
	a.publishProgress(ctx, request.RequestID, "Downloading build metadata", 0, 0)

	// Fetch build using provider
	build, err := prov.FetchBuild(ctx, ref)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to fetch build: %v", err)
		return fmt.Errorf("failed to fetch build: %w", err)
	}

	buildID := build.ID
	a.logger.Info("[IngestAgent] Fetching build metadata for %s", buildID)
	a.logger.Info("[IngestAgent] Found %d jobs in build (state: %s)", len(build.Jobs), build.State)

	// Count script jobs for progress tracking
	scriptJobs := 0
	for _, job := range build.Jobs {
		if job.Type == "script" || job.Type == "" {
			scriptJobs++
		}
	}

	// Process each job
	totalChunks := 0
	processedJobs := 0
	for _, job := range build.Jobs {
		// Skip non-script jobs (GitHub doesn't have this distinction, so Type may be empty)
		if job.Type != "script" && job.Type != "" {
			a.logger.Debug("[IngestAgent] Skipping non-script job: %s (type: %s)", job.Name, job.Type)
			continue
		}

		a.logger.Info("[IngestAgent] Fetching logs for job: %s (id: %s, state: %s)",
			job.Name, job.ID, job.State)

		// Send progress update
		processedJobs++
		a.publishProgress(ctx, request.RequestID, "Fetching logs", processedJobs, scriptJobs)

		// Fetch job log using provider
		logContent, err := prov.FetchJobLog(ctx, job.ID)
		if err != nil {
			a.logger.Error("[IngestAgent] Failed to fetch log for job %s: %v", job.Name, err)
			continue
		}

		// Prepare metadata
		metadata := map[string]string{
			"build_url":    request.BuildURL,
			"build_id":     build.ID,
			"build_number": build.Number,
			"job_state":    job.State,
			"job_type":     job.Type,
			"exit_status":  fmt.Sprintf("%d", job.ExitCode),
			"provider":     prov.Name(),
		}

		// Add provider-specific metadata
		for k, v := range ref.Metadata {
			metadata[k] = v
		}

		// Chunk the log
		chunks := ChunkLog(logContent, request.RequestID, buildID, job.Name, job.ID, metadata)
		a.logger.Info("[IngestAgent] Split job '%s' into %d chunks", job.Name, len(chunks))

		// Publish each chunk
		for _, chunk := range chunks {
			data, err := json.Marshal(chunk)
			if err != nil {
				a.logger.Error("[IngestAgent] Failed to marshal chunk: %v", err)
				continue
			}

			// Publish to destill.logs.raw with buildID as key for ordering
			if err := a.broker.Publish(ctx, contracts.TopicLogsRaw, buildID, data); err != nil {
				a.logger.Error("[IngestAgent] Failed to publish chunk: %v", err)
				continue
			}

			a.logger.Debug("[IngestAgent] Published %s", FormatChunkInfo(chunk))
			totalChunks++
		}
	}

	a.logger.Info("[IngestAgent] Completed processing request %s (%d log chunks)",
		request.RequestID, totalChunks)

	return nil
}

// publishProgress publishes a progress update to the broker.
func (a *Agent) publishProgress(ctx context.Context, requestID, stage string, current, total int) {
	update := contracts.ProgressUpdate{
		RequestID: requestID,
		Stage:     stage,
		Current:   current,
		Total:     total,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(update)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to marshal progress update: %v", err)
		return
	}

	if err := a.broker.Publish(ctx, contracts.TopicProgress, requestID, data); err != nil {
		a.logger.Error("[IngestAgent] Failed to publish progress update: %v", err)
	}
}
