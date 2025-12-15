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

// GetWorkflowJobs fetches jobs for a workflow run (handles pagination)
func (c *Client) GetWorkflowJobs(ctx context.Context, owner, repo, runID string) ([]WorkflowJob, error) {
	var allJobs []WorkflowJob
	page := 1
	perPage := 100 // GitHub's max per page

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/actions/runs/%s/jobs?per_page=%d&page=%d",
			c.baseURL, owner, repo, runID, perPage, page)

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

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API error %d: %s", resp.StatusCode, string(body))
		}

		var jobsResp WorkflowJobsResponse
		if err := json.NewDecoder(resp.Body).Decode(&jobsResp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		allJobs = append(allJobs, jobsResp.Jobs...)

		// Check if we've fetched all jobs
		if len(allJobs) >= jobsResp.TotalCount || len(jobsResp.Jobs) < perPage {
			break
		}

		page++
	}

	return allJobs, nil
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
	// Clone the client settings and set custom CheckRedirect
	client := &http.Client{
		Timeout:   c.httpClient.Timeout,
		Transport: c.httpClient.Transport,
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
		defer rc.Close()

		content, err := io.ReadAll(rc)
		if err != nil {
			return nil, err
		}

		files[file.Name] = content
	}

	return files, nil
}
