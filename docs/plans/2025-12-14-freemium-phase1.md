# Freemium Phase 1 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Deliver MVP free tier with GitHub Actions support, polished local mode UX, and MCP server for Claude integration.

**Architecture:** Extend existing provider pattern with GitHub Actions client, abstract BuildProvider interface, add MCP server as standalone binary. Maintain existing ingest/analyze agents with zero changes to core analysis logic.

**Tech Stack:** Go 1.24+, GitHub REST API v3, MCP SDK for Go (or stdio protocol), existing Buildkite/Redpanda/Postgres stack.

---

## Task 1: Create Build Provider Interface

**Files:**
- Create: `src/provider/interface.go`
- Create: `src/provider/types.go`
- Create: `src/provider/interface_test.go`

**Step 1: Write the failing test**

Create `src/provider/interface_test.go`:

```go
package provider

import (
	"context"
	"testing"
)

func TestBuildProvider_ParseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "buildkite URL",
			url:     "https://buildkite.com/org/pipeline/builds/123",
			wantErr: false,
		},
		{
			name:    "github actions URL",
			url:     "https://github.com/owner/repo/actions/runs/456",
			wantErr: false,
		},
		{
			name:    "invalid URL",
			url:     "https://example.com/invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./src/provider/... -v`
Expected: FAIL with "undefined: ParseURL"

**Step 3: Write minimal types**

Create `src/provider/types.go`:

```go
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

// Artifact represents a build artifact (e.g., JUnit XML)
type Artifact struct {
	ID          string
	JobID       string
	Path        string
	DownloadURL string
	FileSize    int64
}
```

**Step 4: Write interface and URL parser**

Create `src/provider/interface.go`:

```go
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
```

**Step 5: Run test to verify it passes**

Run: `go test ./src/provider/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add src/provider/
git commit -m "feat: add build provider interface

Create provider abstraction for multi-CI support with BuildRef, Build,
Job, and Artifact types. Add URL parsing for Buildkite and GitHub
Actions patterns.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 2: Refactor Buildkite Client to Implement Provider Interface

**Files:**
- Modify: `src/buildkite/client.go`
- Create: `src/buildkite/provider.go`
- Create: `src/buildkite/provider_test.go`

**Step 1: Write the failing test**

Create `src/buildkite/provider_test.go`:

```go
package buildkite

import (
	"context"
	"destill-agent/src/provider"
	"testing"
)

func TestBuildkiteProvider_Name(t *testing.T) {
	p := NewProvider("fake-token")
	if p.Name() != "buildkite" {
		t.Errorf("Name() = %v, want buildkite", p.Name())
	}
}

func TestBuildkiteProvider_ParseURL(t *testing.T) {
	p := NewProvider("fake-token")

	ref, err := p.ParseURL("https://buildkite.com/myorg/mypipeline/builds/123")
	if err != nil {
		t.Fatalf("ParseURL() error = %v", err)
	}

	if ref.Provider != "buildkite" {
		t.Errorf("Provider = %v, want buildkite", ref.Provider)
	}
	if ref.BuildID != "123" {
		t.Errorf("BuildID = %v, want 123", ref.BuildID)
	}
	if ref.Metadata["org"] != "myorg" {
		t.Errorf("org = %v, want myorg", ref.Metadata["org"])
	}
	if ref.Metadata["pipeline"] != "mypipeline" {
		t.Errorf("pipeline = %v, want mypipeline", ref.Metadata["pipeline"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./src/buildkite/... -v`
Expected: FAIL with "undefined: NewProvider"

**Step 3: Create provider wrapper**

Create `src/buildkite/provider.go`:

```go
package buildkite

import (
	"context"
	"destill-agent/src/provider"
)

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
```

**Step 4: Update provider.GetProvider to return Buildkite provider**

Modify `src/provider/interface.go`:

```go
// Add import at top
import (
	"destill-agent/src/buildkite"
	"os"
)

// Replace GetProvider function
func GetProvider(ref *BuildRef) (Provider, error) {
	switch ref.Provider {
	case "buildkite":
		token := os.Getenv("BUILDKITE_API_TOKEN")
		if token == "" {
			return nil, errors.New("BUILDKITE_API_TOKEN environment variable not set")
		}
		return buildkite.NewProvider(token), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrProviderUnknown, ref.Provider)
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./src/buildkite/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add src/buildkite/provider.go src/buildkite/provider_test.go src/provider/interface.go
git commit -m "feat: implement Provider interface for Buildkite

Wrap existing Buildkite client with Provider interface for multi-CI
abstraction. Update GetProvider to return Buildkite implementation.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 3: Create GitHub Actions Client

**Files:**
- Create: `src/githubactions/client.go`
- Create: `src/githubactions/types.go`
- Create: `src/githubactions/client_test.go`

**Step 1: Write the failing test**

Create `src/githubactions/client_test.go`:

```go
package githubactions

import (
	"context"
	"testing"
)

func TestClient_NewClient(t *testing.T) {
	client := NewClient("fake-token")
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestClient_ParseWorkflowRunURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantErr  bool
		wantRepo string
		wantRun  string
	}{
		{
			name:     "valid URL",
			url:      "https://github.com/owner/repo/actions/runs/123456",
			wantErr:  false,
			wantRepo: "owner/repo",
			wantRun:  "123456",
		},
		{
			name:    "invalid URL",
			url:     "https://github.com/owner/repo/pulls/123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, runID, err := ParseWorkflowRunURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseWorkflowRunURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				gotRepo := owner + "/" + repo
				if gotRepo != tt.wantRepo {
					t.Errorf("repo = %v, want %v", gotRepo, tt.wantRepo)
				}
				if runID != tt.wantRun {
					t.Errorf("runID = %v, want %v", runID, tt.wantRun)
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./src/githubactions/... -v`
Expected: FAIL with "undefined: NewClient"

**Step 3: Write GitHub Actions types**

Create `src/githubactions/types.go`:

```go
package githubactions

import "time"

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	RunNumber  int       `json:"run_number"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
}

// WorkflowJob represents a job within a workflow run
type WorkflowJob struct {
	ID         int64     `json:"id"`
	RunID      int64     `json:"run_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	StartedAt  time.Time `json:"started_at"`
	Steps      []Step    `json:"steps"`
}

// Step represents a step within a job
type Step struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	Number     int    `json:"number"`
}

// Artifact represents a workflow artifact
type Artifact struct {
	ID                 int64     `json:"id"`
	Name               string    `json:"name"`
	SizeInBytes        int64     `json:"size_in_bytes"`
	ArchiveDownloadURL string    `json:"archive_download_url"`
	Expired            bool      `json:"expired"`
	CreatedAt          time.Time `json:"created_at"`
}

// WorkflowJobsResponse is the API response for listing jobs
type WorkflowJobsResponse struct {
	TotalCount int           `json:"total_count"`
	Jobs       []WorkflowJob `json:"jobs"`
}

// ArtifactsResponse is the API response for listing artifacts
type ArtifactsResponse struct {
	TotalCount int        `json:"total_count"`
	Artifacts  []Artifact `json:"artifacts"`
}
```

**Step 4: Write GitHub Actions client**

Create `src/githubactions/client.go`:

```go
package githubactions

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidURL = errors.New("invalid GitHub Actions URL")
)

var workflowRunURLPattern = regexp.MustCompile(`^https://github\.com/([^/]+)/([^/]+)/actions/runs/(\d+)`)

// Client is a GitHub Actions API client
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new GitHub Actions client
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.github.com",
	}
}

// ParseWorkflowRunURL extracts owner, repo, and run ID from URL
func ParseWorkflowRunURL(url string) (owner, repo, runID string, err error) {
	matches := workflowRunURLPattern.FindStringSubmatch(url)
	if matches == nil {
		return "", "", "", fmt.Errorf("%w: %s", ErrInvalidURL, url)
	}
	return matches[1], matches[2], matches[3], nil
}

// GetWorkflowRun fetches workflow run metadata
func (c *Client) GetWorkflowRun(ctx context.Context, owner, repo, runID string) (*WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s", c.baseURL, owner, repo, runID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var run WorkflowRun
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, err
	}

	return &run, nil
}

// GetWorkflowJobs fetches jobs for a workflow run
func (c *Client) GetWorkflowJobs(ctx context.Context, owner, repo, runID string) ([]WorkflowJob, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s/jobs", c.baseURL, owner, repo, runID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var jobsResp WorkflowJobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&jobsResp); err != nil {
		return nil, err
	}

	return jobsResp.Jobs, nil
}

// GetJobLogs fetches raw logs for a job (returns zip archive URL redirect)
func (c *Client) GetJobLogs(ctx context.Context, owner, repo string, jobID int64) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/jobs/%d/logs", c.baseURL, owner, repo, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	// Don't follow redirects - we want the redirect URL
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusFound {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	// Follow redirect to download logs
	logURL := resp.Header.Get("Location")
	if logURL == "" {
		return "", errors.New("no redirect location for logs")
	}

	logReq, err := http.NewRequestWithContext(ctx, "GET", logURL, nil)
	if err != nil {
		return "", err
	}

	logResp, err := c.httpClient.Do(logReq)
	if err != nil {
		return "", err
	}
	defer logResp.Body.Close()

	body, err := io.ReadAll(logResp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// GetArtifacts fetches artifacts for a workflow run
func (c *Client) GetArtifacts(ctx context.Context, owner, repo, runID string) ([]Artifact, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s/artifacts", c.baseURL, owner, repo, runID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	var artifactsResp ArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&artifactsResp); err != nil {
		return nil, err
	}

	return artifactsResp.Artifacts, nil
}

// DownloadArtifact downloads and extracts artifact zip
func (c *Client) DownloadArtifact(ctx context.Context, downloadURL string) (map[string][]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
	}

	// Read zip archive
	zipData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Extract files from zip
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte)
	for _, file := range zipReader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			return nil, err
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}

		files[file.Name] = content
	}

	return files, nil
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./src/githubactions/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add src/githubactions/
git commit -m "feat: add GitHub Actions API client

Implement client for fetching workflow runs, jobs, logs, and artifacts
from GitHub Actions API. Handle log zip archives and artifact downloads.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 4: Implement GitHub Actions Provider

**Files:**
- Create: `src/githubactions/provider.go`
- Create: `src/githubactions/provider_test.go`
- Modify: `src/provider/interface.go`

**Step 1: Write the failing test**

Create `src/githubactions/provider_test.go`:

```go
package githubactions

import (
	"destill-agent/src/provider"
	"testing"
)

func TestGitHubProvider_Name(t *testing.T) {
	p := NewProvider("fake-token")
	if p.Name() != "github" {
		t.Errorf("Name() = %v, want github", p.Name())
	}
}

func TestGitHubProvider_ParseURL(t *testing.T) {
	p := NewProvider("fake-token")

	ref, err := p.ParseURL("https://github.com/owner/repo/actions/runs/123")
	if err != nil {
		t.Fatalf("ParseURL() error = %v", err)
	}

	if ref.Provider != "github" {
		t.Errorf("Provider = %v, want github", ref.Provider)
	}
	if ref.BuildID != "123" {
		t.Errorf("BuildID = %v, want 123", ref.BuildID)
	}
	if ref.Metadata["owner"] != "owner" {
		t.Errorf("owner = %v, want owner", ref.Metadata["owner"])
	}
	if ref.Metadata["repo"] != "repo" {
		t.Errorf("repo = %v, want repo", ref.Metadata["repo"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./src/githubactions/... -v`
Expected: FAIL with "undefined: NewProvider"

**Step 3: Implement provider wrapper**

Create `src/githubactions/provider.go`:

```go
package githubactions

import (
	"context"
	"destill-agent/src/provider"
	"fmt"
	"strconv"
	"strings"
)

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
	parts := strings.Split(jobID, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid job ID format: %s", jobID)
	}

	owner := parts[0]
	repo := parts[1]
	// For artifacts, we need the run ID, which should be stored in build metadata
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
```

**Step 4: Update provider.GetProvider to support GitHub**

Modify `src/provider/interface.go`:

```go
// Add import
import (
	"destill-agent/src/githubactions"
)

// Update GetProvider function
func GetProvider(ref *BuildRef) (Provider, error) {
	switch ref.Provider {
	case "buildkite":
		token := os.Getenv("BUILDKITE_API_TOKEN")
		if token == "" {
			return nil, errors.New("BUILDKITE_API_TOKEN environment variable not set")
		}
		return buildkite.NewProvider(token), nil
	case "github":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			return nil, errors.New("GITHUB_TOKEN environment variable not set")
		}
		return githubactions.NewProvider(token), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrProviderUnknown, ref.Provider)
	}
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./src/githubactions/... ./src/provider/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add src/githubactions/provider.go src/githubactions/provider_test.go src/provider/interface.go
git commit -m "feat: implement Provider interface for GitHub Actions

Add GitHub Actions provider implementation with workflow run fetching,
job logs, and artifact handling. Update GetProvider to support GitHub.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 5: Update Ingest Agent to Use Provider Interface

**Files:**
- Modify: `src/ingest/agent.go`
- Modify: `src/cmd/destill/main.go`

**Step 1: Analyze current ingest agent**

Read: `src/ingest/agent.go` to understand current Buildkite-specific implementation

**Step 2: Refactor to use Provider interface**

Modify `src/ingest/agent.go`:

Replace Buildkite-specific code with provider abstraction:

```go
// Replace import
import (
	"destill-agent/src/provider"
)

// Update processRequest function signature
func (a *Agent) processRequest(ctx context.Context, msg broker.Message) error {
	// ... existing code ...

	// Parse URL to detect provider
	ref, err := provider.ParseURL(req.BuildURL)
	if err != nil {
		a.logger.Error("failed to parse build URL", "url", req.BuildURL, "error", err)
		return err
	}

	// Get provider implementation
	prov, err := provider.GetProvider(ref)
	if err != nil {
		a.logger.Error("failed to get provider", "provider", ref.Provider, "error", err)
		return err
	}

	// Fetch build using provider
	build, err := prov.FetchBuild(ctx, ref)
	if err != nil {
		a.logger.Error("failed to fetch build", "provider", prov.Name(), "error", err)
		return err
	}

	a.logger.Info("fetched build", "provider", prov.Name(), "jobs", len(build.Jobs))

	// Process jobs
	for _, job := range build.Jobs {
		// Skip non-script jobs (GitHub doesn't have this distinction)
		if job.Type != "script" && job.Type != "" {
			continue
		}

		// Fetch job log
		logContent, err := prov.FetchJobLog(ctx, job.ID)
		if err != nil {
			a.logger.Error("failed to fetch job log", "job", job.Name, "error", err)
			continue
		}

		// Chunk and publish logs (existing logic)
		chunks := ChunkLog(logContent, job.ID, job.Name, req.BuildURL)
		for _, chunk := range chunks {
			chunk.RequestID = req.RequestID
			if err := a.publishLogChunk(ctx, chunk); err != nil {
				a.logger.Error("failed to publish chunk", "error", err)
			}
		}

		// Fetch and process artifacts (JUnit)
		artifacts, err := prov.FetchArtifacts(ctx, job.ID)
		if err != nil {
			a.logger.Error("failed to fetch artifacts", "job", job.Name, "error", err)
			continue
		}

		a.processJUnitArtifacts(ctx, prov, artifacts, job, req.RequestID, req.BuildURL)
	}

	return nil
}

// Update processJUnitArtifacts to accept provider
func (a *Agent) processJUnitArtifacts(ctx context.Context, prov provider.Provider, artifacts []provider.Artifact, job provider.Job, requestID, buildURL string) {
	for _, artifact := range artifacts {
		if !strings.HasPrefix(artifact.Path, "junit") || !strings.HasSuffix(artifact.Path, ".xml") {
			continue
		}

		data, err := prov.DownloadArtifact(ctx, artifact)
		if err != nil {
			a.logger.Error("failed to download artifact", "path", artifact.Path, "error", err)
			continue
		}

		// Existing JUnit parsing logic
		failures, err := junit.Parse(data)
		if err != nil {
			a.logger.Error("failed to parse JUnit", "path", artifact.Path, "error", err)
			continue
		}

		// Create triage cards (existing logic)
		for _, failure := range failures {
			card := a.createTriageCardFromJUnit(failure, job.Name, buildURL, requestID, artifact.Path)
			if err := a.publishTriageCard(ctx, card); err != nil {
				a.logger.Error("failed to publish triage card", "error", err)
			}
		}
	}
}
```

**Step 3: Update CLI to document GitHub token requirement**

Modify `src/cmd/destill/main.go` to update help text:

```go
// Update analyze command description
var analyzeCmd = &cobra.Command{
	Use:   "analyze <build-url>",
	Short: "Analyze a build locally",
	Long: `Analyze a CI/CD build by fetching logs and running pattern-based analysis locally.

Supports:
  - Buildkite: https://buildkite.com/org/pipeline/builds/123 (requires BUILDKITE_API_TOKEN)
  - GitHub Actions: https://github.com/owner/repo/actions/runs/456 (requires GITHUB_TOKEN)

Results are displayed in an interactive TUI with findings sorted by confidence.`,
	// ... rest of command
}
```

**Step 4: Run tests**

Run: `go test ./src/ingest/... -v`
Expected: PASS (existing tests should still work)

**Step 5: Test manually with Buildkite URL**

Run: `BUILDKITE_API_TOKEN=xxx go run cmd/destill/main.go analyze https://buildkite.com/org/pipeline/builds/123`
Expected: Works as before (no regression)

**Step 6: Commit**

```bash
git add src/ingest/agent.go src/cmd/destill/main.go
git commit -m "refactor: use Provider interface in ingest agent

Replace Buildkite-specific client with provider abstraction to support
multiple CI platforms. Update CLI help text for GitHub Actions support.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 6: Add Progress Indicators to Local Mode

**Files:**
- Modify: `src/cmd/destill/main.go`
- Create: `src/tui/progress.go`

**Step 1: Design progress UI**

We want to show:
- Downloading build metadata...
- Fetching logs (3/5 jobs)...
- Analyzing chunks (127/234)...
- Parsing JUnit artifacts (2/3)...
- Complete! Found 15 findings.

**Step 2: Create progress model**

Create `src/tui/progress.go`:

```go
package tui

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressMsg updates progress display
type ProgressMsg struct {
	Stage   string
	Current int
	Total   int
}

type ProgressModel struct {
	stage   string
	current int
	total   int
	done    bool
}

func NewProgressModel() ProgressModel {
	return ProgressModel{}
}

func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressMsg:
		m.stage = msg.Stage
		m.current = msg.Current
		m.total = msg.Total
		if msg.Stage == "complete" {
			m.done = true
		}
	}
	return m, nil
}

func (m ProgressModel) View() string {
	if m.done {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render("✓ Complete!")
	}

	if m.total > 0 {
		pct := float64(m.current) / float64(m.total) * 100
		return fmt.Sprintf("%s (%d/%d, %.0f%%)", m.stage, m.current, m.total, pct)
	}

	return m.stage + "..."
}
```

**Step 3: Integrate progress into TUI**

Modify `src/tui/triage.go`:

Add progress model to main model and update View() to show progress when cards are loading.

**Step 4: Send progress updates from ingest agent**

Modify `src/ingest/agent.go` to publish progress messages to a dedicated topic or channel.

**Step 5: Test progress display**

Run: `go run cmd/destill/main.go analyze <url>`
Expected: See progress indicators during fetch/analyze

**Step 6: Commit**

```bash
git add src/tui/progress.go src/tui/triage.go src/ingest/agent.go
git commit -m "feat: add progress indicators to local mode

Show real-time progress for fetching, analyzing, and parsing to improve
UX during analysis. Display percentage complete for long operations.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 7: Improve Error Messages

**Files:**
- Modify: `src/cmd/destill/main.go`
- Create: `src/provider/errors.go`

**Step 1: Create user-friendly error types**

Create `src/provider/errors.go`:

```go
package provider

import (
	"errors"
	"fmt"
)

var (
	ErrAuthFailed     = errors.New("authentication failed")
	ErrBuildNotFound  = errors.New("build not found")
	ErrRateLimited    = errors.New("rate limited")
	ErrNetworkTimeout = errors.New("network timeout")
)

// UserError wraps errors with user-friendly messages
type UserError struct {
	Message string
	Hint    string
	Err     error
}

func (e *UserError) Error() string {
	msg := e.Message
	if e.Hint != "" {
		msg += "\n\nHint: " + e.Hint
	}
	if e.Err != nil {
		msg += fmt.Sprintf("\n\nDetails: %v", e.Err)
	}
	return msg
}

func (e *UserError) Unwrap() error {
	return e.Err
}

// WrapError converts API errors to user-friendly messages
func WrapError(err error) error {
	if err == nil {
		return nil
	}

	// Check for common error patterns
	msg := err.Error()

	if errors.Is(err, ErrInvalidURL) {
		return &UserError{
			Message: "Invalid build URL",
			Hint:    "Supported formats:\n  - https://buildkite.com/org/pipeline/builds/123\n  - https://github.com/owner/repo/actions/runs/456",
			Err:     err,
		}
	}

	if msg == "401 Unauthorized" || errors.Is(err, ErrAuthFailed) {
		return &UserError{
			Message: "Authentication failed",
			Hint:    "Check that your API token is valid and has the correct permissions.\n  - Buildkite: Set BUILDKITE_API_TOKEN\n  - GitHub: Set GITHUB_TOKEN",
			Err:     err,
		}
	}

	if msg == "404 Not Found" || errors.Is(err, ErrBuildNotFound) {
		return &UserError{
			Message: "Build not found",
			Hint:    "Check that the build URL is correct and you have access to the repository.",
			Err:     err,
		}
	}

	return err
}
```

**Step 2: Use WrapError in CLI**

Modify `src/cmd/destill/main.go`:

```go
// In analyze command
if err := runAnalyze(cmd.Context(), args[0]); err != nil {
	userErr := provider.WrapError(err)
	fmt.Fprintf(os.Stderr, "Error: %v\n", userErr)
	os.Exit(1)
}
```

**Step 3: Test error scenarios**

Test: Invalid URL, invalid token, build not found, network error
Expected: Clear, helpful error messages with hints

**Step 4: Commit**

```bash
git add src/provider/errors.go src/cmd/destill/main.go
git commit -m "feat: improve error messages with helpful hints

Wrap API errors with user-friendly messages and actionable hints for
common failure scenarios (auth, not found, invalid URL).

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 8: Create MCP Server (Basic)

**Files:**
- Create: `cmd/destill-mcp/main.go`
- Create: `src/mcp/server.go`
- Create: `src/mcp/tools.go`

**Step 1: Research MCP SDK for Go**

Check: Is there an official MCP SDK for Go? If not, implement stdio protocol manually.

**Step 2: Create MCP server entry point**

Create `cmd/destill-mcp/main.go`:

```go
package main

import (
	"context"
	"destill-agent/src/mcp"
	"log"
	"os"
)

func main() {
	server := mcp.NewServer()

	if err := server.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
```

**Step 3: Implement MCP server**

Create `src/mcp/server.go`:

```go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// Server implements MCP stdio protocol
type Server struct {
	tools map[string]Tool
}

// Tool represents an MCP tool
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args map[string]interface{}) (interface{}, error)
}

func NewServer() *Server {
	s := &Server{
		tools: make(map[string]Tool),
	}

	// Register tools
	s.RegisterTool(&AnalyzeBuildTool{})

	return s
}

func (s *Server) RegisterTool(tool Tool) {
	s.tools[tool.Name()] = tool
}

func (s *Server) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	scanner := bufio.NewScanner(stdin)

	for scanner.Scan() {
		line := scanner.Bytes()

		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(stdout, fmt.Sprintf("invalid request: %v", err))
			continue
		}

		resp := s.handleRequest(ctx, &req)

		data, err := json.Marshal(resp)
		if err != nil {
			s.writeError(stdout, fmt.Sprintf("marshal error: %v", err))
			continue
		}

		fmt.Fprintln(stdout, string(data))
	}

	return scanner.Err()
}

func (s *Server) handleRequest(ctx context.Context, req *MCPRequest) *MCPResponse {
	switch req.Method {
	case "tools/list":
		return s.listTools()
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		return &MCPResponse{
			Error: &MCPError{
				Code:    -32601,
				Message: fmt.Sprintf("method not found: %s", req.Method),
			},
		}
	}
}

func (s *Server) listTools() *MCPResponse {
	tools := make([]map[string]interface{}, 0, len(s.tools))

	for _, tool := range s.tools {
		tools = append(tools, map[string]interface{}{
			"name":        tool.Name(),
			"description": tool.Description(),
		})
	}

	return &MCPResponse{
		Result: map[string]interface{}{
			"tools": tools,
		},
	}
}

func (s *Server) callTool(ctx context.Context, req *MCPRequest) *MCPResponse {
	params, ok := req.Params.(map[string]interface{})
	if !ok {
		return &MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: "invalid params",
			},
		}
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return &MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: "missing tool name",
			},
		}
	}

	tool, ok := s.tools[toolName]
	if !ok {
		return &MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: fmt.Sprintf("unknown tool: %s", toolName),
			},
		}
	}

	args, ok := params["arguments"].(map[string]interface{})
	if !ok {
		args = make(map[string]interface{})
	}

	result, err := tool.Execute(ctx, args)
	if err != nil {
		return &MCPResponse{
			Error: &MCPError{
				Code:    -32000,
				Message: err.Error(),
			},
		}
	}

	return &MCPResponse{
		Result: result,
	}
}

func (s *Server) writeError(w io.Writer, msg string) {
	resp := &MCPResponse{
		Error: &MCPError{
			Code:    -32000,
			Message: msg,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Fprintln(w, string(data))
}

// MCP protocol types
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
```

**Step 4: Implement analyze_build tool**

Create `src/mcp/tools.go`:

```go
package mcp

import (
	"context"
	"destill-agent/src/broker"
	"destill-agent/src/ingest"
	"destill-agent/src/analyze"
	"destill-agent/src/provider"
	"fmt"
)

// AnalyzeBuildTool runs local analysis and returns findings
type AnalyzeBuildTool struct{}

func (t *AnalyzeBuildTool) Name() string {
	return "analyze_build"
}

func (t *AnalyzeBuildTool) Description() string {
	return "Analyze a CI/CD build and return findings sorted by confidence. Accepts a build URL (Buildkite or GitHub Actions)."
}

func (t *AnalyzeBuildTool) Execute(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'url' argument")
	}

	// Parse URL
	ref, err := provider.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Get provider
	prov, err := provider.GetProvider(ref)
	if err != nil {
		return nil, fmt.Errorf("provider error: %w", err)
	}

	// Fetch build
	build, err := prov.FetchBuild(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch build error: %w", err)
	}

	// Create in-memory broker
	brk := broker.NewInMemoryBroker()
	defer brk.Close()

	// Run analysis (simplified - would need full ingest/analyze pipeline)
	findings := make([]map[string]interface{}, 0)

	for _, job := range build.Jobs {
		logContent, err := prov.FetchJobLog(ctx, job.ID)
		if err != nil {
			continue
		}

		chunks := ingest.ChunkLog(logContent, job.ID, job.Name, build.URL)

		for _, chunk := range chunks {
			chunkFindings := analyze.AnalyzeChunk(chunk)

			for _, finding := range chunkFindings {
				findings = append(findings, map[string]interface{}{
					"severity":   finding.Severity,
					"message":    finding.RawMessage,
					"confidence": finding.ConfidenceScore,
					"job":        job.Name,
				})
			}
		}
	}

	// Sort by confidence (descending)
	// ... sorting logic ...

	return map[string]interface{}{
		"build_url":      url,
		"findings_count": len(findings),
		"findings":       findings,
	}, nil
}
```

**Step 5: Build MCP binary**

Run: `go build -o bin/destill-mcp cmd/destill-mcp/main.go`
Expected: Binary created successfully

**Step 6: Test MCP server manually**

Test with:
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | bin/destill-mcp
```

Expected: Returns tools list

**Step 7: Commit**

```bash
git add cmd/destill-mcp/ src/mcp/
git commit -m "feat: add MCP server for Claude integration

Implement MCP stdio protocol server with analyze_build tool for
running local analysis from Claude Desktop/Code.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 9: Documentation - README Update

**Files:**
- Modify: `README.md`
- Create: `docs/GITHUB_ACTIONS.md`
- Create: `docs/MCP_INTEGRATION.md`

**Step 1: Update main README**

Modify `README.md`:

```markdown
# Destill - CI/CD Build Failure Analyzer

Destill helps engineers quickly find the root cause of build failures by analyzing logs with pattern-based detection and JUnit parsing.

## Features

- 🔍 **Multi-Platform Support**: Buildkite and GitHub Actions
- ⚡ **Fast Local Analysis**: No infrastructure required
- 🎯 **Smart Confidence Scoring**: JUnit parsing (1.0) + pattern-based detection (0.0-1.0)
- 🤖 **Claude Integration**: MCP server for AI-assisted debugging
- 📊 **Interactive TUI**: Real-time findings sorted by confidence
- 🔧 **Self-Hosted Option**: Optional distributed mode with Redpanda + Postgres

## Quick Start

### Installation

```bash
# macOS (Homebrew)
brew install destill

# Or download binary
curl -L https://github.com/yourusername/destill/releases/latest/download/destill-darwin-amd64 -o destill
chmod +x destill
mv destill /usr/local/bin/
```

### Analyze a Build

**Buildkite:**
```bash
export BUILDKITE_API_TOKEN="your-token"
destill analyze "https://buildkite.com/org/pipeline/builds/123"
```

**GitHub Actions:**
```bash
export GITHUB_TOKEN="your-token"
destill analyze "https://github.com/owner/repo/actions/runs/456"
```

### Claude Integration

See [MCP Integration Guide](docs/MCP_INTEGRATION.md) for setting up Destill with Claude Desktop or Claude Code.

## Documentation

- [GitHub Actions Setup](docs/GITHUB_ACTIONS.md)
- [MCP Integration](docs/MCP_INTEGRATION.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Self-Hosted Mode](docs/SELF_HOSTED.md)

## License

MIT (open source free tier)
```

**Step 2: Create GitHub Actions guide**

Create `docs/GITHUB_ACTIONS.md`:

```markdown
# GitHub Actions Setup

## Authentication

Create a Personal Access Token (PAT) with `repo` scope:

1. Go to GitHub Settings → Developer settings → Personal access tokens
2. Click "Generate new token (classic)"
3. Select scope: `repo` (Full control of private repositories)
4. Copy token and set environment variable:

```bash
export GITHUB_TOKEN="ghp_your_token_here"
```

## Usage

```bash
destill analyze "https://github.com/owner/repo/actions/runs/123456"
```

## Differences from Buildkite

- GitHub logs are zip archives (handled automatically)
- Artifacts are per-run, not per-job
- Job types are not distinguished (all treated as "script")
```

**Step 3: Create MCP integration guide**

Create `docs/MCP_INTEGRATION.md`:

```markdown
# Claude Integration via MCP

Destill provides an MCP server that Claude can use to analyze builds.

## Setup

### 1. Build MCP Server

```bash
go build -o ~/.local/bin/destill-mcp cmd/destill-mcp/main.go
```

### 2. Configure Claude Desktop

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "destill": {
      "command": "/Users/youruser/.local/bin/destill-mcp",
      "env": {
        "BUILDKITE_API_TOKEN": "your-buildkite-token",
        "GITHUB_TOKEN": "your-github-token"
      }
    }
  }
}
```

### 3. Restart Claude Desktop

### 4. Test

In Claude, try:
> "Analyze this build: https://github.com/owner/repo/actions/runs/123"

Claude will call the `analyze_build` tool and return findings.

## Available Tools

- `analyze_build(url: string)` - Analyze a build and return findings sorted by confidence
```

**Step 4: Commit**

```bash
git add README.md docs/GITHUB_ACTIONS.md docs/MCP_INTEGRATION.md
git commit -m "docs: update README and add setup guides

Add GitHub Actions and MCP integration documentation. Update README
with multi-platform support and quick start guide.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Task 10: Integration Testing

**Files:**
- Create: `tests/integration/github_actions_test.go`
- Create: `tests/integration/buildkite_test.go`

**Step 1: Create integration test for GitHub Actions**

Create `tests/integration/github_actions_test.go`:

```go
// +build integration

package integration

import (
	"context"
	"destill-agent/src/provider"
	"os"
	"testing"
)

func TestGitHubActionsIntegration(t *testing.T) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		t.Skip("GITHUB_TOKEN not set, skipping integration test")
	}

	url := os.Getenv("TEST_GITHUB_URL")
	if url == "" {
		t.Skip("TEST_GITHUB_URL not set, skipping integration test")
	}

	ref, err := provider.ParseURL(url)
	if err != nil {
		t.Fatalf("ParseURL failed: %v", err)
	}

	prov, err := provider.GetProvider(ref)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	build, err := prov.FetchBuild(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchBuild failed: %v", err)
	}

	if len(build.Jobs) == 0 {
		t.Error("Expected jobs, got 0")
	}

	t.Logf("Fetched build %s with %d jobs", build.ID, len(build.Jobs))
}
```

**Step 2: Run integration tests**

Run: `go test -tags=integration ./tests/integration/... -v`
Expected: PASS (with real GitHub token and URL)

**Step 3: Commit**

```bash
git add tests/integration/
git commit -m "test: add integration tests for GitHub Actions

Add integration tests for GitHub Actions provider with real API calls.
Requires GITHUB_TOKEN and TEST_GITHUB_URL environment variables.

🤖 Generated with Claude Code

Co-Authored-By: Claude <noreply@anthropic.com>"
```

---

## Summary

This plan implements Phase 1 MVP with:

1. ✅ **Provider Interface** - Abstraction for multi-CI support
2. ✅ **Buildkite Provider** - Refactored to implement interface
3. ✅ **GitHub Actions Client** - New API client for GitHub
4. ✅ **GitHub Actions Provider** - Provider implementation
5. ✅ **Updated Ingest Agent** - Uses provider abstraction
6. ✅ **Progress Indicators** - Better UX during analysis
7. ✅ **Error Messages** - User-friendly with hints
8. ✅ **MCP Server** - Claude integration via stdio protocol
9. ✅ **Documentation** - README, guides, setup instructions
10. ✅ **Integration Tests** - Verify GitHub Actions works end-to-end

**Total Tasks:** 10
**Estimated Time:** 2-3 days (assuming 2-5 min per step, with testing/debugging)

**Next Steps After Phase 1:**
- Gather user feedback on free tier
- Measure adoption metrics
- Plan Phase 2 (Premium SaaS) based on validated demand
