package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/pipeline"
	"destill-agent/src/provider"
	"destill-agent/src/store"
)

// Server is the MCP server for destill.
type Server struct {
	mcpServer *server.MCPServer
	store     store.Store
}

// NewServer creates a new MCP server with the given store.
func NewServer(st store.Store) *Server {
	mcpSrv := server.NewMCPServer(
		"destill",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	srv := &Server{
		mcpServer: mcpSrv,
		store:     st,
	}
	srv.registerTools()

	return srv
}

// registerTools registers all available tools.
func (s *Server) registerTools() {
	analyzeTool := mcp.NewTool("analyze_build",
		mcp.WithDescription("Analyze a CI/CD build and return tiered findings. Returns all tier 1 findings (unique failures) fully expanded with context - these are the likely root causes. Tier 2-3 findings are summarized; use get_finding_details to drill into them if needed."),
		mcp.WithString("url",
			mcp.Required(),
			mcp.Description("Build URL (Buildkite or GitHub Actions)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Max findings per tier (default: 15)"),
		),
	)

	detailsTool := mcp.NewTool("get_finding_details",
		mcp.WithDescription("Get full details for a specific finding, including context lines. Use after analyze_build to drill into findings."),
		mcp.WithString("request_id",
			mcp.Required(),
			mcp.Description("Request ID from analyze_build response"),
		),
		mcp.WithString("finding_id",
			mcp.Required(),
			mcp.Description("Finding ID (message_hash) from the manifest"),
		),
	)

	s.mcpServer.AddTool(analyzeTool, s.handleAnalyzeBuild)
	s.mcpServer.AddTool(detailsTool, s.handleGetFindingDetails)
}

// Run starts the MCP server on stdio.
func (s *Server) Run() error {
	return server.ServeStdio(s.mcpServer)
}

// handleAnalyzeBuild handles the analyze_build tool call.
// Returns a lightweight manifest; use get_finding_details for full context.
func (s *Server) handleAnalyzeBuild(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Extract parameters
	url := request.GetString("url", "")
	if url == "" {
		return mcp.NewToolResultError("url parameter is required"), nil
	}

	limit := request.GetInt("limit", 15)

	// Run analysis
	cards, buildInfo, testSummary, err := s.runAnalysis(ctx, url)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analysis failed: %v", err)), nil
	}

	// Extract request ID from cards
	requestID := ExtractRequestID(cards)
	if requestID == "" {
		requestID = generateRequestID()
	}

	// Store raw cards for drill-down
	if err := s.store.Store(ctx, requestID, cards); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to store findings: %v", err)), nil
	}

	// Record findings to SQLite history for novelty detection
	s.recordFindingsToHistory(ctx, buildInfo, cards)

	// Tier findings on read
	response := TierFindings(cards, limit)
	response.Build = buildInfo

	// Return lightweight manifest with test results
	manifest := ToManifest(requestID, response, testSummary)
	jsonBytes, err := json.Marshal(manifest)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal response: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleGetFindingDetails handles the get_finding_details tool call.
// Returns full finding with context lines for a specific finding ID.
func (s *Server) handleGetFindingDetails(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestID := request.GetString("request_id", "")
	if requestID == "" {
		return mcp.NewToolResultError("request_id parameter is required"), nil
	}

	findingID := request.GetString("finding_id", "")
	if findingID == "" {
		return mcp.NewToolResultError("finding_id parameter is required"), nil
	}

	// Look up the card by hash
	card, err := s.store.GetByHash(ctx, requestID, findingID)
	if err != nil {
		var notFound store.ErrNotFound
		if errors.As(err, &notFound) {
			return mcp.NewToolResultError(fmt.Sprintf("finding not found: request_id=%s, finding_id=%s", requestID, findingID)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("failed to get finding: %v", err)), nil
	}

	// Convert TriageCard to Finding
	finding := CardToFinding(card)

	// Return full finding with context
	jsonBytes, err := json.Marshal(finding)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal finding: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// runAnalysis runs the full analysis pipeline and collects cards.
func (s *Server) runAnalysis(ctx context.Context, buildURL string) ([]contracts.TriageCard, BuildInfo, *contracts.TestSummary, error) {
	// Validate URL and token upfront to fail fast
	ref, err := provider.ParseURL(buildURL)
	if err != nil {
		return nil, BuildInfo{}, nil, provider.WrapError(err)
	}
	if err := provider.ValidateToken(ref); err != nil {
		return nil, BuildInfo{}, nil, provider.WrapError(err)
	}

	// Create in-memory broker and start pipeline
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Subscribe to topics BEFORE starting pipeline to avoid race conditions
	findingsCh, err := msgBroker.Subscribe(ctx, contracts.TopicAnalysisFindings, "mcp-server-findings")
	if err != nil {
		return nil, BuildInfo{}, nil, fmt.Errorf("failed to subscribe to findings: %w", err)
	}
	testsCh, err := msgBroker.Subscribe(ctx, contracts.TopicTestResults, "mcp-server-tests")
	if err != nil {
		return nil, BuildInfo{}, nil, fmt.Errorf("failed to subscribe to test results: %w", err)
	}
	metadataCh, err := msgBroker.Subscribe(ctx, contracts.TopicBuildMetadata, "mcp-server-metadata")
	if err != nil {
		return nil, BuildInfo{}, nil, fmt.Errorf("failed to subscribe to build metadata: %w", err)
	}

	if err := pipeline.Start(msgBroker, pipelineCtx); err != nil {
		return nil, BuildInfo{}, nil, fmt.Errorf("failed to start pipeline: %w", err)
	}

	// Submit analysis request
	requestID := generateRequestID()
	req := contracts.AnalysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	reqData, err := json.Marshal(req)
	if err != nil {
		return nil, BuildInfo{}, nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	msgBroker.Publish(ctx, contracts.TopicRequests, requestID, reqData)

	// Collect findings, test results, and build metadata with timeout
	cards, testResults, buildMeta, err := s.collectFromChannels(ctx, findingsCh, testsCh, metadataCh)
	if err != nil {
		return nil, BuildInfo{}, nil, err
	}

	// Build info from authoritative metadata (fallback to card-based extraction)
	buildInfo := buildInfoFromMetadata(buildMeta, cards, testResults, buildURL)

	// Build test summary if we have test results
	var testSummary *contracts.TestSummary
	if len(testResults) > 0 {
		testSummary = buildTestSummary(requestID, testResults)
	}

	return cards, buildInfo, testSummary, nil
}

// collectFromChannels collects findings, test results, and build metadata from pre-subscribed channels until timeout.
func (s *Server) collectFromChannels(ctx context.Context, findingsCh, testsCh, metadataCh <-chan broker.Message) ([]contracts.TriageCard, []contracts.TestResult, *contracts.BuildMetadata, error) {
	var cards []contracts.TriageCard
	var testResults []contracts.TestResult
	var buildMeta *contracts.BuildMetadata
	timeout := time.After(120 * time.Second)
	lastActivity := time.Now()

	for {
		select {
		case msg := <-findingsCh:
			var card contracts.TriageCard
			if err := json.Unmarshal(msg.Value, &card); err == nil {
				cards = append(cards, card)
				lastActivity = time.Now()
			}
		case msg := <-testsCh:
			var result contracts.TestResult
			if err := json.Unmarshal(msg.Value, &result); err == nil {
				testResults = append(testResults, result)
				lastActivity = time.Now()
			}
		case msg := <-metadataCh:
			var meta contracts.BuildMetadata
			if err := json.Unmarshal(msg.Value, &meta); err == nil {
				buildMeta = &meta
				lastActivity = time.Now()
			}
		case <-timeout:
			return cards, testResults, buildMeta, nil
		case <-ctx.Done():
			return cards, testResults, buildMeta, ctx.Err()
		default:
			if time.Since(lastActivity) > 10*time.Second && len(cards) > 0 {
				return cards, testResults, buildMeta, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// buildInfoFromMetadata creates BuildInfo from authoritative metadata.
// Returns "unknown" status if metadata is not available.
func buildInfoFromMetadata(meta *contracts.BuildMetadata, cards []contracts.TriageCard, testResults []contracts.TestResult, url string) BuildInfo {
	// Extract job counts from cards (metadata doesn't have per-job info)
	failedJobs := make(map[string]bool)
	passedJobs := make(map[string]bool)
	otherJobs := make(map[string]bool)

	for _, card := range cards {
		switch card.Metadata["job_state"] {
		case "failed":
			failedJobs[card.JobName] = true
		case "passed":
			passedJobs[card.JobName] = true
		case "":
			// Skip cards without job_state metadata
		default:
			otherJobs[card.JobName] = true
		}
	}

	// Track which jobs have test results
	jobsWithTests := make(map[string]bool)
	for _, tr := range testResults {
		if tr.JobName != "" {
			jobsWithTests[tr.JobName] = true
		}
	}

	// Classify failed jobs by whether they have tests
	var failed, failedWithTests, failedNoTests []string
	for job := range failedJobs {
		failed = append(failed, job)
		if jobsWithTests[job] {
			failedWithTests = append(failedWithTests, job)
		} else {
			failedNoTests = append(failedNoTests, job)
		}
	}

	// Return unknown status if no authoritative metadata
	if meta == nil {
		return BuildInfo{
			URL:             url,
			Status:          "unknown",
			FailedJobs:      failed,
			FailedWithTests: failedWithTests,
			FailedNoTests:   failedNoTests,
			PassedCount:     len(passedJobs),
			OtherCount:      len(otherJobs),
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
		}
	}

	return BuildInfo{
		URL:             meta.URL,
		Number:          meta.Number,
		Status:          meta.State,
		Branch:          meta.Branch,
		Commit:          meta.Commit,
		Message:         meta.Message,
		Source:          meta.Source,
		StartedAt:       meta.StartedAt,
		FinishedAt:      meta.FinishedAt,
		Duration:        meta.Duration,
		FailedJobs:      failed,
		FailedWithTests: failedWithTests,
		FailedNoTests:   failedNoTests,
		PassedCount:     len(passedJobs),
		OtherCount:      len(otherJobs),
		Timestamp:       meta.Timestamp,
	}
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	timestamp := time.Now().UTC().Format("20060102T150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return fmt.Sprintf("req-%s-%s", timestamp, hex.EncodeToString(randomBytes))
}

// recordFindingsToHistory records findings to SQLite for novelty detection.
// This is best-effort; errors are logged but don't fail the request.
func (s *Server) recordFindingsToHistory(ctx context.Context, buildInfo BuildInfo, cards []contracts.TriageCard) {
	if len(cards) == 0 || buildInfo.Number == "" {
		return
	}

	// Parse build number
	buildNumber, err := strconv.Atoi(buildInfo.Number)
	if err != nil || buildNumber == 0 {
		return
	}

	// Build pipeline ID from URL (e.g., "org/pipeline")
	ref, err := provider.ParseURL(buildInfo.URL)
	if err != nil {
		return
	}
	pipelineID := ref.Metadata["org"] + "/" + ref.Metadata["pipeline"]
	if pipelineID == "/" {
		return
	}

	// Open test history (same pattern as buildTestSummary)
	history, err := store.NewTestHistory("")
	if err != nil {
		return
	}
	defer history.Close()

	// Record findings (best-effort, don't fail if it errors)
	_ = history.RecordFindingsFromCards(ctx, pipelineID, buildNumber, cards)
}

// buildTestSummary creates a TestSummary from collected test results.
// Uses test history to classify failures as novel vs flaky.
func buildTestSummary(requestID string, results []contracts.TestResult) *contracts.TestSummary {
	if len(results) == 0 {
		return nil
	}

	summary := &contracts.TestSummary{
		RequestID: requestID,
	}

	// Extract pipeline info from first result
	if len(results) > 0 {
		summary.PipelineID = results[0].PipelineID
		summary.BuildNumber = results[0].BuildNumber
	}

	// Open test history for flaky detection
	history, err := store.NewTestHistory("")
	if err != nil {
		// If we can't open history, fall back to marking all as novel
		history = nil
	}
	if history != nil {
		defer history.Close()
	}

	ctx := context.Background()

	// Count pass/fail and classify failures
	for _, r := range results {
		summary.TotalTests++
		if r.Passed {
			summary.PassedCount++
		} else {
			summary.FailedCount++

			failure := contracts.TestFailure{
				TestName:       r.TestName,
				FailureMessage: r.FailureMessage,
			}

			// Check test history for flakiness and last failure info
			isFlaky := false
			if history != nil {
				flakeInfo, err := history.GetTestFlakeInfo(ctx, summary.PipelineID, r.TestName, summary.BuildNumber)
				if err == nil {
					// Always capture last failure date if available
					if !flakeInfo.LastFailedAt.IsZero() {
						failure.LastFailedAt = flakeInfo.LastFailedAt.Format(time.RFC3339)
					}
					// Mark as flaky if it meets the criteria
					if flakeInfo.IsFlaky {
						isFlaky = true
						failure.IsFlaky = true
						failure.HistoryRuns = flakeInfo.TotalRuns
						failure.HistoryFails = flakeInfo.FailedRuns
					}
				}
			}

			if isFlaky {
				summary.FlakyFailures = append(summary.FlakyFailures, failure)
			} else {
				summary.NovelFailures = append(summary.NovelFailures, failure)
			}
		}
	}

	return summary
}
