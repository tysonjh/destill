package mcp

import (
	"testing"
)

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
