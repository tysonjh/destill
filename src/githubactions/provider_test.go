package githubactions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"destill-agent/src/provider"
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

	// Verify the ref is of the correct type (this ensures provider import is used)
	var _ *provider.BuildRef = ref
}

func TestGitHubProvider_FetchBuild(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("Authorization header = %v, want Bearer test-token", auth)
		}

		// Mock workflow run endpoint
		if r.URL.Path == "/repos/testowner/testrepo/actions/runs/12345" {
			run := WorkflowRun{
				ID:         12345,
				Name:       "CI",
				RunNumber:  42,
				Status:     "completed",
				Conclusion: "success",
				HTMLURL:    "https://github.com/testowner/testrepo/actions/runs/12345",
				CreatedAt:  time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(run)
			return
		}

		// Mock workflow jobs endpoint
		if r.URL.Path == "/repos/testowner/testrepo/actions/runs/12345/jobs" {
			jobs := WorkflowJobsResponse{
				TotalCount: 2,
				Jobs: []WorkflowJob{
					{
						ID:         67890,
						RunID:      12345,
						Name:       "test",
						Status:     "completed",
						Conclusion: "success",
						StartedAt:  time.Date(2024, 1, 1, 12, 5, 0, 0, time.UTC),
					},
					{
						ID:         67891,
						RunID:      12345,
						Name:       "build",
						Status:     "completed",
						Conclusion: "failure",
						StartedAt:  time.Date(2024, 1, 1, 12, 6, 0, 0, time.UTC),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(jobs)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create provider with mock server
	p := NewProvider("test-token")
	p.client.baseURL = server.URL

	// Create build ref
	ref := &provider.BuildRef{
		Provider: "github",
		BuildID:  "12345",
		Metadata: map[string]string{
			"owner": "testowner",
			"repo":  "testrepo",
		},
	}

	// Fetch build
	build, err := p.FetchBuild(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchBuild() error = %v", err)
	}

	// Verify build metadata
	if build.ID != "12345" {
		t.Errorf("Build.ID = %v, want 12345", build.ID)
	}
	if build.Number != "42" {
		t.Errorf("Build.Number = %v, want 42", build.Number)
	}
	if build.State != "passed" {
		t.Errorf("Build.State = %v, want passed", build.State)
	}
	if build.URL != "https://github.com/testowner/testrepo/actions/runs/12345" {
		t.Errorf("Build.URL = %v, want https://github.com/testowner/testrepo/actions/runs/12345", build.URL)
	}

	// Verify jobs
	if len(build.Jobs) != 2 {
		t.Fatalf("len(Build.Jobs) = %v, want 2", len(build.Jobs))
	}

	// Verify first job
	job1 := build.Jobs[0]
	if job1.ID != "testowner/testrepo/67890" {
		t.Errorf("Job[0].ID = %v, want testowner/testrepo/67890", job1.ID)
	}
	if job1.Name != "test" {
		t.Errorf("Job[0].Name = %v, want test", job1.Name)
	}
	if job1.State != "passed" {
		t.Errorf("Job[0].State = %v, want passed", job1.State)
	}
	if job1.ExitCode != 0 {
		t.Errorf("Job[0].ExitCode = %v, want 0", job1.ExitCode)
	}

	// Verify second job
	job2 := build.Jobs[1]
	if job2.ID != "testowner/testrepo/67891" {
		t.Errorf("Job[1].ID = %v, want testowner/testrepo/67891", job2.ID)
	}
	if job2.Name != "build" {
		t.Errorf("Job[1].Name = %v, want build", job2.Name)
	}
	if job2.State != "failed" {
		t.Errorf("Job[1].State = %v, want failed", job2.State)
	}
	if job2.ExitCode != 1 {
		t.Errorf("Job[1].ExitCode = %v, want 1", job2.ExitCode)
	}
}

func TestGitHubProvider_FetchJobLog(t *testing.T) {
	// Create mock server
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock job logs endpoint (returns redirect)
		if r.URL.Path == "/repos/testowner/testrepo/actions/jobs/67890/logs" {
			// Check authorization header for API endpoint
			if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
				t.Errorf("Authorization header = %v, want Bearer test-token", auth)
			}
			// Return redirect to log download URL
			w.Header().Set("Location", serverURL+"/logs/download")
			w.WriteHeader(http.StatusFound)
			return
		}

		// Mock log download endpoint (pre-signed URL, no auth required)
		if r.URL.Path == "/logs/download" {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("2024-01-01T12:00:00.000Z Starting job\n2024-01-01T12:01:00.000Z Job completed\n"))
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()
	serverURL = server.URL

	// Create provider with mock server
	p := NewProvider("test-token")
	p.client.baseURL = server.URL

	// Fetch job log with correct format: owner/repo/jobID
	logs, err := p.FetchJobLog(context.Background(), "testowner/testrepo/67890")
	if err != nil {
		t.Fatalf("FetchJobLog() error = %v", err)
	}

	expectedLogs := "2024-01-01T12:00:00.000Z Starting job\n2024-01-01T12:01:00.000Z Job completed\n"
	if logs != expectedLogs {
		t.Errorf("FetchJobLog() = %v, want %v", logs, expectedLogs)
	}
}

func TestGitHubProvider_FetchJobLog_InvalidFormat(t *testing.T) {
	p := NewProvider("test-token")

	// Test with invalid job ID format (missing owner/repo)
	_, err := p.FetchJobLog(context.Background(), "67890")
	if err == nil {
		t.Error("FetchJobLog() expected error for invalid format, got nil")
	}
}

func TestMapGitHubStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		conclusion string
		want       string
	}{
		{
			name:       "completed success",
			status:     "completed",
			conclusion: "success",
			want:       "passed",
		},
		{
			name:       "completed failure",
			status:     "completed",
			conclusion: "failure",
			want:       "failed",
		},
		{
			name:       "completed cancelled",
			status:     "completed",
			conclusion: "cancelled",
			want:       "canceled",
		},
		{
			name:       "completed skipped",
			status:     "completed",
			conclusion: "skipped",
			want:       "skipped",
		},
		{
			name:       "in progress",
			status:     "in_progress",
			conclusion: "",
			want:       "in_progress",
		},
		{
			name:       "queued",
			status:     "queued",
			conclusion: "",
			want:       "queued",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapGitHubStatus(tt.status, tt.conclusion)
			if got != tt.want {
				t.Errorf("mapGitHubStatus(%v, %v) = %v, want %v", tt.status, tt.conclusion, got, tt.want)
			}
		})
	}
}
