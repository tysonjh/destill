package ingest

import (
	"context"
	"testing"

	"destill-agent/src/provider"
)

// mockProvider implements provider.Provider for testing
type mockProvider struct {
	artifacts    []provider.Artifact
	artifactData map[string][]byte
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) ParseURL(url string) (*provider.BuildRef, error) {
	return &provider.BuildRef{Provider: "mock", BuildID: "100"}, nil
}

func (m *mockProvider) FetchBuild(ctx context.Context, ref *provider.BuildRef) (*provider.Build, error) {
	return &provider.Build{
		ID:     "build-1",
		Number: "100",
		Jobs: []provider.Job{
			{ID: "job-1", Name: "test-job", State: "failed"},
		},
	}, nil
}

func (m *mockProvider) FetchJobLog(ctx context.Context, jobID string) (string, error) {
	return "", nil
}

func (m *mockProvider) FetchArtifacts(ctx context.Context, jobID string) ([]provider.Artifact, error) {
	return m.artifacts, nil
}

func (m *mockProvider) DownloadArtifact(ctx context.Context, artifact provider.Artifact) ([]byte, error) {
	return m.artifactData[artifact.Path], nil
}

func TestProcessArtifactsForBuild_ParsesJUnit(t *testing.T) {
	// Sample JUnit XML
	junitXML := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="MyTests" tests="3" failures="1" errors="0">
  <testcase name="test_passing_1" classname="tests.MyTests" time="0.5"/>
  <testcase name="test_passing_2" classname="tests.MyTests" time="0.3"/>
  <testcase name="test_failing" classname="tests.MyTests" time="1.2">
    <failure message="AssertionError">Expected true but got false</failure>
  </testcase>
</testsuite>`

	// Create mock provider
	mockProv := &mockProvider{
		artifacts: []provider.Artifact{
			{ID: "art-1", Path: "results/junit.xml", DownloadURL: "http://example.com/junit.xml"},
		},
		artifactData: map[string][]byte{
			"results/junit.xml": []byte(junitXML),
		},
	}

	ctx := context.Background()

	ref := &provider.BuildRef{
		Provider: "mock",
		BuildID:  "100",
		Metadata: map[string]string{"org": "test", "pipeline": "test"},
	}

	build := &provider.Build{
		ID:     "build-1",
		Number: "100",
		Jobs: []provider.Job{
			{ID: "job-1", Name: "test-job", State: "failed"},
		},
	}

	// Process artifacts
	results, err := ProcessArtifactsForBuild(ctx, mockProv, ref, build, nil, "https://example.com/builds/100")
	if err != nil {
		t.Fatalf("ProcessArtifactsForBuild failed: %v", err)
	}

	// Verify results
	if len(results) != 3 {
		t.Errorf("Expected 3 test results, got %d", len(results))
	}

	// Count passed/failed
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	if passed != 2 {
		t.Errorf("Expected 2 passed, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("Expected 1 failed, got %d", failed)
	}
}

func TestParseJUnitXML_ValidXML(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="MyTests" tests="2" failures="1">
  <testcase name="test_pass" classname="tests.Example" time="0.1"/>
  <testcase name="test_fail" classname="tests.Example" time="0.2">
    <failure message="Oops">Stack trace here</failure>
  </testcase>
</testsuite>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("ParseJUnitXML failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	// First should pass
	if !results[0].Passed {
		t.Error("Expected first test to pass")
	}
	if results[0].TestName != "tests.Example.test_pass" {
		t.Errorf("Expected test name 'tests.Example.test_pass', got '%s'", results[0].TestName)
	}

	// Second should fail
	if results[1].Passed {
		t.Error("Expected second test to fail")
	}
	if results[1].FailureMessage == "" {
		t.Error("Expected failure message")
	}
}
