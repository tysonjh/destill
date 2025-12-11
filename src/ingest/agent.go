// Package ingest provides the Ingestion Agent for the agentic architecture.
// This agent consumes requests from Redpanda and publishes log chunks.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"destill-agent/src/broker"
	"destill-agent/src/buildkite"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
)

// Agent consumes analysis requests and publishes log chunks.
type Agent struct {
	broker          broker.Broker
	buildkiteClient *buildkite.Client
	logger          logger.Logger
}

// NewAgent creates a new ingest agent.
func NewAgent(brk broker.Broker, buildkiteToken string, log logger.Logger) *Agent {
	return &Agent{
		broker:          brk,
		buildkiteClient: buildkite.NewClient(buildkiteToken),
		logger:          log,
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

	// Extract build information
	org, pipeline, buildNumber, err := buildkite.ParseBuildURL(request.BuildURL)
	if err != nil {
		return fmt.Errorf("failed to parse build URL: %w", err)
	}

	buildID := fmt.Sprintf("%s-%s-%d", org, pipeline, buildNumber)
	a.logger.Info("[IngestAgent] Fetching build metadata for %s", buildID)

	// Fetch build metadata
	build, err := a.buildkiteClient.GetBuild(org, pipeline, buildNumber)
	if err != nil {
		return fmt.Errorf("failed to fetch build: %w", err)
	}

	a.logger.Info("[IngestAgent] Found %d jobs in build (state: %s)", len(build.Jobs), build.State)

	// Process each job
	totalChunks := 0
	for _, job := range build.Jobs {
		// Skip non-script jobs
		if job.Type != "script" {
			a.logger.Debug("[IngestAgent] Skipping non-script job: %s (type: %s)", job.Name, job.Type)
			continue
		}

		a.logger.Info("[IngestAgent] Fetching logs for job: %s (id: %s, state: %s)",
			job.Name, job.ID, job.State)

		// Fetch job log
		logContent, err := a.buildkiteClient.GetJobLog(org, pipeline, buildNumber, job.ID)
		if err != nil {
			a.logger.Error("[IngestAgent] Failed to fetch log for job %s: %v", job.Name, err)
			continue
		}

		// Prepare metadata
		metadata := map[string]string{
			"build_url":    request.BuildURL,
			"org":          org,
			"pipeline":     pipeline,
			"build_number": fmt.Sprintf("%d", buildNumber),
			"job_state":    job.State,
			"job_type":     job.Type,
		}

		if job.ExitStatus != nil {
			metadata["exit_status"] = fmt.Sprintf("%d", *job.ExitStatus)
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

	a.logger.Info("[IngestAgent] Completed processing request %s (%d chunks published)",
		request.RequestID, totalChunks)

	return nil
}
