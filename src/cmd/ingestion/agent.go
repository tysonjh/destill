// Package ingestion provides the Ingestion Agent for the Destill log triage tool.
// This agent consumes requests from a topic and publishes raw log data.
package ingestion

import (
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/buildkite"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
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

		// Create a LogChunk for this job
		logChunk := contracts.LogChunk{
			ID:        fmt.Sprintf("chunk-%s-%s", request.RequestID, job.ID),
			RequestID: request.RequestID,
			JobName:   job.Name,
			Content:   logContent,
			Timestamp: time.Now().Format(time.RFC3339),
			Metadata: map[string]string{
				"build_url":    request.BuildURL,
				"org":          org,
				"pipeline":     pipeline,
				"build_number": fmt.Sprintf("%d", buildNumber),
				"job_id":       job.ID,
				"job_state":    job.State,
				"job_type":     job.Type,
			},
		}

		// Add exit status if available
		if job.ExitStatus != nil {
			logChunk.Metadata["exit_status"] = fmt.Sprintf("%d", *job.ExitStatus)
		}

		// Marshal and publish to ci_logs_raw topic
		data, err := json.Marshal(logChunk)
		if err != nil {
			a.logger.Error("[IngestionAgent] Error marshaling log chunk for job %s: %v", job.Name, err)
			continue
		}

		if err := a.msgBroker.Publish("ci_logs_raw", data); err != nil {
			a.logger.Error("[IngestionAgent] Error publishing log chunk for job %s: %v", job.Name, err)
			continue
		}

		a.logger.Debug("[IngestionAgent] Published log chunk for job '%s' to 'ci_logs_raw' (%d bytes)",
			job.Name, len(logContent))
	}

	a.logger.Info("[IngestionAgent] Completed processing build request %s", request.RequestID)
	return nil
}
