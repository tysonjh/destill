package buildkite

import (
	"context"
	"destill-agent/src/provider"
)

func init() {
	// Register the Buildkite provider factory
	provider.RegisterProvider("buildkite", func(token string) provider.Provider {
		return NewProvider(token)
	})
}

// Provider implements provider.Provider for Buildkite
type Provider struct {
	client *Client
}

// NewProvider creates a Buildkite provider with API token
func NewProvider(token string) *Provider {
	return &Provider{
		client: NewClient(token),
	}
}

// Name returns "buildkite"
func (p *Provider) Name() string {
	return "buildkite"
}

// ParseURL delegates to provider.ParseURL
func (p *Provider) ParseURL(url string) (*provider.BuildRef, error) {
	return provider.ParseURL(url)
}

// FetchBuild retrieves build metadata using Buildkite API
func (p *Provider) FetchBuild(ctx context.Context, ref *provider.BuildRef) (*provider.Build, error) {
	org := ref.Metadata["org"]
	pipeline := ref.Metadata["pipeline"]
	buildNum := ref.BuildID

	bkBuild, err := p.client.GetBuild(ctx, org, pipeline, buildNum)
	if err != nil {
		return nil, err
	}

	build := &provider.Build{
		ID:        bkBuild.ID,
		Number:    bkBuild.Number,
		URL:       bkBuild.WebURL,
		State:     bkBuild.State,
		Timestamp: bkBuild.CreatedAt,
		Jobs:      make([]provider.Job, 0, len(bkBuild.Jobs)),
	}

	for _, bkJob := range bkBuild.Jobs {
		build.Jobs = append(build.Jobs, provider.Job{
			ID:        bkJob.ID,
			Name:      bkJob.Name,
			Type:      bkJob.Type,
			State:     bkJob.State,
			ExitCode:  bkJob.ExitStatus,
			BuildID:   bkBuild.ID,
			Timestamp: bkJob.CreatedAt,
		})
	}

	return build, nil
}

// FetchJobLog retrieves raw log content
func (p *Provider) FetchJobLog(ctx context.Context, jobID string) (string, error) {
	return p.client.GetJobLog(ctx, jobID)
}

// FetchArtifacts retrieves artifacts for a job
func (p *Provider) FetchArtifacts(ctx context.Context, jobID string) ([]provider.Artifact, error) {
	bkArtifacts, err := p.client.GetJobArtifacts(ctx, jobID)
	if err != nil {
		return nil, err
	}

	artifacts := make([]provider.Artifact, 0, len(bkArtifacts))
	for _, bkArt := range bkArtifacts {
		artifacts = append(artifacts, provider.Artifact{
			ID:          bkArt.ID,
			JobID:       bkArt.JobID,
			Path:        bkArt.Path,
			DownloadURL: bkArt.DownloadURL,
			FileSize:    bkArt.FileSize,
		})
	}

	return artifacts, nil
}

// DownloadArtifact downloads artifact content
func (p *Provider) DownloadArtifact(ctx context.Context, artifact provider.Artifact) ([]byte, error) {
	return p.client.DownloadArtifact(ctx, artifact.DownloadURL)
}
