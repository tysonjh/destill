package buildkite

import (
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
