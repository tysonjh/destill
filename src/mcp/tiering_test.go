package mcp

import (
	"testing"

	"destill-agent/src/contracts"
)

func TestBuildJobStateMap(t *testing.T) {
	cards := []contracts.TriageCard{
		{NormalizedMsg: "pattern-a", Metadata: map[string]string{"job_state": "failed"}},
		{NormalizedMsg: "pattern-a", Metadata: map[string]string{"job_state": "passed"}},
		{NormalizedMsg: "pattern-b", Metadata: map[string]string{"job_state": "failed"}},
		{NormalizedMsg: "pattern-c", Metadata: map[string]string{"job_state": "passed"}},
	}

	result := buildJobStateMap(cards)

	expected := map[string]string{
		"pattern-a": "both",
		"pattern-b": "failed",
		"pattern-c": "passed",
	}

	for pattern, expectedState := range expected {
		if result[pattern] != expectedState {
			t.Errorf("buildJobStateMap()[%q] = %q, expected %q", pattern, result[pattern], expectedState)
		}
	}
}

func TestClassifyFinding(t *testing.T) {
	tests := []struct {
		name         string
		card         contracts.TriageCard
		jobStates    map[string]string // normalized_msg -> "failed", "passed", or "both"
		expectedTier int
	}{
		{
			name: "unique to failed job",
			card: contracts.TriageCard{
				NormalizedMsg: "error-pattern-1",
				Metadata:      map[string]string{"job_state": "failed"},
			},
			jobStates:    map[string]string{"error-pattern-1": "failed"},
			expectedTier: 1,
		},
		{
			name: "appears in both failed and passed",
			card: contracts.TriageCard{
				NormalizedMsg: "error-pattern-2",
				Metadata:      map[string]string{"job_state": "failed"},
			},
			jobStates:    map[string]string{"error-pattern-2": "both"},
			expectedTier: 3,
		},
		{
			name: "unknown pattern",
			card: contracts.TriageCard{
				NormalizedMsg: "new-pattern",
				Metadata:      map[string]string{"job_state": "failed"},
			},
			jobStates:    map[string]string{},
			expectedTier: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tier := classifyFinding(tt.card, tt.jobStates)
			if tier != tt.expectedTier {
				t.Errorf("classifyFinding() = %d, expected %d", tier, tt.expectedTier)
			}
		})
	}
}

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

	finding := convertToFinding(card, false)

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
