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
	"destill-agent/src/junit"
	"destill-agent/src/provider"
)

// AnalyzeBuildInput defines the input parameters for analyze_build tool
type AnalyzeBuildInput struct {
	URL string `json:"url" jsonschema:"required"`
}

// AnalyzeBuildOutput defines the output structure for analyze_build tool
type AnalyzeBuildOutput struct {
	BuildURL      string        `json:"build_url"`
	BuildNumber   string        `json:"build_number"`
	BuildState    string        `json:"build_state"`
	Provider      string        `json:"provider"`
	JobsAnalyzed  int           `json:"jobs_analyzed"`
	FindingsCount int           `json:"findings_count"`
	Findings      []FindingItem `json:"findings"`
}

// FindingItem represents a single finding in the output
type FindingItem struct {
	Severity        string  `json:"severity"`
	Message         string  `json:"message"`
	JobName         string  `json:"job_name"`
	ConfidenceScore float64 `json:"confidence_score"`
	Source          string  `json:"source"` // "log" or "junit"
}

// RegisterAnalyzeBuildTool registers the analyze_build tool with the MCP server
func RegisterAnalyzeBuildTool(server *mcp.Server) {
	tool := &mcp.Tool{
		Name:        "analyze_build",
		Description: "Analyze a CI/CD build and return findings sorted by confidence. Supports Buildkite and GitHub Actions. Returns error messages, test failures, and other issues found in build logs.",
	}

	handler := func(ctx context.Context, req *mcp.CallToolRequest, args AnalyzeBuildInput) (*mcp.CallToolResult, any, error) {
		// Validate input
		if args.URL == "" {
			return nil, nil, fmt.Errorf("url parameter is required")
		}

		// Run the analysis
		output, err := analyzeBuild(ctx, args.URL)
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
func analyzeBuild(ctx context.Context, buildURL string) (*AnalyzeBuildOutput, error) {
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
					Source:          "log",
				})
			}
		}

		// Fetch and analyze JUnit artifacts
		artifacts, err := prov.FetchArtifacts(ctx, job.ID)
		if err != nil {
			// Artifacts may not exist, continue
			continue
		}

		for _, artifact := range artifacts {
			// Only process JUnit XML files
			if !strings.HasPrefix(artifact.Path, "junit") || !strings.HasSuffix(artifact.Path, ".xml") {
				continue
			}

			// Download artifact
			data, err := prov.DownloadArtifact(ctx, artifact)
			if err != nil {
				continue
			}

			// Parse JUnit XML
			failures, err := junit.Parse(data)
			if err != nil {
				continue
			}

			// Add JUnit failures as findings with confidence 1.0
			for _, failure := range failures {
				message := fmt.Sprintf("Test failed: %s.%s", failure.SuiteName, failure.TestName)
				if failure.Message != "" {
					message += fmt.Sprintf(" - %s", failure.Message)
				}

				allFindings = append(allFindings, FindingItem{
					Severity:        "ERROR",
					Message:         message,
					JobName:         job.Name,
					ConfidenceScore: 1.0, // JUnit failures have highest confidence
					Source:          "junit",
				})
			}
		}
	}

	// Sort findings by confidence score (descending)
	sort.Slice(allFindings, func(i, j int) bool {
		return allFindings[i].ConfidenceScore > allFindings[j].ConfidenceScore
	})

	output := &AnalyzeBuildOutput{
		BuildURL:      buildURL,
		BuildNumber:   build.Number,
		BuildState:    build.State,
		Provider:      prov.Name(),
		JobsAnalyzed:  jobsAnalyzed,
		FindingsCount: len(allFindings),
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
	sb.WriteString(fmt.Sprintf("Total Findings: %d\n\n", output.FindingsCount))

	if output.FindingsCount == 0 {
		sb.WriteString("No issues found.\n")
		return sb.String()
	}

	sb.WriteString(fmt.Sprintf("Findings (sorted by confidence):\n"))
	sb.WriteString(fmt.Sprintf("================================\n\n"))

	for i, finding := range output.Findings {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, finding.Severity, finding.JobName))
		sb.WriteString(fmt.Sprintf("   Confidence: %.2f\n", finding.ConfidenceScore))
		sb.WriteString(fmt.Sprintf("   Source: %s\n", finding.Source))

		// Truncate long messages
		message := finding.Message
		if len(message) > 200 {
			message = message[:200] + "..."
		}
		sb.WriteString(fmt.Sprintf("   Message: %s\n\n", message))
	}

	return sb.String()
}
