// Package mcp implements MCP tools for Destill.
package mcp

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"destill-agent/src/analyze"
	"destill-agent/src/ingest"
	"destill-agent/src/provider"
)

// AnalyzeBuildInput defines the input parameters for analyze_build tool
type AnalyzeBuildInput struct {
	URL    string `json:"url" jsonschema:"required"`
	Offset int    `json:"offset,omitempty"` // Skip first N findings (default: 0)
	Limit  int    `json:"limit,omitempty"`  // Max findings to return (default: 25)
}

// AnalyzeBuildOutput defines the output structure for analyze_build tool
type AnalyzeBuildOutput struct {
	BuildURL      string        `json:"build_url"`
	BuildNumber   string        `json:"build_number"`
	BuildState    string        `json:"build_state"`
	Provider      string        `json:"provider"`
	JobsAnalyzed  int           `json:"jobs_analyzed"`
	TotalFindings int           `json:"total_findings"` // Total findings before pagination
	Offset        int           `json:"offset"`         // Current offset
	Limit         int           `json:"limit"`          // Current limit
	HasMore       bool          `json:"has_more"`       // More findings available
	Findings      []FindingItem `json:"findings"`
}

// FindingItem represents a single finding in the output
type FindingItem struct {
	Severity        string  `json:"severity"`
	Message         string  `json:"message"`
	JobName         string  `json:"job_name"`
	ConfidenceScore float64 `json:"confidence_score"`
}

// RegisterAnalyzeBuildTool registers the analyze_build tool with the MCP server
func RegisterAnalyzeBuildTool(server *mcp.Server) {
	tool := &mcp.Tool{
		Name:        "analyze_build",
		Description: "Analyze a CI/CD build and return findings sorted by confidence. Supports Buildkite and GitHub Actions. Returns top 25 findings by default. Use offset/limit params to paginate (e.g., offset=25 for next page).",
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, args AnalyzeBuildInput) (*mcp.CallToolResult, any, error) {
		// Validate input
		if args.URL == "" {
			return nil, nil, fmt.Errorf("url parameter is required")
		}

		// Apply defaults for pagination
		limit := args.Limit
		if limit <= 0 {
			limit = 25
		}
		offset := args.Offset
		if offset < 0 {
			offset = 0
		}

		// Run the analysis
		output, err := analyzeBuild(ctx, args.URL, offset, limit)
		if err != nil {
			return nil, nil, err
		}

		// Format the output as text content
		textContent := formatAnalysisOutput(output)

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: textContent,
				},
			},
		}

		return result, output, nil
	}

	mcp.AddTool(server, tool, handler)
}

// analyzeBuild performs the actual build analysis
func analyzeBuild(ctx context.Context, buildURL string, offset, limit int) (*AnalyzeBuildOutput, error) {
	// Parse the URL to detect provider
	ref, err := provider.ParseURL(buildURL)
	if err != nil {
		return nil, fmt.Errorf("invalid build URL: %w", err)
	}

	// Get the appropriate provider
	prov, err := provider.GetProvider(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	// Fetch build metadata
	build, err := prov.FetchBuild(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch build: %w", err)
	}

	// Collect all findings from all jobs
	var allFindings []FindingItem
	jobsAnalyzed := 0

	for _, job := range build.Jobs {
		// Skip non-script jobs for Buildkite
		if job.Type != "script" && job.Type != "" {
			continue
		}

		jobsAnalyzed++

		// Fetch job logs
		logContent, err := prov.FetchJobLog(ctx, job.ID)
		if err != nil {
			// Log error but continue with other jobs
			continue
		}

		// Chunk the logs
		requestID := fmt.Sprintf("mcp-%d", time.Now().UnixNano())
		metadata := map[string]string{
			"build_url": buildURL,
		}
		chunks := ingest.ChunkLog(logContent, requestID, build.ID, job.Name, job.ID, metadata)

		// Analyze each chunk
		for _, chunk := range chunks {
			findings := analyze.AnalyzeChunk(chunk)

			for _, finding := range findings {
				allFindings = append(allFindings, FindingItem{
					Severity:        finding.Severity,
					Message:         finding.RawMessage,
					JobName:         job.Name,
					ConfidenceScore: finding.ConfidenceScore,
				})
			}
		}
	}

	// Sort findings by confidence score (descending)
	sort.Slice(allFindings, func(i, j int) bool {
		return allFindings[i].ConfidenceScore > allFindings[j].ConfidenceScore
	})

	// Apply pagination
	totalFindings := len(allFindings)
	hasMore := false

	if offset >= totalFindings {
		allFindings = []FindingItem{}
	} else {
		end := offset + limit
		if end > totalFindings {
			end = totalFindings
		} else {
			hasMore = true
		}
		allFindings = allFindings[offset:end]
	}

	output := &AnalyzeBuildOutput{
		BuildURL:      buildURL,
		BuildNumber:   build.Number,
		BuildState:    build.State,
		Provider:      prov.Name(),
		JobsAnalyzed:  jobsAnalyzed,
		TotalFindings: totalFindings,
		Offset:        offset,
		Limit:         limit,
		HasMore:       hasMore,
		Findings:      allFindings,
	}

	return output, nil
}

// formatAnalysisOutput formats the analysis output as human-readable text
func formatAnalysisOutput(output *AnalyzeBuildOutput) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Build Analysis Results\n"))
	sb.WriteString(fmt.Sprintf("======================\n\n"))
	sb.WriteString(fmt.Sprintf("Build URL: %s\n", output.BuildURL))
	sb.WriteString(fmt.Sprintf("Build Number: %s\n", output.BuildNumber))
	sb.WriteString(fmt.Sprintf("Build State: %s\n", output.BuildState))
	sb.WriteString(fmt.Sprintf("Provider: %s\n", output.Provider))
	sb.WriteString(fmt.Sprintf("Jobs Analyzed: %d\n", output.JobsAnalyzed))
	sb.WriteString(fmt.Sprintf("Total Findings: %d\n", output.TotalFindings))
	sb.WriteString(fmt.Sprintf("Showing: %d-%d of %d\n\n", output.Offset+1, output.Offset+len(output.Findings), output.TotalFindings))

	if output.TotalFindings == 0 {
		sb.WriteString("No issues found.\n")
		return sb.String()
	}

	if len(output.Findings) == 0 {
		sb.WriteString("No more findings at this offset.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Findings (sorted by confidence):\n"))
	sb.WriteString(fmt.Sprintf("================================\n\n"))

	for i, finding := range output.Findings {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", output.Offset+i+1, finding.Severity, finding.JobName))
		sb.WriteString(fmt.Sprintf("   Confidence: %.2f\n", finding.ConfidenceScore))

		// Truncate long messages
		message := finding.Message
		if len(message) > 200 {
			message = message[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("   Message: %s\n\n", message))
	}

	if output.HasMore {
		sb.WriteString(fmt.Sprintf("--- More findings available. Use offset=%d to see next page ---\n", output.Offset+output.Limit))
	}

	return sb.String()
}
