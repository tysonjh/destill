package buildkite

import (
	"encoding/json"
	"testing"
)

func TestBuildkiteProvider_Name(t *testing.T) {
	p := NewProvider("fake-token")
	if p.Name() != "buildkite" {
		t.Errorf("Name() = %v, want buildkite", p.Name())
	}
}

func TestBuildkiteProvider_ParseURL(t *testing.T) {
	p := NewProvider("fake-token")

	ref, err := p.ParseURL("https://buildkite.com/myorg/mypipeline/builds/123")
	if err != nil {
		t.Fatalf("ParseURL() error = %v", err)
	}

	if ref.Provider != "buildkite" {
		t.Errorf("Provider = %v, want buildkite", ref.Provider)
	}
	if ref.BuildID != "123" {
		t.Errorf("BuildID = %v, want 123", ref.BuildID)
	}
	if ref.Metadata["org"] != "myorg" {
		t.Errorf("org = %v, want myorg", ref.Metadata["org"])
	}
	if ref.Metadata["pipeline"] != "mypipeline" {
		t.Errorf("pipeline = %v, want mypipeline", ref.Metadata["pipeline"])
	}
}

func TestBuildJSONUnmarshal_NumberAsInt(t *testing.T) {
	// Test that Build.Number correctly unmarshals from integer JSON field
	// This is a regression test for the bug where Buildkite API returns number as int
	jsonData := `{
		"id": "test-build-id",
		"number": 77825,
		"state": "failed",
		"web_url": "https://buildkite.com/org/pipeline/builds/77825",
		"created_at": "2024-01-01T00:00:00Z",
		"jobs": []
	}`

	var build Build
	if err := json.Unmarshal([]byte(jsonData), &build); err != nil {
		t.Fatalf("Failed to unmarshal build with integer number: %v", err)
	}

	if build.Number != 77825 {
		t.Errorf("Number = %d, want 77825", build.Number)
	}
}
