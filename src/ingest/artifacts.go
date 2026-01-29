package ingest

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// logDownloadsEnabled returns true if DESTILL_DOWNLOAD_LOGS is set to true
func logDownloadsEnabled() bool {
	val := strings.ToLower(os.Getenv("DESTILL_DOWNLOAD_LOGS"))
	return val == "true" || val == "1" || val == "yes"
}

// getMaxLogFileSize returns the max size per log file in bytes (default: 10MB)
func getMaxLogFileSize() int64 {
	val := os.Getenv("DESTILL_MAX_LOG_FILE_SIZE")
	if val == "" {
		return 10 * 1024 * 1024 // 10MB default
	}
	size, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 10 * 1024 * 1024 // 10MB default on parse error
	}
	return size
}

// getMaxTotalLogSize returns the max total size for all logs in bytes (default: 50MB)
func getMaxTotalLogSize() int64 {
	val := os.Getenv("DESTILL_MAX_TOTAL_LOG_SIZE")
	if val == "" {
		return 50 * 1024 * 1024 // 50MB default
	}
	size, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 50 * 1024 * 1024 // 50MB default on parse error
	}
	return size
}

// isLogFile checks if a filename is a log file (.log or .log.gz)
func isLogFile(filename string) bool {
	lower := strings.ToLower(filename)
	return strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".log.gz")
}

// extractFailedTestNames extracts test names from failed test results
func extractFailedTestNames(results []ParsedTestResult) []string {
	var failed []string
	for _, result := range results {
		if !result.Passed {
			// Collect various name forms to improve matching
			if result.TestName != "" {
				failed = append(failed, result.TestName)
			}
			if result.Name != "" {
				failed = append(failed, result.Name)
			}
			if result.ClassName != "" {
				failed = append(failed, result.ClassName)
			}
		}
	}
	return failed
}

// matchesFailedTest checks if an artifact path matches any failed test name
func matchesFailedTest(artifactPath string, failedTests []string) bool {
	lowerPath := strings.ToLower(artifactPath)
	for _, testName := range failedTests {
		lowerTest := strings.ToLower(testName)
		// Extract test base name (remove package/class prefixes)
		parts := strings.Split(lowerTest, ".")
		baseName := parts[len(parts)-1]

		// Check if path contains the test name or base name
		if strings.Contains(lowerPath, lowerTest) || strings.Contains(lowerPath, baseName) {
			return true
		}
	}
	return false
}

// getArtifactCacheDir returns the cache directory for artifacts
func getArtifactCacheDir(requestID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, ".destill", "cache", requestID, "artifacts")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}
	return cacheDir, nil
}

// saveLogFile saves a log file to the local cache and returns the decompressed content
func saveLogFile(data []byte, artifactPath, requestID string) ([]byte, error) {
	cacheDir, err := getArtifactCacheDir(requestID)
	if err != nil {
		return nil, err
	}

	// Clean the artifact path to create a safe filename
	filename := filepath.Base(artifactPath)
	filepath := filepath.Join(cacheDir, filename)

	// Save the original file
	if err := os.WriteFile(filepath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// If it's gzipped, decompress and save decompressed version
	if strings.HasSuffix(strings.ToLower(artifactPath), ".gz") {
		decompressed, err := decompressGzip(data)
		if err != nil {
			// Return original data if decompression fails
			return data, nil
		}

		// Save decompressed version
		decompressedPath := strings.TrimSuffix(filepath, ".gz")
		if err := os.WriteFile(decompressedPath, decompressed, 0644); err != nil {
			// Non-fatal, just return decompressed data
			return decompressed, nil
		}

		return decompressed, nil
	}

	return data, nil
}

// decompressGzip decompresses gzip data
func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress: %w", err)
	}

	return decompressed, nil
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

	// Second pass: Process log files if enabled
	if logDownloadsEnabled() {
		failedTests := extractFailedTestNames(allResults)

		// If no XML test results but job failed, download logs opportunistically
		if len(failedTests) == 0 && len(allResults) == 0 && job.ExitCode != 0 {
			a.logger.Info("[ArtifactAgent] Job %s failed with no XML results, downloading log files opportunistically", job.Name)
			if debugArtifacts() {
				debugLog("Job %s failed (exit %d) with no XML, downloading all log files", job.Name, job.ExitCode)
			}
			// Empty failedTests list means processLogArtifacts will download all log files (with size limits)
			logCount, bytesDownloaded := a.processLogArtifacts(ctx, prov, artifacts, nil, requestID, job.Name, buildURL)
			if logCount > 0 {
				a.logger.Info("[ArtifactAgent] Downloaded %d log files (%d bytes) for job %s", logCount, bytesDownloaded, job.Name)
			}
		} else if len(failedTests) > 0 {
			a.logger.Info("[ArtifactAgent] Found %d failed tests, checking for log files", len(failedTests))
			if debugArtifacts() {
				debugLog("Failed tests: %v", failedTests)
			}

			logCount, bytesDownloaded := a.processLogArtifacts(ctx, prov, artifacts, failedTests, requestID, job.Name, buildURL)
			if logCount > 0 {
				a.logger.Info("[ArtifactAgent] Downloaded %d log files (%d bytes) for job %s", logCount, bytesDownloaded, job.Name)
			}
		}
	}

	if debugArtifacts() {
		debugLog("Job %s summary: %d XML files found, %d test results parsed\n",
			job.Name, xmlCount, len(allResults))
	}

	return allResults, hasAuthFailure, nil
}

// processLogArtifacts downloads and processes log files for failed tests.
// Returns count of logs downloaded and total bytes.
func (a *ArtifactAgent) processLogArtifacts(
	ctx context.Context,
	prov provider.Provider,
	artifacts []provider.Artifact,
	failedTests []string,
	requestID, jobName, buildURL string,
) (int, int64) {
	maxFileSize := getMaxLogFileSize()
	maxTotalSize := getMaxTotalLogSize()

	var totalBytes int64
	logCount := 0

	for _, artifact := range artifacts {
		// Check if this is a log file
		if !isLogFile(artifact.Path) {
			continue
		}

		// Check if it matches a failed test (skip if failedTests provided but no match)
		// If failedTests is nil/empty, download all log files (opportunistic mode)
		if failedTests != nil && len(failedTests) > 0 && !matchesFailedTest(artifact.Path, failedTests) {
			if debugArtifacts() {
				debugLog("Skipping log file (no match): %s", artifact.Path)
			}
			continue
		}

		// Check file size limit
		if artifact.FileSize > maxFileSize {
			a.logger.Debug("[ArtifactAgent] Skipping large log file %s (%d bytes > %d limit)",
				artifact.Path, artifact.FileSize, maxFileSize)
			continue
		}

		// Check total size limit
		if totalBytes+artifact.FileSize > maxTotalSize {
			a.logger.Info("[ArtifactAgent] Reached total log size limit (%d bytes), skipping remaining logs", maxTotalSize)
			break
		}

		if debugArtifacts() {
			debugLog("Downloading log file: %s (%d bytes)", artifact.Path, artifact.FileSize)
		}
		a.logger.Debug("[ArtifactAgent] Downloading log file: %s", artifact.Path)

		// Download the log file
		data, err := prov.DownloadArtifact(ctx, artifact)
		if err != nil {
			a.logger.Debug("[ArtifactAgent] Failed to download log %s: %v", artifact.Path, err)
			continue
		}

		// Save locally and get decompressed content
		decompressed, err := saveLogFile(data, artifact.Path, requestID)
		if err != nil {
			a.logger.Error("[ArtifactAgent] Failed to save log file %s: %v", artifact.Path, err)
			// Continue with analysis even if save failed
			decompressed = data
		}

		// Analyze the log content and publish findings
		a.analyzeAndPublishLog(ctx, decompressed, artifact.Path, requestID, jobName, buildURL)

		totalBytes += artifact.FileSize
		logCount++

		if debugArtifacts() {
			debugLog("Successfully processed log file: %s", artifact.Path)
		}
	}

	return logCount, totalBytes
}

// analyzeAndPublishLog analyzes log content and publishes findings to the broker.
func (a *ArtifactAgent) analyzeAndPublishLog(
	ctx context.Context,
	content []byte,
	artifactPath, requestID, jobName, buildURL string,
) {
	// Import the analyze package at the top if not already imported
	// For now, we'll just publish the log content as a log chunk
	// The analyze agent will pick it up and process it

	// Split content into manageable chunks (similar to how job logs are chunked)
	logContent := string(content)
	chunks := ChunkLog(logContent, requestID, "artifact-"+artifactPath, jobName, artifactPath, map[string]string{
		"build_url":     buildURL,
		"artifact_path": artifactPath,
		"source":        "artifact_log",
	})

	// Publish each chunk to the logs topic for analysis
	for _, chunk := range chunks {
		data, err := json.Marshal(chunk)
		if err != nil {
			a.logger.Error("[ArtifactAgent] Failed to marshal log chunk: %v", err)
			continue
		}

		if err := a.broker.Publish(ctx, contracts.TopicLogsRaw, requestID, data); err != nil {
			a.logger.Error("[ArtifactAgent] Failed to publish log chunk: %v", err)
		}
	}

	if len(chunks) > 0 {
		a.logger.Debug("[ArtifactAgent] Published %d chunks from artifact log %s", len(chunks), artifactPath)
	}
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
