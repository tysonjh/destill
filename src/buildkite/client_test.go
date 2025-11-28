package buildkite

import (
	"testing"
)

func TestParseBuildURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantOrg     string
		wantPipeline string
		wantNumber  int
		wantErr     bool
	}{
		{
			name:        "valid URL",
			url:         "https://buildkite.com/my-org/my-pipeline/builds/4091",
			wantOrg:     "my-org",
			wantPipeline: "my-pipeline",
			wantNumber:  4091,
			wantErr:     false,
		},
		{
			name:        "valid URL with dashes",
			url:         "https://buildkite.com/my-org-name/my-pipeline-name/builds/123",
			wantOrg:     "my-org-name",
			wantPipeline: "my-pipeline-name",
			wantNumber:  123,
			wantErr:     false,
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
