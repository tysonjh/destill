// Package buildkite provides a client for interacting with the Buildkite API.
package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
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
	ID        string    `json:"id"`
	Number    int       `json:"number"`
	State     string    `json:"state"`
	WebURL    string    `json:"web_url"`
	CreatedAt time.Time `json:"created_at"`
	Jobs      []Job     `json:"jobs"`
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
func (c *Client) GetJobArtifacts(ctx context.Context, jobID string) ([]Artifact, error) {
	// The job ID from the API response can be used directly
	url := fmt.Sprintf("%s/jobs/%s/artifacts", APIBaseURL, jobID)

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
func (c *Client) DownloadArtifact(ctx context.Context, downloadURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiToken))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact content: %w", err)
	}

	return data, nil
}
