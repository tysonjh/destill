package buildkite

import (
	"testing"
)

func TestParseBuildURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantOrg      string
		wantPipeline string
		wantNumber   int
		wantErr      bool
	}{
		{
			name:         "valid URL",
			url:          "https://buildkite.com/my-org/my-pipeline/builds/4091",
			wantOrg:      "my-org",
			wantPipeline: "my-pipeline",
			wantNumber:   4091,
			wantErr:      false,
		},
		{
			name:         "valid URL with dashes",
			url:          "https://buildkite.com/my-org-name/my-pipeline-name/builds/123",
			wantOrg:      "my-org-name",
			wantPipeline: "my-pipeline-name",
			wantNumber:   123,
			wantErr:      false,
		},
		{
			name:    "invalid URL - missing build number",
			url:     "https://buildkite.com/my-org/my-pipeline/builds/",
			wantErr: true,
		},
		{
			name:    "invalid URL - wrong format",
			url:     "https://example.com/builds/123",
			wantErr: true,
		},
		{
			name:    "invalid URL - non-numeric build number",
			url:     "https://buildkite.com/my-org/my-pipeline/builds/abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, pipeline, number, err := ParseBuildURL(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBuildURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if org != tt.wantOrg {
					t.Errorf("ParseBuildURL() org = %v, want %v", org, tt.wantOrg)
				}
				if pipeline != tt.wantPipeline {
					t.Errorf("ParseBuildURL() pipeline = %v, want %v", pipeline, tt.wantPipeline)
				}
				if number != tt.wantNumber {
					t.Errorf("ParseBuildURL() number = %v, want %v", number, tt.wantNumber)
				}
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	token := "test-api-token"
	client := NewClient(token)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.apiToken != token {
		t.Errorf("NewClient() apiToken = %v, want %v", client.apiToken, token)
	}

	if client.httpClient == nil {
		t.Error("NewClient() httpClient is nil")
	}
}

func TestParsePipelineURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantOrg      string
		wantPipeline string
		wantBranch   string
		wantErr      bool
	}{
		{
			name:         "pipeline URL with branch",
			url:          "https://buildkite.com/redpanda/redpanda/builds?branch=dev",
			wantOrg:      "redpanda",
			wantPipeline: "redpanda",
			wantBranch:   "dev",
			wantErr:      false,
		},
		{
			name:         "pipeline URL without branch",
			url:          "https://buildkite.com/my-org/my-pipeline/builds",
			wantOrg:      "my-org",
			wantPipeline: "my-pipeline",
			wantBranch:   "",
			wantErr:      false,
		},
		{
			name:         "pipeline URL without /builds",
			url:          "https://buildkite.com/my-org/my-pipeline",
			wantOrg:      "my-org",
			wantPipeline: "my-pipeline",
			wantBranch:   "",
			wantErr:      false,
		},
		{
			name:         "pipeline URL with multiple query params",
			url:          "https://buildkite.com/org/pipe/builds?branch=main&page=2",
			wantOrg:      "org",
			wantPipeline: "pipe",
			wantBranch:   "main",
			wantErr:      false,
		},
		{
			name:    "invalid URL - wrong domain",
			url:     "https://example.com/org/pipeline/builds",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, pipeline, branch, err := ParsePipelineURL(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePipelineURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if org != tt.wantOrg {
					t.Errorf("ParsePipelineURL() org = %v, want %v", org, tt.wantOrg)
				}
				if pipeline != tt.wantPipeline {
					t.Errorf("ParsePipelineURL() pipeline = %v, want %v", pipeline, tt.wantPipeline)
				}
				if branch != tt.wantBranch {
					t.Errorf("ParsePipelineURL() branch = %v, want %v", branch, tt.wantBranch)
				}
			}
		})
	}
}
