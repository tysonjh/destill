// Package ranking provides shared tier classification logic for CI/CD findings.
// Both the MCP server and TUI consume this package to ensure consistent
// prioritization of findings.
package ranking

import (
	"sort"

	"destill-agent/src/contracts"
)

// Tier constants for finding classification.
const (
	TierUnique = 1 // Unique failures - only appear in failed jobs
	TierNoise  = 3 // Common noise - appears in both failed and passing jobs
)

// RankedCard wraps a TriageCard with tier and rank information.
type RankedCard struct {
	Card contracts.TriageCard
	Tier int // TierUnique (1) or TierNoise (3)
	Rank int // Position within the flattened list (1-indexed)
}

// TieredCards groups cards by tier, each tier sorted by confidence.
type TieredCards struct {
	Unique []RankedCard // Unique failures (highest signal)
	Noise  []RankedCard // Common noise (lowest signal)
}

// RankCards classifies cards into tiers and returns grouped results.
// Each tier is sorted by confidence (descending), then recurrence (descending).
// Duplicates (same NormalizedMsg) are removed, keeping highest confidence.
func RankCards(cards []contracts.TriageCard) TieredCards {
	if len(cards) == 0 {
		return TieredCards{}
	}

	// Build job state map for cross-job analysis
	jobStates := BuildJobStateMap(cards)

	// Sort by confidence descending, then recurrence descending
	sorted := make([]contracts.TriageCard, len(cards))
	copy(sorted, cards)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ConfidenceScore != sorted[j].ConfidenceScore {
			return sorted[i].ConfidenceScore > sorted[j].ConfidenceScore
		}
		return sorted[i].GetRecurrenceCount() > sorted[j].GetRecurrenceCount()
	})

	// Track seen patterns to deduplicate
	seen := make(map[string]bool)

	var unique, noise []RankedCard

	for _, card := range sorted {
		// Skip duplicates (keep first occurrence = highest confidence)
		if seen[card.NormalizedMsg] {
			continue
		}
		seen[card.NormalizedMsg] = true

		tier := ClassifyTier(card, jobStates)
		ranked := RankedCard{
			Card: card,
			Tier: tier,
		}

		switch tier {
		case TierUnique:
			unique = append(unique, ranked)
		case TierNoise:
			noise = append(noise, ranked)
		}
	}

	return TieredCards{
		Unique: unique,
		Noise:  noise,
	}
}

// FlattenByTier returns all cards sorted by tier (unique first, then noise),
// preserving confidence order within each tier. Assigns global rank (1-indexed).
func (tc TieredCards) FlattenByTier() []RankedCard {
	total := len(tc.Unique) + len(tc.Noise)
	if total == 0 {
		return nil
	}

	result := make([]RankedCard, 0, total)
	result = append(result, tc.Unique...)
	result = append(result, tc.Noise...)

	// Assign global ranks
	for i := range result {
		result[i].Rank = i + 1
	}

	return result
}

// Counts returns the count of unique failures and noise.
func (tc TieredCards) Counts() (unique, noise int) {
	return len(tc.Unique), len(tc.Noise)
}

// ClassifyTier determines which tier a card belongs to.
// Returns TierUnique (unique failures) or TierNoise (common noise).
func ClassifyTier(card contracts.TriageCard, jobStates map[string]string) int {
	state, exists := jobStates[card.NormalizedMsg]
	if !exists {
		// Unknown pattern - treat as unique failure
		return TierUnique
	}

	// Pattern only in failed jobs → unique failure
	if state == "failed" {
		return TierUnique
	}

	// Pattern in both failed and passed → common noise
	return TierNoise
}

// BuildJobStateMap creates a map of normalized_msg -> job state.
// Values are "failed", "passed", or "both".
// Cards with missing or empty job_state are skipped.
func BuildJobStateMap(cards []contracts.TriageCard) map[string]string {
	result := make(map[string]string)

	for _, card := range cards {
		jobState := card.Metadata["job_state"]
		if jobState == "" {
			// Skip cards without job_state to avoid incorrect classification
			continue
		}

		pattern := card.NormalizedMsg

		existing, exists := result[pattern]
		if !exists {
			result[pattern] = jobState
		} else if existing != jobState && existing != "both" {
			result[pattern] = "both"
		}
	}

	return result
}

// CountPassingJobs counts how many unique passing jobs contain this pattern.
func CountPassingJobs(cards []contracts.TriageCard, pattern string) int {
	count := 0
	seenJobs := make(map[string]bool)
	for _, card := range cards {
		if card.NormalizedMsg == pattern && card.Metadata["job_state"] == "passed" {
			if !seenJobs[card.JobName] {
				seenJobs[card.JobName] = true
				count++
			}
		}
	}
	return count
}
