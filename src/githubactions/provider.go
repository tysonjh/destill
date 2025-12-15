package githubactions

import (
	"context"
	"destill-agent/src/provider"
	"fmt"
	"strconv"
	"strings"
)

func init() {
	// Register the GitHub Actions provider factory
	provider.RegisterProvider("github", func(token string) provider.Provider {
		return NewProvider(token)
	})
}

// Provider implements provider.Provider for GitHub Actions
type Provider struct {
	client *Client
}

// NewProvider creates a GitHub Actions provider with API token
func NewProvider(token string) *Provider {
	return &Provider{
		client: NewClient(token),
	}
}

// Name returns "github"
func (p *Provider) Name() string {
	return "github"
}

// ParseURL delegates to provider.ParseURL
func (p *Provider) ParseURL(url string) (*provider.BuildRef, error) {
	return provider.ParseURL(url)
}

// FetchBuild retrieves workflow run metadata using GitHub API
func (p *Provider) FetchBuild(ctx context.Context, ref *provider.BuildRef) (*provider.Build, error) {
	owner := ref.Metadata["owner"]
	repo := ref.Metadata["repo"]
	runID := ref.BuildID

	run, err := p.client.GetWorkflowRun(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	jobs, err := p.client.GetWorkflowJobs(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	build := &provider.Build{
		ID:        fmt.Sprintf("%d", run.ID),
		Number:    fmt.Sprintf("%d", run.RunNumber),
		URL:       run.HTMLURL,
		State:     mapGitHubStatus(run.Status, run.Conclusion),
		Timestamp: run.CreatedAt,
		Jobs:      make([]provider.Job, 0, len(jobs)),
	}

	for _, ghJob := range jobs {
		exitCode := 0
		if ghJob.Conclusion == "failure" {
			exitCode = 1
		}

		build.Jobs = append(build.Jobs, provider.Job{
			ID:        fmt.Sprintf("%d", ghJob.ID),
			Name:      ghJob.Name,
			Type:      "script", // GitHub Actions doesn't distinguish types
			State:     mapGitHubStatus(ghJob.Status, ghJob.Conclusion),
			ExitCode:  exitCode,
			BuildID:   fmt.Sprintf("%d", run.ID),
			Timestamp: ghJob.StartedAt,
		})
	}

	return build, nil
}

// FetchJobLog retrieves raw log content for a job
func (p *Provider) FetchJobLog(ctx context.Context, jobID string) (string, error) {
	// Extract owner/repo from stored metadata (we'll need to pass this differently)
	// For now, parse from job ID format "owner/repo/jobID"
	parts := strings.Split(jobID, "/")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid job ID format: %s", jobID)
	}

	owner := parts[0]
	repo := parts[1]
	id, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid job ID number: %s", parts[2])
	}

	return p.client.GetJobLogs(ctx, owner, repo, id)
}

// FetchArtifacts retrieves artifacts for the workflow run
func (p *Provider) FetchArtifacts(ctx context.Context, jobID string) ([]provider.Artifact, error) {
	// GitHub artifacts are per-run, not per-job
	// Parse job ID to extract owner/repo/runID
	// This is a limitation - we'll need to refactor to pass run ID
	// For now, return empty list
	return []provider.Artifact{}, nil
}

// DownloadArtifact downloads artifact content (returns first file from zip)
func (p *Provider) DownloadArtifact(ctx context.Context, artifact provider.Artifact) ([]byte, error) {
	files, err := p.client.DownloadArtifact(ctx, artifact.DownloadURL)
	if err != nil {
		return nil, err
	}

	// Return first file (JUnit XMLs are typically single files)
	for _, content := range files {
		return content, nil
	}

	return nil, fmt.Errorf("no files in artifact")
}

// mapGitHubStatus maps GitHub status/conclusion to Buildkite-like state
func mapGitHubStatus(status, conclusion string) string {
	if status == "completed" {
		switch conclusion {
		case "success":
			return "passed"
		case "failure":
			return "failed"
		case "cancelled":
			return "canceled"
		default:
			return conclusion
		}
	}
	return status
}
