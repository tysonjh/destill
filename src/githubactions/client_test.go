package githubactions

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestClient_GetWorkflowRun_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/repos/owner/repo/actions/runs/123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"id": 123,
			"name": "Test Run",
			"status": "completed",
			"conclusion": "success",
			"html_url": "https://github.com/owner/repo/actions/runs/123"
		}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	run, err := client.GetWorkflowRun(context.Background(), "owner", "repo", "123")
	if err != nil {
		t.Fatalf("GetWorkflowRun() error = %v", err)
	}

	if run.ID != 123 {
		t.Errorf("ID = %d, want 123", run.ID)
	}
	if run.Name != "Test Run" {
		t.Errorf("Name = %s, want Test Run", run.Name)
	}
	if run.Status != "completed" {
		t.Errorf("Status = %s, want completed", run.Status)
	}
	if run.Conclusion != "success" {
		t.Errorf("Conclusion = %s, want success", run.Conclusion)
	}
}

func TestClient_GetWorkflowRun_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer server.Close()

	client := NewClient("test-token")
	client.baseURL = server.URL

	_, err := client.GetWorkflowRun(context.Background(), "owner", "repo", "999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify error contains status code
	expectedErr := "GitHub API error 404"
	if len(err.Error()) < len(expectedErr) || err.Error()[:len(expectedErr)] != expectedErr {
		t.Errorf("error = %v, want to start with %s", err, expectedErr)
	}
}

func TestClient_GetWorkflowRun_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "Bad credentials"}`))
	}))
	defer server.Close()

	client := NewClient("invalid-token")
	client.baseURL = server.URL

	_, err := client.GetWorkflowRun(context.Background(), "owner", "repo", "123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify error contains status code
	expectedErr := "GitHub API error 401"
	if len(err.Error()) < len(expectedErr) || err.Error()[:len(expectedErr)] != expectedErr {
		t.Errorf("error = %v, want to start with %s", err, expectedErr)
	}
}
