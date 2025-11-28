// Package buildkite provides a client for interacting with the Buildkite API.
package buildkite

import (
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
	ID     string `json:"id"`
	Number int    `json:"number"`
	State  string `json:"state"`
	Jobs   []Job  `json:"jobs"`
}

// Job represents a Buildkite job within a build.
type Job struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	State      string `json:"state"`
	ExitStatus *int   `json:"exit_status"`
	LogURL     string `json:"log_url"`
	RawLogURL  string `json:"raw_log_url"`
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
func (c *Client) GetBuild(org, pipeline string, buildNumber int) (*Build, error) {
	url := fmt.Sprintf("%s/organizations/%s/pipelines/%s/builds/%d", APIBaseURL, org, pipeline, buildNumber)

	req, err := http.NewRequest("GET", url, nil)
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
func (c *Client) GetJobLog(org, pipeline string, buildNumber int, jobID string) (string, error) {
	// Use the jobs endpoint to get the log URL
	url := fmt.Sprintf("%s/organizations/%s/pipelines/%s/builds/%d/jobs/%s/log",
		APIBaseURL, org, pipeline, buildNumber, jobID)

	req, err := http.NewRequest("GET", url, nil)
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
