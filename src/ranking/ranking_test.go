package ranking

import (
	"testing"

	"destill-agent/src/contracts"
)

func TestBuildJobStateMap(t *testing.T) {
	tests := []struct {
		name  string
		cards []contracts.TriageCard
		want  map[string]string
	}{
		{
			name:  "empty cards",
			cards: []contracts.TriageCard{},
			want:  map[string]string{},
		},
		{
			name: "single failed job",
			cards: []contracts.TriageCard{
				{NormalizedMsg: "error-1", Metadata: map[string]string{"job_state": "failed"}},
			},
			want: map[string]string{"error-1": "failed"},
		},
		{
			name: "same pattern in failed and passed",
			cards: []contracts.TriageCard{
				{NormalizedMsg: "error-1", Metadata: map[string]string{"job_state": "failed"}},
				{NormalizedMsg: "error-1", Metadata: map[string]string{"job_state": "passed"}},
			},
			want: map[string]string{"error-1": "both"},
		},
		{
			name: "skip empty job_state",
			cards: []contracts.TriageCard{
				{NormalizedMsg: "error-1", Metadata: map[string]string{"job_state": "failed"}},
				{NormalizedMsg: "error-2", Metadata: map[string]string{}},
			},
			want: map[string]string{"error-1": "failed"},
		},
		{
			name: "multiple patterns",
			cards: []contracts.TriageCard{
				{NormalizedMsg: "error-1", Metadata: map[string]string{"job_state": "failed"}},
				{NormalizedMsg: "error-2", Metadata: map[string]string{"job_state": "passed"}},
				{NormalizedMsg: "error-3", Metadata: map[string]string{"job_state": "failed"}},
				{NormalizedMsg: "error-3", Metadata: map[string]string{"job_state": "passed"}},
			},
			want: map[string]string{
				"error-1": "failed",
				"error-2": "passed",
				"error-3": "both",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildJobStateMap(tt.cards)
			if len(got) != len(tt.want) {
				t.Errorf("BuildJobStateMap() returned %d entries, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("BuildJobStateMap()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestClassifyTier(t *testing.T) {
	jobStates := map[string]string{
		"error-failed": "failed",
		"error-passed": "passed",
		"error-both":   "both",
	}

	tests := []struct {
		name     string
		pattern  string
		wantTier int
	}{
		{"failed only → unique", "error-failed", TierUnique},
		{"passed only → noise", "error-passed", TierNoise},
		{"both → noise", "error-both", TierNoise},
		{"unknown → unique", "error-unknown", TierUnique},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := contracts.TriageCard{NormalizedMsg: tt.pattern}
			got := ClassifyTier(card, jobStates)
			if got != tt.wantTier {
				t.Errorf("ClassifyTier() = %d, want %d", got, tt.wantTier)
			}
		})
	}
}

func TestRankCards(t *testing.T) {
	cards := []contracts.TriageCard{
		{
			NormalizedMsg:   "error-unique",
			ConfidenceScore: 0.9,
			Metadata:        map[string]string{"job_state": "failed", "recurrence_count": "1"},
		},
		{
			NormalizedMsg:   "error-noise",
			ConfidenceScore: 0.8,
			Metadata:        map[string]string{"job_state": "failed", "recurrence_count": "1"},
		},
		{
			NormalizedMsg:   "error-noise",
			ConfidenceScore: 0.7,
			Metadata:        map[string]string{"job_state": "passed", "recurrence_count": "1"},
		},
	}

	tiered := RankCards(cards)

	// Verify unique has unique error
	if len(tiered.Unique) != 1 {
		t.Errorf("Unique count = %d, want 1", len(tiered.Unique))
	}
	if tiered.Unique[0].Card.NormalizedMsg != "error-unique" {
		t.Errorf("Unique[0] = %q, want %q", tiered.Unique[0].Card.NormalizedMsg, "error-unique")
	}

	// Verify noise has noise (deduplicated)
	if len(tiered.Noise) != 1 {
		t.Errorf("Noise count = %d, want 1", len(tiered.Noise))
	}
	if tiered.Noise[0].Card.NormalizedMsg != "error-noise" {
		t.Errorf("Noise[0] = %q, want %q", tiered.Noise[0].Card.NormalizedMsg, "error-noise")
	}
}

func TestRankCards_SortByConfidence(t *testing.T) {
	cards := []contracts.TriageCard{
		{NormalizedMsg: "low", ConfidenceScore: 0.5, Metadata: map[string]string{"job_state": "failed"}},
		{NormalizedMsg: "high", ConfidenceScore: 0.9, Metadata: map[string]string{"job_state": "failed"}},
		{NormalizedMsg: "mid", ConfidenceScore: 0.7, Metadata: map[string]string{"job_state": "failed"}},
	}

	tiered := RankCards(cards)

	if len(tiered.Unique) != 3 {
		t.Fatalf("Unique count = %d, want 3", len(tiered.Unique))
	}

	// Should be sorted: high, mid, low
	expected := []string{"high", "mid", "low"}
	for i, exp := range expected {
		if tiered.Unique[i].Card.NormalizedMsg != exp {
			t.Errorf("Unique[%d] = %q, want %q", i, tiered.Unique[i].Card.NormalizedMsg, exp)
		}
	}
}

func TestTieredCards_FlattenByTier(t *testing.T) {
	tiered := TieredCards{
		Unique: []RankedCard{
			{Card: contracts.TriageCard{NormalizedMsg: "unique-a"}, Tier: TierUnique},
			{Card: contracts.TriageCard{NormalizedMsg: "unique-b"}, Tier: TierUnique},
		},
		Noise: []RankedCard{
			{Card: contracts.TriageCard{NormalizedMsg: "noise-a"}, Tier: TierNoise},
		},
	}

	flat := tiered.FlattenByTier()

	if len(flat) != 3 {
		t.Fatalf("FlattenByTier() returned %d items, want 3", len(flat))
	}

	// Check order: unique first, then noise
	expected := []string{"unique-a", "unique-b", "noise-a"}
	for i, exp := range expected {
		if flat[i].Card.NormalizedMsg != exp {
			t.Errorf("flat[%d] = %q, want %q", i, flat[i].Card.NormalizedMsg, exp)
		}
		// Check rank assignment
		if flat[i].Rank != i+1 {
			t.Errorf("flat[%d].Rank = %d, want %d", i, flat[i].Rank, i+1)
		}
	}
}

func TestTieredCards_Counts(t *testing.T) {
	tiered := TieredCards{
		Unique: make([]RankedCard, 3),
		Noise:  make([]RankedCard, 5),
	}

	unique, noise := tiered.Counts()
	if unique != 3 || noise != 5 {
		t.Errorf("Counts() = (%d, %d), want (3, 5)", unique, noise)
	}
}

func TestCountPassingJobs(t *testing.T) {
	cards := []contracts.TriageCard{
		{NormalizedMsg: "error-1", JobName: "job-a", Metadata: map[string]string{"job_state": "passed"}},
		{NormalizedMsg: "error-1", JobName: "job-b", Metadata: map[string]string{"job_state": "passed"}},
		{NormalizedMsg: "error-1", JobName: "job-a", Metadata: map[string]string{"job_state": "passed"}}, // duplicate
		{NormalizedMsg: "error-1", JobName: "job-c", Metadata: map[string]string{"job_state": "failed"}},
		{NormalizedMsg: "error-2", JobName: "job-d", Metadata: map[string]string{"job_state": "passed"}},
	}

	count := CountPassingJobs(cards, "error-1")
	if count != 2 { // job-a and job-b (deduplicated)
		t.Errorf("CountPassingJobs() = %d, want 2", count)
	}
}

func TestRankCards_Empty(t *testing.T) {
	tiered := RankCards(nil)
	if len(tiered.Unique) != 0 || len(tiered.Noise) != 0 {
		t.Error("RankCards(nil) should return empty tiers")
	}

	flat := tiered.FlattenByTier()
	if flat != nil {
		t.Error("FlattenByTier() on empty should return nil")
	}
}
