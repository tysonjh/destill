// Package buildkite provides a client for interacting with the Buildkite API.
package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// APIBaseURL is the base URL for the Buildkite API.
	APIBaseURL = "https://api.buildkite.com/v2"
)

// Client is a Buildkite API client.
type Client struct {
	apiToken   string
	httpClient *http.Client
}

// Build represents a Buildkite build.
type Build struct {
	ID         string    `json:"id"`
	Number     int       `json:"number"`
	State      string    `json:"state"`
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"`
	Message    string    `json:"message"`
	Source     string    `json:"source"`
	WebURL     string    `json:"web_url"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Jobs       []Job     `json:"jobs"`
}

// Job represents a Buildkite job within a build.
type Job struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	State      string    `json:"state"`
	ExitStatus int       `json:"exit_status"`
	CreatedAt  time.Time `json:"created_at"`
	LogURL     string    `json:"log_url"`
	RawLogURL  string    `json:"raw_log_url"`
}

// Artifact represents a build artifact.
type Artifact struct {
	ID          string `json:"id"`
	JobID       string `json:"job_id"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	FileSize    int64  `json:"file_size"`
	Sha1Sum     string `json:"sha1sum"`
}

// NewClient creates a new Buildkite API client.
func NewClient(apiToken string) *Client {
	return &Client{
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ParseBuildURL extracts the organization, pipeline, and build number from a Buildkite URL.
// Expected format: https://buildkite.com/{org}/{pipeline}/builds/{number}
func ParseBuildURL(buildURL string) (org, pipeline string, buildNumber int, err error) {
	// Regex pattern to match Buildkite build URLs
	pattern := `https://buildkite\.com/([^/]+)/([^/]+)/builds/(\d+)`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(buildURL)
	if len(matches) != 4 {
		return "", "", 0, fmt.Errorf("invalid Buildkite URL format: %s", buildURL)
	}

	org = matches[1]
	pipeline = matches[2]
	buildNumber, err = strconv.Atoi(matches[3])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid build number in URL: %w", err)
	}

	return org, pipeline, buildNumber, nil
}

// GetBuild fetches a build's metadata from the Buildkite API.
func (c *Client) GetBuild(ctx context.Context, org, pipeline, buildNumber string) (*Build, error) {
	url := fmt.Sprintf("%s/organizations/%s/pipelines/%s/builds/%s", APIBaseURL, org, pipeline, buildNumber)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var build Build
	if err := json.NewDecoder(resp.Body).Decode(&build); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &build, nil
}

// ListBuildsOptions contains optional filters for listing builds.
type ListBuildsOptions struct {
	Branch string // Filter by branch name (optional)
}

// ListBuilds fetches recent builds for a pipeline.
// Returns up to `limit` builds, ordered by build number descending (newest first).
// If opts is nil or opts.Branch is empty, returns builds from all branches.
func (c *Client) ListBuilds(ctx context.Context, org, pipeline string, limit int, opts *ListBuildsOptions) ([]Build, error) {
	url := fmt.Sprintf("%s/organizations/%s/pipelines/%s/builds?per_page=%d", APIBaseURL, org, pipeline, limit)

	// Add branch filter if specified
	if opts != nil && opts.Branch != "" {
		url += "&branch=" + opts.Branch
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var builds []Build
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return builds, nil
}

// ParsePipelineURL extracts the organization and pipeline from a Buildkite pipeline URL.
// Supports formats:
//   - https://buildkite.com/{org}/{pipeline}/builds?branch=dev
//   - https://buildkite.com/{org}/{pipeline}/builds
//   - https://buildkite.com/{org}/{pipeline}
//
// Returns org, pipeline, and optional branch from query string.
func ParsePipelineURL(pipelineURL string) (org, pipeline, branch string, err error) {
	// Pattern to match pipeline URLs (with or without /builds and query string)
	pattern := `https://buildkite\.com/([^/]+)/([^/?]+)(?:/builds)?(?:\?.*)?$`
	re := regexp.MustCompile(pattern)

	matches := re.FindStringSubmatch(pipelineURL)
	if len(matches) < 3 {
		return "", "", "", fmt.Errorf("invalid Buildkite pipeline URL format: %s", pipelineURL)
	}

	org = matches[1]
	pipeline = matches[2]

	// Extract branch from query string if present
	if idx := strings.Index(pipelineURL, "?"); idx != -1 {
		queryString := pipelineURL[idx+1:]
		for _, param := range strings.Split(queryString, "&") {
			if strings.HasPrefix(param, "branch=") {
				branch = strings.TrimPrefix(param, "branch=")
				break
			}
		}
	}

	return org, pipeline, branch, nil
}

// GetJobLog fetches the raw log content for a specific job.
// Deprecated: Use GetJobLogByURL instead with the raw_log_url from the job metadata.
func (c *Client) GetJobLog(ctx context.Context, jobID string) (string, error) {
	// The job ID from the API response can be used directly
	url := fmt.Sprintf("%s/jobs/%s/log", APIBaseURL, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	logBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read log content: %w", err)
	}

	return string(logBytes), nil
}

// GetJobLogByURL fetches the raw log content using the provided raw_log_url.
// This is the preferred method as it uses the URL provided by the Buildkite API.
func (c *Client) GetJobLogByURL(ctx context.Context, rawLogURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawLogURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Accept", "text/plain")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	logBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read log content: %w", err)
	}

	return string(logBytes), nil
}

// GetJobArtifacts fetches the list of artifacts for a specific job.
func (c *Client) GetJobArtifacts(ctx context.Context, org, pipeline, buildNumber, jobID string) ([]Artifact, error) {
	url := fmt.Sprintf("%s/organizations/%s/pipelines/%s/builds/%s/jobs/%s/artifacts", APIBaseURL, org, pipeline, buildNumber, jobID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// 404 is OK - job might not have artifacts
	if resp.StatusCode == http.StatusNotFound {
		return []Artifact{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var artifacts []Artifact
	if err := json.NewDecoder(resp.Body).Decode(&artifacts); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return artifacts, nil
}

// DownloadArtifact downloads the content of an artifact by its download URL.
// The download endpoint returns a 302 redirect to either:
// - A pre-signed S3 URL (no auth needed)
// - A custom artifact server (may need Basic auth via ARTIFACT_SERVER_USER/PASSWORD)
func (c *Client) DownloadArtifact(ctx context.Context, downloadURL string) ([]byte, error) {
	// Step 1: Get the redirect URL (don't follow automatically)
	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects automatically - we need to handle them manually
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Expect a 302 redirect
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusTemporaryRedirect {
		redirectURL := resp.Header.Get("Location")
		if redirectURL == "" {
			return nil, fmt.Errorf("redirect response missing Location header")
		}

		// Step 2: Follow the redirect - check if it's S3 or custom server
		if isS3URL(redirectURL) {
			// Pre-signed S3 URL - no auth needed
			return c.downloadFromURL(ctx, redirectURL)
		}
		// Custom artifact server - may need Basic auth
		return c.downloadFromCustomServer(ctx, redirectURL)
	}

	// If we got 200 directly (shouldn't happen but handle it)
	if resp.StatusCode == http.StatusOK {
		return io.ReadAll(resp.Body)
	}

	body, _ := io.ReadAll(resp.Body)
	// Add hint about token scopes for 401 errors
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("download failed with status %d (ensure token has 'read_artifacts' scope): %s", resp.StatusCode, string(body))
	}
	return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
}

// isS3URL checks if the URL points to AWS S3
func isS3URL(url string) bool {
	return strings.Contains(url, ".s3.amazonaws.com") ||
		strings.Contains(url, "s3.amazonaws.com") ||
		strings.Contains(url, ".s3-") // e.g., s3-us-west-2.amazonaws.com
}

// downloadFromCustomServer downloads from a custom artifact server.
// Uses Basic auth if ARTIFACT_SERVER_USER and ARTIFACT_SERVER_PASSWORD are set.
func (c *Client) downloadFromCustomServer(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Check for Basic auth credentials
	user := os.Getenv("ARTIFACT_SERVER_USER")
	pass := os.Getenv("ARTIFACT_SERVER_PASSWORD")
	if user != "" && pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if user == "" || pass == "" {
			return nil, fmt.Errorf("artifact server requires authentication - set ARTIFACT_SERVER_USER and ARTIFACT_SERVER_PASSWORD environment variables")
		}
		return nil, fmt.Errorf("artifact server authentication failed (401) - check ARTIFACT_SERVER_USER and ARTIFACT_SERVER_PASSWORD")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// downloadFromURL downloads from a direct URL without authentication.
func (c *Client) downloadFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
