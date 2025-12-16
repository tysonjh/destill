package mcp

import "destill-agent/src/contracts"

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
