package githubactions

import (
	"testing"

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
