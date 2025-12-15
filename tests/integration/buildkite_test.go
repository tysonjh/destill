// +build integration

package integration

import (
	"context"
	"destill-agent/src/provider"
	"os"
	"testing"
)

func TestBuildkiteIntegration(t *testing.T) {
	token := os.Getenv("BUILDKITE_API_TOKEN")
	if token == "" {
		t.Skip("BUILDKITE_API_TOKEN not set, skipping integration test")
	}

	url := os.Getenv("TEST_BUILDKITE_URL")
	if url == "" {
		t.Skip("TEST_BUILDKITE_URL not set, skipping integration test")
	}

	ref, err := provider.ParseURL(url)
	if err != nil {
		t.Fatalf("ParseURL failed: %v", err)
	}

	prov, err := provider.GetProvider(ref)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}

	build, err := prov.FetchBuild(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchBuild failed: %v", err)
	}

	if len(build.Jobs) == 0 {
		t.Error("Expected jobs, got 0")
	}

	t.Logf("Fetched build %s with %d jobs", build.ID, len(build.Jobs))
}
