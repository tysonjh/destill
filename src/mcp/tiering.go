package mcp

import (
	"sort"

	"destill-agent/src/contracts"
	"destill-agent/src/sanitize"
)

// Context line limits per tier.
// Tier 1 (unique failures) gets more context for root cause analysis.
// Tier 2/3 get progressively less since they're lower signal.
const (
	Tier1PreContext  = 5
	Tier1PostContext = 10

	Tier2PreContext  = 3
	Tier2PostContext = 5

	Tier3PreContext  = 2
	Tier3PostContext = 3
)

// Default finding limits per tier.
// Tier 1 gets more findings since they're highest signal.
// Tier 2/3 are limited to reduce output size - just show top examples.
const (
	DefaultTier1Limit = 15
	DefaultTier2Limit = 5
	DefaultTier3Limit = 3
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
// Context is truncated based on tier to reduce token usage.
func convertToFinding(card contracts.TriageCard, alsoInPassing bool, tier int) Finding {
	// Get tier-specific context limits
	preLimit, postLimit := getContextLimits(tier)

	// Truncate context to tier-specific limits
	// Pre-context: keep last N lines (closest to the error)
	// Post-context: keep first N lines (immediately after error)
	preContext := truncatePreContext(card.PreContext, preLimit)
	postContext := truncateContext(card.PostContext, postLimit)

	return Finding{
		ID:                card.MessageHash, // Stable identifier for drill-down
		Message:           sanitize.Clean(card.RawMessage),
		Severity:          card.Severity,
		Confidence:        card.ConfidenceScore,
		Job:               card.JobName,
		JobState:          card.Metadata["job_state"],
		Recurrence:        card.GetRecurrenceCount(),
		AlsoInPassingJobs: alsoInPassing,
		PreContext:        sanitize.CleanLines(preContext),
		PostContext:       sanitize.CleanLines(postContext),
	}
}

// getContextLimits returns pre/post context line limits for a tier.
func getContextLimits(tier int) (pre, post int) {
	switch tier {
	case 1:
		return Tier1PreContext, Tier1PostContext
	case 2:
		return Tier2PreContext, Tier2PostContext
	default:
		return Tier3PreContext, Tier3PostContext
	}
}

// truncateContext truncates a slice to at most limit elements.
// For pre-context, keeps the last N lines (closest to the error).
// For post-context, keeps the first N lines (immediately after error).
func truncateContext(lines []string, limit int) []string {
	if len(lines) <= limit {
		return lines
	}
	return lines[:limit]
}

// truncatePreContext keeps the last N lines of pre-context (closest to error).
func truncatePreContext(lines []string, limit int) []string {
	if len(lines) <= limit {
		return lines
	}
	return lines[len(lines)-limit:]
}

// TierFindings groups cards into tiers and returns a TieredResponse.
// limit specifies max findings for tier 1 (must be > 0). Tier 2/3 use
// proportionally smaller limits to reduce output size.
//
// Note: This function sorts the input slice by confidence (descending).
// Note: Build field is not populated here - caller should set it.
// Note: Tier 2 (frequency spikes) is not yet implemented.
func TierFindings(cards []contracts.TriageCard, limit int) TieredResponse {
	// Calculate per-tier limits
	tier1Limit := DefaultTier1Limit
	tier2Limit := DefaultTier2Limit
	tier3Limit := DefaultTier3Limit

	// If caller specified a limit, scale proportionally
	if limit > 0 && limit != DefaultTier1Limit {
		tier1Limit = limit
		tier2Limit = max(1, limit/3)
		tier3Limit = max(1, limit/5)
	}

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
		finding := convertToFinding(card, alsoInPassing, tier)

		switch tier {
		case 1:
			if len(tier1) < tier1Limit {
				tier1 = append(tier1, finding)
			}
		case 2:
			if len(tier2) < tier2Limit {
				tier2 = append(tier2, finding)
			}
		case 3:
			if len(tier3) < tier3Limit {
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

// ToManifest converts a TieredResponse to a ManifestResponse.
// Tier 1 findings are fully expanded with compression applied.
// Tier 2-3 findings are converted to lightweight summaries.
func ToManifest(requestID string, response TieredResponse) ManifestResponse {
	// Compress and include full tier 1 findings
	tier1 := make([]Finding, len(response.Tier1UniqueFailures))
	for i, f := range response.Tier1UniqueFailures {
		tier1[i] = compressFinding(f)
	}

	// Convert tier 2-3 to summaries
	var other []FindingSummary
	for _, f := range response.Tier2FrequencySpikes {
		other = append(other, toSummary(f, 2))
	}
	for _, f := range response.Tier3CommonNoise {
		other = append(other, toSummary(f, 3))
	}

	return ManifestResponse{
		RequestID:     requestID,
		Build:         response.Build,
		Tier1Findings: tier1,
		OtherFindings: other,
	}
}

// compressFinding applies log compression to a Finding.
func compressFinding(f Finding) Finding {
	return Finding{
		ID:                f.ID,
		Message:           CompressLine(f.Message),
		Severity:          f.Severity,
		Confidence:        f.Confidence,
		Job:               f.Job,
		JobState:          f.JobState,
		Recurrence:        f.Recurrence,
		AlsoInPassingJobs: f.AlsoInPassingJobs,
		PreContext:        CompressContextLines(f.PreContext),
		PostContext:       CompressContextLines(f.PostContext),
	}
}

// toSummary converts a Finding to a FindingSummary.
func toSummary(f Finding, tier int) FindingSummary {
	msg := f.Message
	if len(msg) > 100 {
		msg = msg[:97] + "..."
	}
	return FindingSummary{
		ID:         f.ID,
		Tier:       tier,
		Message:    msg,
		Severity:   f.Severity,
		Confidence: f.Confidence,
		Job:        f.Job,
	}
}
