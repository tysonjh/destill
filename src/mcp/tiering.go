package mcp

import (
	"sort"

	"destill-agent/src/contracts"
	"destill-agent/src/sanitize"
)

// buildJobStateMap creates a map of normalized_msg -> job state.
// Values are "failed", "passed", or "both".
func buildJobStateMap(cards []contracts.TriageCard) map[string]string {
	result := make(map[string]string)

	for _, card := range cards {
		jobState := card.Metadata["job_state"]
		pattern := card.NormalizedMsg

		existing, exists := result[pattern]
		if !exists {
			result[pattern] = jobState
		} else if existing != jobState {
			result[pattern] = "both"
		}
	}

	return result
}

// classifyFinding determines which tier a finding belongs to.
// Returns 1 (unique failures), 2 (frequency spikes), or 3 (common noise).
func classifyFinding(card contracts.TriageCard, jobStates map[string]string) int {
	state, exists := jobStates[card.NormalizedMsg]
	if !exists {
		// Unknown pattern - treat as unique
		return 1
	}

	// If pattern appears only in failed jobs -> Tier 1
	if state == "failed" {
		return 1
	}

	// If pattern appears in both -> Tier 3 (common noise)
	// Tier 2 (frequency spikes) requires historical data - implement later
	return 3
}

// convertToFinding converts a TriageCard to an LLM-ready Finding.
func convertToFinding(card contracts.TriageCard, alsoInPassing bool) Finding {
	return Finding{
		Message:           sanitize.Clean(card.RawMessage),
		Severity:          card.Severity,
		Confidence:        card.ConfidenceScore,
		Job:               card.JobName,
		JobState:          card.Metadata["job_state"],
		Recurrence:        card.GetRecurrenceCount(),
		AlsoInPassingJobs: alsoInPassing,
		PreContext:        sanitize.CleanLines(card.PreContext),
		PostContext:       sanitize.CleanLines(card.PostContext),
	}
}

// TierFindings groups cards into tiers and returns a TieredResponse.
// limit specifies max findings per tier.
func TierFindings(cards []contracts.TriageCard, limit int) TieredResponse {
	// Build job state map for cross-job analysis
	jobStates := buildJobStateMap(cards)

	// Track which normalized messages we've already included
	seen := make(map[string]bool)

	var tier1, tier2, tier3 []Finding

	// Sort by confidence descending
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].ConfidenceScore > cards[j].ConfidenceScore
	})

	for _, card := range cards {
		// Skip duplicates (keep first occurrence = highest confidence)
		if seen[card.NormalizedMsg] {
			continue
		}
		seen[card.NormalizedMsg] = true

		tier := classifyFinding(card, jobStates)
		alsoInPassing := jobStates[card.NormalizedMsg] == "both"
		finding := convertToFinding(card, alsoInPassing)

		switch tier {
		case 1:
			if len(tier1) < limit {
				tier1 = append(tier1, finding)
			}
		case 2:
			if len(tier2) < limit {
				tier2 = append(tier2, finding)
			}
		case 3:
			if len(tier3) < limit {
				finding.PassingJobCount = countPassingJobs(cards, card.NormalizedMsg)
				tier3 = append(tier3, finding)
			}
		}
	}

	return TieredResponse{
		Tier1UniqueFailures:  tier1,
		Tier2FrequencySpikes: tier2,
		Tier3CommonNoise:     tier3,
	}
}

// countPassingJobs counts how many passing jobs have this pattern.
func countPassingJobs(cards []contracts.TriageCard, pattern string) int {
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
