package mcp

import (
	"testing"

	"destill-agent/src/contracts"
)

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
