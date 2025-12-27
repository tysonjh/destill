package githubactions

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"destill-agent/src/provider"
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
	// Cached artifacts expanded from zip files (GitHub artifacts are per-run, not per-job)
	// Each entry represents a single file extracted from artifact zips
	cachedArtifacts []provider.Artifact
	// Cached file contents keyed by artifact ID
	cachedFiles map[string][]byte
	// Owner/repo for artifact operations
	owner string
	repo  string
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

	// Cache owner/repo for artifact operations
	p.owner = owner
	p.repo = repo

	run, err := p.client.GetWorkflowRun(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	jobs, err := p.client.GetWorkflowJobs(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}

	// Fetch and expand artifacts for the run (GitHub artifacts are per-run, not per-job)
	// Each GitHub artifact is a zip file - we expand them to individual file entries
	ghArtifacts, err := p.client.GetArtifacts(ctx, owner, repo, runID)
	if err != nil {
		// Don't fail the build fetch if artifacts fail - they're optional
		ghArtifacts = nil
	}
	p.cachedArtifacts = nil
	p.cachedFiles = make(map[string][]byte)
	for _, art := range ghArtifacts {
		// Download and expand this artifact zip
		files, err := p.client.DownloadArtifact(ctx, art.ArchiveDownloadURL)
		if err != nil {
			// Skip artifacts that fail to download
			continue
		}
		// Create an artifact entry for each file in the zip
		for filename, content := range files {
			artifactID := fmt.Sprintf("%d/%s", art.ID, filename)
			p.cachedArtifacts = append(p.cachedArtifacts, provider.Artifact{
				ID:          artifactID,
				JobID:       "", // GitHub artifacts aren't tied to specific jobs
				Path:        filename,
				DownloadURL: artifactID, // Use ID as key for cached lookup
				FileSize:    int64(len(content)),
			})
			p.cachedFiles[artifactID] = content
		}
	}

	// Determine finished time - use UpdatedAt if run is complete
	var finishedAt time.Time
	if run.Status == "completed" {
		finishedAt = run.UpdatedAt
	}

	build := &provider.Build{
		ID:         fmt.Sprintf("%d", run.ID),
		Number:     fmt.Sprintf("%d", run.RunNumber),
		URL:        run.HTMLURL,
		State:      mapGitHubStatus(run.Status, run.Conclusion),
		Branch:     run.HeadBranch,
		Commit:     run.HeadSHA,
		Message:    "", // GitHub API doesn't include commit message in workflow run
		Source:     run.Event,
		StartedAt:  run.RunStartedAt,
		FinishedAt: finishedAt,
		Timestamp:  run.CreatedAt,
		Jobs:       make([]provider.Job, 0, len(jobs)),
	}

	for _, ghJob := range jobs {
		exitCode := 0
		if ghJob.Conclusion == "failure" {
			exitCode = 1
		}

		build.Jobs = append(build.Jobs, provider.Job{
			ID:        fmt.Sprintf("%s/%s/%d", owner, repo, ghJob.ID),
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

// FetchArtifacts retrieves artifacts for the workflow run.
// Note: GitHub artifacts are per-run, not per-job. This returns all run artifacts
// regardless of jobID. Callers should deduplicate if processing multiple jobs.
func (p *Provider) FetchArtifacts(ctx context.Context, jobID string) ([]provider.Artifact, error) {
	// Return cached artifacts from FetchBuild
	// Since GitHub artifacts are per-run, we return the same artifacts for any job
	return p.cachedArtifacts, nil
}

// DownloadArtifact returns cached artifact content.
// GitHub artifacts are downloaded and expanded during FetchBuild.
func (p *Provider) DownloadArtifact(ctx context.Context, artifact provider.Artifact) ([]byte, error) {
	// Look up in cache using the artifact ID (which is the DownloadURL for cached artifacts)
	if content, ok := p.cachedFiles[artifact.DownloadURL]; ok {
		return content, nil
	}
	return nil, fmt.Errorf("artifact not found in cache: %s", artifact.ID)
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
