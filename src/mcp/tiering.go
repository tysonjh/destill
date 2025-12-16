package mcp

import "destill-agent/src/contracts"

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
