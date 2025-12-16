package mcp

import (
	"testing"
)

func TestInMemoryStore(t *testing.T) {
	store := NewInMemoryStore()

	// Create test findings
	response := TieredResponse{
		Build: BuildInfo{URL: "https://example.com/build/123"},
		Tier1UniqueFailures: []Finding{
			{ID: "hash-1", Message: "error 1", Severity: "ERROR"},
			{ID: "hash-2", Message: "error 2", Severity: "ERROR"},
		},
		Tier3CommonNoise: []Finding{
			{ID: "hash-3", Message: "noise 1", Severity: "WARNING"},
		},
	}

	// Test Store
	store.Store("req-123", response)

	// Test Get - found
	f, found := store.Get("req-123", "hash-1")
	if !found {
		t.Error("Get() expected to find finding hash-1")
	}
	if f.Message != "error 1" {
		t.Errorf("Get() message = %q, want %q", f.Message, "error 1")
	}

	// Test Get - not found (wrong request)
	_, found = store.Get("req-999", "hash-1")
	if found {
		t.Error("Get() expected not to find finding in wrong request")
	}

	// Test Get - not found (wrong hash)
	_, found = store.Get("req-123", "hash-999")
	if found {
		t.Error("Get() expected not to find non-existent hash")
	}

	// Test GetAll
	resp, found := store.GetAll("req-123")
	if !found {
		t.Error("GetAll() expected to find response")
	}
	if len(resp.Tier1UniqueFailures) != 2 {
		t.Errorf("GetAll() tier1 count = %d, want 2", len(resp.Tier1UniqueFailures))
	}

	// Test GetAll - not found
	_, found = store.GetAll("req-999")
	if found {
		t.Error("GetAll() expected not to find non-existent request")
	}
}

func TestToManifest(t *testing.T) {
	response := TieredResponse{
		Build: BuildInfo{
			URL:        "https://example.com/build/123",
			Status:     "failed",
			FailedJobs: []string{"job-1"},
		},
		Tier1UniqueFailures: []Finding{
			{ID: "hash-1", Message: "Short error", Severity: "ERROR", Confidence: 0.95, Job: "job-1"},
			{ID: "hash-2", Message: "This is a very long error message that exceeds one hundred characters and should be truncated in the manifest", Severity: "ERROR", Confidence: 0.85, Job: "job-1"},
		},
		Tier3CommonNoise: []Finding{
			{ID: "hash-3", Message: "Noise", Severity: "WARNING", Confidence: 0.5, Job: "job-2"},
		},
	}

	manifest := ToManifest("req-123", response)

	// Check request ID
	if manifest.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want %q", manifest.RequestID, "req-123")
	}

	// Check build info preserved
	if manifest.Build.Status != "failed" {
		t.Errorf("Build.Status = %q, want %q", manifest.Build.Status, "failed")
	}

	// Check tier 1 findings are fully expanded (not truncated)
	if len(manifest.Tier1Findings) != 2 {
		t.Errorf("Tier1Findings count = %d, want 2", len(manifest.Tier1Findings))
	}
	// Tier 1 messages should NOT be truncated (full Finding objects)
	if manifest.Tier1Findings[0].Message != "Short error" {
		t.Errorf("Tier1 short message = %q, want %q", manifest.Tier1Findings[0].Message, "Short error")
	}
	if len(manifest.Tier1Findings[1].Message) <= 100 {
		t.Errorf("Tier1 long message should not be truncated: len = %d", len(manifest.Tier1Findings[1].Message))
	}

	// Check tier 2-3 findings are summaries (truncated)
	if len(manifest.OtherFindings) != 1 {
		t.Errorf("OtherFindings count = %d, want 1", len(manifest.OtherFindings))
	}
	if manifest.OtherFindings[0].Tier != 3 {
		t.Errorf("OtherFindings tier = %d, want 3", manifest.OtherFindings[0].Tier)
	}
}
