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
	ID        string
	Number    string
	URL       string
	State     string
	Timestamp time.Time
	Jobs      []Job
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
