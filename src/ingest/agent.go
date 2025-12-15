// Package ingest provides the Ingestion Agent for the distributed architecture.
// This agent consumes requests from Redpanda and publishes log chunks.
package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"destill-agent/src/broker"
	_ "destill-agent/src/buildkite" // Import for provider registration
	"destill-agent/src/contracts"
	_ "destill-agent/src/githubactions" // Import for provider registration
	"destill-agent/src/junit"
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
	totalJUnitFindings := 0
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

		// Also fetch and process JUnit artifacts for this job
		junitFindings := a.processJUnitArtifacts(ctx, prov, request, job)
		totalJUnitFindings += junitFindings
	}

	a.logger.Info("[IngestAgent] Completed processing request %s (%d log chunks, %d JUnit findings)",
		request.RequestID, totalChunks, totalJUnitFindings)

	return nil
}

// processJUnitArtifacts fetches and processes JUnit XML artifacts for a job.
// Returns the number of test failures found.
func (a *Agent) processJUnitArtifacts(ctx context.Context, prov provider.Provider,
	req contracts.AnalysisRequest, job provider.Job) int {

	// Fetch artifacts for this job
	artifacts, err := prov.FetchArtifacts(ctx, job.ID)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to fetch artifacts for job %s: %v", job.Name, err)
		return 0
	}

	if len(artifacts) == 0 {
		a.logger.Debug("[IngestAgent] No artifacts found for job %s", job.Name)
		return 0
	}

	a.logger.Debug("[IngestAgent] Found %d artifacts for job %s", len(artifacts), job.Name)

	totalFindings := 0

	// Process each artifact that looks like JUnit XML
	for _, artifact := range artifacts {
		if !isJUnitArtifact(artifact.Path) {
			continue
		}

		findings := a.processJUnitArtifact(ctx, prov, req, job, artifact)
		totalFindings += findings
	}

	if totalFindings > 0 {
		a.logger.Info("[IngestAgent] Processed JUnit artifacts for job '%s': %d test failures",
			job.Name, totalFindings)
	}

	return totalFindings
}

// isJUnitArtifact checks if an artifact path looks like a JUnit XML file.
func isJUnitArtifact(path string) bool {
	lower := strings.ToLower(path)
	// Look for junit*.xml or **/junit*.xml patterns
	return strings.Contains(lower, "junit") && strings.HasSuffix(lower, ".xml")
}

// processJUnitArtifact downloads and processes a single JUnit XML artifact.
// Returns the number of test failures found.
func (a *Agent) processJUnitArtifact(ctx context.Context, prov provider.Provider,
	req contracts.AnalysisRequest, job provider.Job, artifact provider.Artifact) int {

	a.logger.Debug("[IngestAgent] Processing JUnit artifact: %s", artifact.Path)

	// Download the artifact
	data, err := prov.DownloadArtifact(ctx, artifact)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to download artifact %s: %v", artifact.Path, err)
		return 0
	}

	// Parse JUnit XML
	failures, err := junit.Parse(data)
	if err != nil {
		a.logger.Error("[IngestAgent] Failed to parse JUnit XML %s: %v", artifact.Path, err)
		return 0
	}

	if len(failures) == 0 {
		a.logger.Debug("[IngestAgent] No test failures in %s (all tests passed)", artifact.Path)
		return 0
	}

	a.logger.Info("[IngestAgent] Found %d test failures in %s", len(failures), artifact.Path)

	// Convert each test failure to a TriageCard and publish
	publishedCount := 0
	for _, failure := range failures {
		card := a.createTriageCardFromJUnit(req, job, artifact.Path, failure)

		// Marshal the card
		data, err := json.Marshal(card)
		if err != nil {
			a.logger.Error("[IngestAgent] Failed to marshal JUnit finding: %v", err)
			continue
		}

		// Publish directly to findings topic (bypasses analyze agent)
		// JUnit failures are definitive and don't need heuristic analysis
		if err := a.broker.Publish(ctx, contracts.TopicAnalysisFindings, req.RequestID, data); err != nil {
			a.logger.Error("[IngestAgent] Failed to publish JUnit finding: %v", err)
			continue
		}

		a.logger.Debug("[IngestAgent] Published JUnit finding: %s", failure.GetNormalizedName())
		publishedCount++
	}

	return publishedCount
}

// createTriageCardFromJUnit converts a JUnit test failure to a TriageCard.
func (a *Agent) createTriageCardFromJUnit(req contracts.AnalysisRequest, job provider.Job,
	artifactPath string, failure junit.TestFailure) contracts.TriageCard {

	return contracts.TriageCard{
		// Identity
		ID:          fmt.Sprintf("%s-%s-%s", req.RequestID, job.ID, failure.GenerateHash()),
		RequestID:   req.RequestID,
		MessageHash: failure.GenerateHash(),

		// Source
		Source:   fmt.Sprintf("junit:%s", artifactPath),
		JobName:  job.Name,
		BuildURL: req.BuildURL,

		// Content
		Severity:        "error", // Test failures are always errors
		RawMessage:      failure.GetDisplayMessage(),
		NormalizedMsg:   failure.GetNormalizedName(),
		ConfidenceScore: 1.0, // JUnit failures are definitive

		// Context (stack trace as post-context)
		PreContext:  []string{}, // No pre-context for JUnit
		PostContext: failure.SplitStackTrace(50),
		ContextNote: "JUnit test failure (structured data)",

		// Chunk info (N/A for JUnit)
		ChunkIndex:  0,
		LineInChunk: 0,

		// Metadata
		Metadata: map[string]string{
			"source_type":   "junit",
			"artifact_path": artifactPath,
			"test_name":     failure.TestName,
			"class_name":    failure.ClassName,
			"suite_name":    failure.SuiteName,
			"failure_type":  failure.Type,
			"duration_sec":  fmt.Sprintf("%.3f", failure.Duration),
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
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
