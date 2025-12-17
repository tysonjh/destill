package mcp

import (
	"fmt"
	"testing"

	"destill-agent/src/contracts"
)

// Note: Tests for BuildJobStateMap and ClassifyTier are in src/ranking/ranking_test.go

func TestConvertToFinding(t *testing.T) {
	card := contracts.TriageCard{
		RawMessage:      "\x1b[31mERROR\x1b[0m: test failed\r",
		Severity:        "ERROR",
		ConfidenceScore: 0.95,
		JobName:         "test-job",
		PreContext:      []string{"\x1b[32mline1\x1b[0m", "line2"},
		PostContext:     []string{"line3", "\x1b[33mline4\x1b[0m"},
		Metadata: map[string]string{
			"job_state":        "failed",
			"recurrence_count": "5",
		},
	}

	finding := convertToFinding(card, false, 1) // tier 1

	if finding.Message != "ERROR: test failed" {
		t.Errorf("Message = %q, expected %q", finding.Message, "ERROR: test failed")
	}
	if finding.Recurrence != 5 {
		t.Errorf("Recurrence = %d, expected %d", finding.Recurrence, 5)
	}
	if finding.AlsoInPassingJobs != false {
		t.Errorf("AlsoInPassingJobs = %v, expected false", finding.AlsoInPassingJobs)
	}
	if finding.Job != "test-job" {
		t.Errorf("Job = %q, expected %q", finding.Job, "test-job")
	}
	if finding.Severity != "ERROR" {
		t.Errorf("Severity = %q, expected %q", finding.Severity, "ERROR")
	}
	// Check context was sanitized
	if len(finding.PreContext) != 2 || finding.PreContext[0] != "line1" {
		t.Errorf("PreContext not properly sanitized: %v", finding.PreContext)
	}
}

func TestContextTruncation(t *testing.T) {
	// Create card with more context than any tier allows
	preContext := make([]string, 20)
	postContext := make([]string, 20)
	for i := 0; i < 20; i++ {
		preContext[i] = fmt.Sprintf("pre-line-%d", i)
		postContext[i] = fmt.Sprintf("post-line-%d", i)
	}

	card := contracts.TriageCard{
		RawMessage:      "test error",
		Severity:        "ERROR",
		ConfidenceScore: 0.9,
		JobName:         "test-job",
		PreContext:      preContext,
		PostContext:     postContext,
		Metadata:        map[string]string{"job_state": "failed"},
	}

	tests := []struct {
		tier            int
		expectedPreLen  int
		expectedPostLen int
	}{
		{tier: 1, expectedPreLen: Tier1PreContext, expectedPostLen: Tier1PostContext},
		{tier: 3, expectedPreLen: Tier3PreContext, expectedPostLen: Tier3PostContext},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("tier-%d", tt.tier), func(t *testing.T) {
			finding := convertToFinding(card, false, tt.tier)

			if len(finding.PreContext) != tt.expectedPreLen {
				t.Errorf("Tier %d PreContext len = %d, expected %d", tt.tier, len(finding.PreContext), tt.expectedPreLen)
			}
			if len(finding.PostContext) != tt.expectedPostLen {
				t.Errorf("Tier %d PostContext len = %d, expected %d", tt.tier, len(finding.PostContext), tt.expectedPostLen)
			}

			// Pre-context should keep LAST N lines (closest to error)
			if len(finding.PreContext) > 0 {
				expectedFirstPre := fmt.Sprintf("pre-line-%d", 20-tt.expectedPreLen)
				if finding.PreContext[0] != expectedFirstPre {
					t.Errorf("Tier %d PreContext[0] = %q, expected %q (should keep last N lines)", tt.tier, finding.PreContext[0], expectedFirstPre)
				}
			}

			// Post-context should keep FIRST N lines (immediately after error)
			if len(finding.PostContext) > 0 && finding.PostContext[0] != "post-line-0" {
				t.Errorf("Tier %d PostContext[0] = %q, expected %q (should keep first N lines)", tt.tier, finding.PostContext[0], "post-line-0")
			}
		})
	}
}

func TestTierFindings(t *testing.T) {
	cards := []contracts.TriageCard{
		// Unique to failed (tier 1)
		{
			NormalizedMsg:   "unique-error",
			RawMessage:      "unique error message",
			ConfidenceScore: 0.95,
			Severity:        "ERROR",
			JobName:         "job-1",
			Metadata:        map[string]string{"job_state": "failed"},
		},
		// Appears in both (tier 3)
		{
			NormalizedMsg:   "common-error",
			RawMessage:      "common error message",
			ConfidenceScore: 0.85,
			Severity:        "ERROR",
			JobName:         "job-1",
			Metadata:        map[string]string{"job_state": "failed"},
		},
		{
			NormalizedMsg:   "common-error",
			RawMessage:      "common error message",
			ConfidenceScore: 0.6,
			Severity:        "ERROR",
			JobName:         "job-2",
			Metadata:        map[string]string{"job_state": "passed"},
		},
	}

	result := TierFindings(cards, 10)

	if len(result.Tier1UniqueFailures) != 1 {
		t.Errorf("Tier1 count = %d, expected 1", len(result.Tier1UniqueFailures))
	}
	if len(result.Tier3CommonNoise) != 1 {
		t.Errorf("Tier3 count = %d, expected 1", len(result.Tier3CommonNoise))
	}
	// Tier 3 should have passing job count
	if len(result.Tier3CommonNoise) > 0 && result.Tier3CommonNoise[0].PassingJobCount != 1 {
		t.Errorf("PassingJobCount = %d, expected 1", result.Tier3CommonNoise[0].PassingJobCount)
	}
}

func TestToManifest_HybridResponse(t *testing.T) {
	response := TieredResponse{
		Build: BuildInfo{URL: "https://example.com/build/1", Status: "failed"},
		Tier1UniqueFailures: []Finding{
			{
				ID:          "tier1-id",
				Message:     "2024-05-21T10:00:00Z /var/lib/long/path/to/file.go:123 - Error with abc123def456789",
				Severity:    "ERROR",
				Confidence:  0.95,
				Job:         "test-job",
				PreContext:  []string{"2024-05-21T09:59:59Z [INFO] [com.company.module.Class] Pre line"},
				PostContext: []string{"2024-05-21T10:00:01Z [INFO] [com.company.module.Class] Post line"},
			},
		},
		Tier3CommonNoise: []Finding{
			{
				ID:         "tier3-id",
				Message:    "Common warning message that appears everywhere in the logs",
				Severity:   "WARNING",
				Confidence: 0.5,
				Job:        "test-job",
			},
		},
	}

	manifest := ToManifest("req-123", response)

	// Tier 1 should be fully expanded with compression
	if len(manifest.Tier1Findings) != 1 {
		t.Fatalf("Tier1Findings len = %d, expected 1", len(manifest.Tier1Findings))
	}

	tier1 := manifest.Tier1Findings[0]

	// Message should be compressed (timestamp stripped, path shortened, hash masked)
	if tier1.Message == response.Tier1UniqueFailures[0].Message {
		t.Error("Tier1 message should be compressed")
	}
	if tier1.ID != "tier1-id" {
		t.Errorf("Tier1 ID = %q, expected %q", tier1.ID, "tier1-id")
	}

	// Tier 2-3 should be summaries
	if len(manifest.OtherFindings) != 1 {
		t.Fatalf("OtherFindings len = %d, expected 1", len(manifest.OtherFindings))
	}

	other := manifest.OtherFindings[0]
	if other.Tier != 3 {
		t.Errorf("OtherFindings[0].Tier = %d, expected 3", other.Tier)
	}
}
