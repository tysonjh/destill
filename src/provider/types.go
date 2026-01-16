package provider

import "time"

// BuildRef identifies a build in a CI system
type BuildRef struct {
	Provider string            // "buildkite" or "github"
	BuildID  string            // Unique build identifier
	Metadata map[string]string // Provider-specific metadata
}

// Build represents a CI build with jobs
type Build struct {
	ID          string
	Number      string
	URL         string
	State       string    // passed, failed, canceled, running, etc.
	Branch      string    // Git branch name
	Commit      string    // Git commit SHA
	Message     string    // Commit message
	Source      string    // Build trigger: webhook, api, schedule, ui
	StartedAt   time.Time // When first job started
	FinishedAt  time.Time // When build completed (zero if still running)
	Timestamp   time.Time // Created at
	Jobs        []Job
	GitHubOwner string // GitHub repository owner (extracted from repo URL for Buildkite)
	GitHubRepo  string // GitHub repository name (extracted from repo URL for Buildkite)
}

// Job represents a single job within a build
type Job struct {
	ID        string
	Name      string
	Type      string
	State     string
	ExitCode  int
	BuildID   string
	Timestamp time.Time
}

// Artifact represents a build artifact
type Artifact struct {
	ID          string
	JobID       string
	Path        string
	DownloadURL string
	FileSize    int64
}
