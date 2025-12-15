package githubactions

import (
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
