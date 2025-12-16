package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/pipeline"
	"destill-agent/src/provider"
)

// Server is the MCP server for destill.
type Server struct {
	mcpServer *server.MCPServer
	store     FindingsStore
}

// NewServer creates a new MCP server.
func NewServer() *Server {
	s := server.NewMCPServer(
		"destill",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	srv := &Server{
		mcpServer: s,
		store:     NewInMemoryStore(),
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
	cards, buildInfo, err := s.runAnalysis(ctx, url)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("analysis failed: %v", err)), nil
	}

	// Extract request ID from cards
	requestID := ExtractRequestID(cards)
	if requestID == "" {
		requestID = generateRequestID()
	}

	// Tier the findings
	response := TierFindings(cards, limit)
	response.Build = buildInfo

	// Store full findings for drill-down
	s.store.Store(requestID, response)

	// Return lightweight manifest
	manifest := ToManifest(requestID, response)
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

	// Look up the finding
	finding, found := s.store.Get(requestID, findingID)
	if !found {
		return mcp.NewToolResultError(fmt.Sprintf("finding not found: request_id=%s, finding_id=%s", requestID, findingID)), nil
	}

	// Return full finding with context
	jsonBytes, err := json.Marshal(finding)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to marshal finding: %v", err)), nil
	}

	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// runAnalysis runs the full analysis pipeline and collects cards.
func (s *Server) runAnalysis(ctx context.Context, buildURL string) ([]contracts.TriageCard, BuildInfo, error) {
	// Validate URL
	if _, err := provider.ParseURL(buildURL); err != nil {
		return nil, BuildInfo{}, provider.WrapError(err)
	}

	// Create in-memory broker and start pipeline
	msgBroker := broker.NewInMemoryBroker()
	defer msgBroker.Close()

	pipelineCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := pipeline.Start(msgBroker, pipelineCtx); err != nil {
		return nil, BuildInfo{}, fmt.Errorf("failed to start pipeline: %w", err)
	}

	// Submit analysis request
	requestID := generateRequestID()
	req := contracts.AnalysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	reqData, _ := json.Marshal(req)
	msgBroker.Publish(ctx, contracts.TopicRequests, requestID, reqData)

	// Collect findings with timeout
	cards, err := s.collectFindings(ctx, msgBroker)
	if err != nil {
		return nil, BuildInfo{}, err
	}

	// Build info
	buildInfo := extractBuildInfo(cards, buildURL)

	return cards, buildInfo, nil
}

// collectFindings subscribes to findings and collects them until timeout.
func (s *Server) collectFindings(ctx context.Context, msgBroker broker.Broker) ([]contracts.TriageCard, error) {
	ch, err := msgBroker.Subscribe(ctx, contracts.TopicAnalysisFindings, "mcp-server")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	var cards []contracts.TriageCard
	timeout := time.After(120 * time.Second)
	lastActivity := time.Now()

	for {
		select {
		case msg := <-ch:
			var card contracts.TriageCard
			if err := json.Unmarshal(msg.Value, &card); err == nil {
				cards = append(cards, card)
				lastActivity = time.Now()
			}
		case <-timeout:
			return cards, nil
		case <-ctx.Done():
			return cards, ctx.Err()
		default:
			if time.Since(lastActivity) > 10*time.Second && len(cards) > 0 {
				return cards, nil
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// extractBuildInfo extracts build metadata from cards.
func extractBuildInfo(cards []contracts.TriageCard, url string) BuildInfo {
	failedJobs := make(map[string]bool)
	passedJobs := make(map[string]bool)

	for _, card := range cards {
		if card.Metadata["job_state"] == "failed" {
			failedJobs[card.JobName] = true
		} else {
			passedJobs[card.JobName] = true
		}
	}

	var failed []string
	for job := range failedJobs {
		failed = append(failed, job)
	}

	status := "passed"
	if len(failed) > 0 {
		status = "failed"
	}

	return BuildInfo{
		URL:             url,
		Status:          status,
		FailedJobs:      failed,
		PassedJobsCount: len(passedJobs),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}
}

// generateRequestID creates a unique request identifier.
func generateRequestID() string {
	timestamp := time.Now().UTC().Format("20060102T150405")
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return fmt.Sprintf("req-%s-%s", timestamp, hex.EncodeToString(randomBytes))
}
