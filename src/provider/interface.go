package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrInvalidURL      = errors.New("invalid build URL")
	ErrProviderUnknown = errors.New("unknown CI provider")
)

// Provider defines the interface for CI/CD platform integrations
type Provider interface {
	// Name returns the provider name (e.g., "buildkite", "github")
	Name() string

	// ParseURL extracts build reference from URL
	ParseURL(url string) (*BuildRef, error)

	// FetchBuild retrieves build metadata and jobs
	FetchBuild(ctx context.Context, ref *BuildRef) (*Build, error)

	// FetchJobLog retrieves raw log content for a job
	FetchJobLog(ctx context.Context, jobID string) (string, error)

	// FetchArtifacts retrieves list of artifacts for a job
	FetchArtifacts(ctx context.Context, jobID string) ([]Artifact, error)

	// DownloadArtifact downloads artifact content
	DownloadArtifact(ctx context.Context, artifact Artifact) ([]byte, error)
}

var (
	buildkiteURLPattern = regexp.MustCompile(`^https://buildkite\.com/([^/]+)/([^/]+)/builds/(\d+)`)
	githubURLPattern    = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/actions/runs/(\d+)`)
)

// ParseURL detects provider and parses build reference from URL
func ParseURL(url string) (*BuildRef, error) {
	// Try Buildkite pattern
	if matches := buildkiteURLPattern.FindStringSubmatch(url); matches != nil {
		return &BuildRef{
			Provider: "buildkite",
			BuildID:  matches[3],
			Metadata: map[string]string{
				"org":      matches[1],
				"pipeline": matches[2],
			},
		}, nil
	}

	// Try GitHub Actions pattern
	if matches := githubURLPattern.FindStringSubmatch(url); matches != nil {
		return &BuildRef{
			Provider: "github",
			BuildID:  matches[3],
			Metadata: map[string]string{
				"owner": matches[1],
				"repo":  matches[2],
			},
		}, nil
	}

	return nil, fmt.Errorf("%w: %s", ErrInvalidURL, url)
}

// GetProvider returns the appropriate provider implementation for a build ref
func GetProvider(ref *BuildRef) (Provider, error) {
	// Will be implemented in next tasks when we create providers
	return nil, fmt.Errorf("%w: %s", ErrProviderUnknown, ref.Provider)
}
