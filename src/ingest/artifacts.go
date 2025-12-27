package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
	"destill-agent/src/provider"
	"destill-agent/src/store"
)

// debugArtifacts returns true if DESTILL_DEBUG_ARTIFACTS is set
func debugArtifacts() bool {
	return os.Getenv("DESTILL_DEBUG_ARTIFACTS") != ""
}

// debugLog writes debug output to a file to avoid interfering with TUI
func debugLog(format string, args ...interface{}) {
	if !debugArtifacts() {
		return
	}
	f, err := os.OpenFile("/tmp/destill-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, format+"", args...)
}

// ArtifactAgent fetches and processes JUnit XML artifacts from builds.
type ArtifactAgent struct {
	broker  broker.Broker
	history *store.TestHistory
	logger  logger.Logger
}

// NewArtifactAgent creates a new artifact agent.
func NewArtifactAgent(brk broker.Broker, history *store.TestHistory, log logger.Logger) *ArtifactAgent {
	return &ArtifactAgent{
		broker:  brk,
		history: history,
		logger:  log,
	}
}

// RunWithChannel runs the agent's processing loop using a pre-subscribed channel.
func (a *ArtifactAgent) RunWithChannel(ctx context.Context, msgChan <-chan broker.Message) error {
	a.logger.Info("[ArtifactAgent] Listening for requests on '%s' topic...", contracts.TopicRequests)

	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				a.logger.Info("[ArtifactAgent] Message channel closed, shutting down")
				return nil
			}

			if err := a.processRequest(ctx, msg); err != nil {
				a.logger.Error("[ArtifactAgent] Error processing request: %v", err)
			}

		case <-ctx.Done():
			a.logger.Info("[ArtifactAgent] Context cancelled, shutting down")
			return ctx.Err()
		}
	}
}

// processRequest handles an incoming analysis request.
func (a *ArtifactAgent) processRequest(ctx context.Context, msg broker.Message) error {
	debugLog("ArtifactAgent received message on requests topic")

	// Parse request
	var request contracts.AnalysisRequest
	if err := json.Unmarshal(msg.Value, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	debugLog("Processing request %s for URL: %s", request.RequestID, request.BuildURL)
	a.logger.Info("[ArtifactAgent] Processing request %s", request.RequestID)

	// Parse URL to detect provider
	ref, err := provider.ParseURL(request.BuildURL)
	if err != nil {
		debugLog("ERROR: failed to parse build URL: %v", err)
		return fmt.Errorf("failed to parse build URL: %w", err)
	}
	debugLog("Parsed URL - provider: %s, buildID: %s", ref.Provider, ref.BuildID)

	// Get provider implementation
	prov, err := provider.GetProvider(ref)
	if err != nil {
		debugLog("ERROR: failed to get provider: %v", err)
		return fmt.Errorf("failed to get provider: %w", err)
	}
	debugLog("Got provider: %s", prov.Name())

	// Fetch build to get job list
	debugLog("Fetching build...")
	build, err := prov.FetchBuild(ctx, ref)
	if err != nil {
		debugLog("ERROR: failed to fetch build: %v", err)
		return fmt.Errorf("failed to fetch build: %w", err)
	}
	debugLog("Fetched build with %d jobs", len(build.Jobs))

	// Extract pipeline ID for history tracking
	pipelineID := extractPipelineID(ref)
	buildNumber := parseBuildNumber(build.Number)

	debugLog("Processing %d jobs for pipeline %s build %d", len(build.Jobs), pipelineID, buildNumber)

	// Always fetch fresh artifacts - builds may have retried jobs
	// History is still recorded for flaky detection
	a.logger.Info("[ArtifactAgent] Fetching artifacts for %d jobs (pipeline: %s, build: %d)",
		len(build.Jobs), pipelineID, buildNumber)

	// Publish progress update
	a.publishProgress(ctx, request.RequestID, "Fetching test artifacts", 0, len(build.Jobs))

	// Process each job's artifacts
	var allResults []ParsedTestResult
	hasAuthFailure := false
	for i, job := range build.Jobs {
		// Update progress
		a.publishProgress(ctx, request.RequestID, "Fetching test artifacts", i+1, len(build.Jobs))

		results, authFailed, err := a.processJobArtifacts(ctx, prov, job, request.RequestID, pipelineID, buildNumber, request.BuildURL)
		if err != nil {
			a.logger.Error("[ArtifactAgent] Error processing artifacts for job %s: %v", job.Name, err)
			continue
		}
		if authFailed {
			hasAuthFailure = true
		}
		allResults = append(allResults, results...)
	}

	// Publish warning if there were auth failures
	if hasAuthFailure {
		a.publishWarning(ctx, request.RequestID, "Artifact download failed - set ARTIFACT_SERVER_USER and ARTIFACT_SERVER_PASSWORD for custom artifact servers")
	}

	debugLog("Completed processing request %s (%d test results from artifacts)",
		request.RequestID, len(allResults))
	a.logger.Info("[ArtifactAgent] Completed processing request %s (%d test results from artifacts)",
		request.RequestID, len(allResults))

	return nil
}

// publishProgress publishes a progress update to the progress topic.
func (a *ArtifactAgent) publishProgress(ctx context.Context, requestID, stage string, current, total int) {
	update := contracts.ProgressUpdate{
		RequestID: requestID,
		Stage:     stage,
		Current:   current,
		Total:     total,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(update)
	if err != nil {
		return
	}

	a.broker.Publish(ctx, contracts.TopicProgress, requestID, data)
}

// publishWarning publishes a warning message to the progress topic.
func (a *ArtifactAgent) publishWarning(ctx context.Context, requestID, warning string) {
	update := contracts.ProgressUpdate{
		RequestID: requestID,
		Stage:     "Warning",
		Warning:   warning,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(update)
	if err != nil {
		a.logger.Error("[ArtifactAgent] Failed to marshal warning: %v", err)
		return
	}

	if err := a.broker.Publish(ctx, contracts.TopicProgress, requestID, data); err != nil {
		a.logger.Error("[ArtifactAgent] Failed to publish warning: %v", err)
	}
}

// processJobArtifacts fetches and parses artifacts for a single job.
// Returns parsed results and a boolean indicating if there were auth failures.
func (a *ArtifactAgent) processJobArtifacts(
	ctx context.Context,
	prov provider.Provider,
	job provider.Job,
	requestID, pipelineID string,
	buildNumber int,
	buildURL string,
) ([]ParsedTestResult, bool, error) {
	// Fetch artifact list
	artifacts, err := prov.FetchArtifacts(ctx, job.ID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to fetch artifacts: %w", err)
	}

	if debugArtifacts() {
		debugLog("Job %s: found %d artifacts", job.Name, len(artifacts))
	}

	if len(artifacts) == 0 {
		return nil, false, nil
	}

	var allResults []ParsedTestResult
	hasAuthFailure := false

	// Process each XML artifact
	xmlCount := 0
	for _, artifact := range artifacts {
		if !IsXMLFile(artifact.Path) {
			continue
		}
		xmlCount++

		if debugArtifacts() {
			debugLog("Downloading XML artifact: %s", artifact.Path)
		}
		a.logger.Debug("[ArtifactAgent] Downloading artifact: %s", artifact.Path)

		// Download artifact content
		data, err := prov.DownloadArtifact(ctx, artifact)
		if err != nil {
			if debugArtifacts() {
				debugLog("Download failed: %v", err)
			}
			a.logger.Debug("[ArtifactAgent] Failed to download %s: %v", artifact.Path, err)
			// Check if this is an auth failure
			if strings.Contains(err.Error(), "ARTIFACT_SERVER_USER") {
				hasAuthFailure = true
			} else if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
				hasAuthFailure = true
			}
			continue
		}

		if debugArtifacts() {
			debugLog("Downloaded %d bytes", len(data))
		}

		// Try to parse as JUnit XML
		results, err := ParseJUnitXML(data)
		if err != nil {
			if debugArtifacts() {
				debugLog("Parse failed: %v", err)
			}
			a.logger.Debug("[ArtifactAgent] Failed to parse %s: %v", artifact.Path, err)
			continue
		}

		if len(results) == 0 {
			if debugArtifacts() {
				debugLog("No test results in %s (not JUnit format?)", artifact.Path)
			}
			// Not a JUnit XML file or empty
			continue
		}

		if debugArtifacts() {
			debugLog("Parsed %d test results from %s", len(results), artifact.Path)
		}
		a.logger.Debug("[ArtifactAgent] Parsed %d test results from %s", len(results), artifact.Path)
		allResults = append(allResults, results...)

		// Store results in history and publish to broker
		for _, result := range results {
			// Store in SQLite history
			if a.history != nil {
				historyResult := store.TestResult{
					PipelineID:     pipelineID,
					TestName:       result.TestName,
					BuildNumber:    buildNumber,
					Passed:         result.Passed,
					FailureMessage: result.FailureMessage,
					BuildURL:       buildURL,
					JobName:        job.Name,
					CreatedAt:      time.Now().UTC(),
				}
				if err := a.history.RecordResult(ctx, historyResult); err != nil {
					a.logger.Error("[ArtifactAgent] Failed to store test result: %v", err)
				}
			}

			// Publish to broker
			testResult := contracts.TestResult{
				RequestID:      requestID,
				PipelineID:     pipelineID,
				BuildNumber:    buildNumber,
				BuildURL:       buildURL,
				JobName:        job.Name,
				TestName:       result.TestName,
				ClassName:      result.ClassName,
				Passed:         result.Passed,
				FailureMessage: result.FailureMessage,
				Duration:       result.Duration,
			}

			data, err := json.Marshal(testResult)
			if err != nil {
				a.logger.Error("[ArtifactAgent] Failed to marshal test result: %v", err)
				continue
			}

			if err := a.broker.Publish(ctx, contracts.TopicTestResults, requestID, data); err != nil {
				a.logger.Error("[ArtifactAgent] Failed to publish test result: %v", err)
			}
		}
	}

	if debugArtifacts() {
		debugLog("Job %s summary: %d XML files found, %d test results parsed\n",
			job.Name, xmlCount, len(allResults))
	}

	return allResults, hasAuthFailure, nil
}

// extractPipelineID extracts a pipeline identifier from the build reference.
func extractPipelineID(ref *provider.BuildRef) string {
	switch ref.Provider {
	case "buildkite":
		org := ref.Metadata["org"]
		pipeline := ref.Metadata["pipeline"]
		if org != "" && pipeline != "" {
			return org + "/" + pipeline
		}
	case "github":
		owner := ref.Metadata["owner"]
		repo := ref.Metadata["repo"]
		if owner != "" && repo != "" {
			return owner + "/" + repo
		}
	}
	return ref.Provider + "/" + ref.BuildID
}

// parseBuildNumber parses a build number string to int.
func parseBuildNumber(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ProcessArtifactsForBuild is a convenience function for processing artifacts
// without running the full agent loop. Useful for MCP server and testing.
func ProcessArtifactsForBuild(
	ctx context.Context,
	prov provider.Provider,
	ref *provider.BuildRef,
	build *provider.Build,
	history *store.TestHistory,
	buildURL string,
) ([]contracts.TestResult, error) {
	pipelineID := extractPipelineID(ref)
	buildNumber := parseBuildNumber(build.Number)

	var allResults []contracts.TestResult

	for _, job := range build.Jobs {
		// Fetch artifact list
		artifacts, err := prov.FetchArtifacts(ctx, job.ID)
		if err != nil {
			continue
		}

		// Process each XML artifact
		for _, artifact := range artifacts {
			if !IsXMLFile(artifact.Path) {
				continue
			}

			// Download artifact content
			data, err := prov.DownloadArtifact(ctx, artifact)
			if err != nil {
				continue
			}

			// Try to parse as JUnit XML
			results, err := ParseJUnitXML(data)
			if err != nil || len(results) == 0 {
				continue
			}

			// Convert and store results
			for _, result := range results {
				// Store in history if available
				if history != nil {
					historyResult := store.TestResult{
						PipelineID:     pipelineID,
						TestName:       result.TestName,
						BuildNumber:    buildNumber,
						Passed:         result.Passed,
						FailureMessage: result.FailureMessage,
						BuildURL:       buildURL,
						JobName:        job.Name,
						CreatedAt:      time.Now().UTC(),
					}
					history.RecordResult(ctx, historyResult)
				}

				testResult := contracts.TestResult{
					PipelineID:     pipelineID,
					BuildNumber:    buildNumber,
					BuildURL:       buildURL,
					JobName:        job.Name,
					TestName:       result.TestName,
					ClassName:      result.ClassName,
					Passed:         result.Passed,
					FailureMessage: result.FailureMessage,
					Duration:       result.Duration,
				}
				allResults = append(allResults, testResult)
			}
		}
	}

	return allResults, nil
}
