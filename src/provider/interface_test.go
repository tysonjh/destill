package provider

import (
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
