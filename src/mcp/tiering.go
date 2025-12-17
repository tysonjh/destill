package mcp

import (
	"destill-agent/src/contracts"
	"destill-agent/src/ranking"
	"destill-agent/src/sanitize"
)

// Context line limits per tier.
// Tier 1 (unique failures) gets more context for root cause analysis.
// Tier 3 (noise) gets less since it's lower signal.
const (
	Tier1PreContext  = 5
	Tier1PostContext = 10

	Tier3PreContext  = 2
	Tier3PostContext = 3
)

// Default finding limits per tier.
// Tier 1 (unique failures) gets more findings since they're highest signal.
// Tier 3 (noise) is limited to reduce output size - just show top examples.
const (
	DefaultTier1Limit = 15
	DefaultTier3Limit = 3
)

// CardToFinding converts a single TriageCard to a Finding for drill-down.
// Used by get_finding_details when retrieving a single card from the store.
// Aggregate fields (AlsoInPassingJobs, etc.) are not populated as they
// require cross-card analysis. Context is included at full detail (tier 1).
func CardToFinding(card contracts.TriageCard) Finding {
	return Finding{
		ID:          card.MessageHash,
		Message:     sanitize.Clean(card.RawMessage),
		Severity:    card.Severity,
		Confidence:  card.ConfidenceScore,
		Job:         card.JobName,
		JobState:    card.Metadata["job_state"],
		Recurrence:  card.GetRecurrenceCount(),
		PreContext:  sanitize.CleanLines(card.PreContext),
		PostContext: sanitize.CleanLines(card.PostContext),
	}
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
	default: // Tier 3 (noise) or unknown
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
// Note: Build field is not populated here - caller should set it.
// Note: Tier 2 (frequency spikes) is not yet implemented.
func TierFindings(cards []contracts.TriageCard, limit int) TieredResponse {
	// Calculate per-tier limits
	tier1Limit := DefaultTier1Limit
	tier3Limit := DefaultTier3Limit

	// If caller specified a limit, scale proportionally
	if limit > 0 && limit != DefaultTier1Limit {
		tier1Limit = limit
		tier3Limit = max(1, limit/5)
	}

	// Use shared ranking logic
	tiered := ranking.RankCards(cards)
	jobStates := ranking.BuildJobStateMap(cards)

	// Convert ranked cards to Findings with limits
	// Note: Tier2 (frequency spikes) is not yet implemented - always empty
	unique := convertRankedToFindings(tiered.Unique, jobStates, cards, tier1Limit)
	noise := convertRankedToFindings(tiered.Noise, jobStates, cards, tier3Limit)

	return TieredResponse{
		Tier1UniqueFailures:  unique,
		Tier2FrequencySpikes: nil, // Not yet implemented
		Tier3CommonNoise:     noise,
	}
}

// convertRankedToFindings converts RankedCards to Findings with a limit.
func convertRankedToFindings(ranked []ranking.RankedCard, jobStates map[string]string, allCards []contracts.TriageCard, limit int) []Finding {
	var findings []Finding
	for _, rc := range ranked {
		if len(findings) >= limit {
			break
		}
		alsoInPassing := jobStates[rc.Card.NormalizedMsg] == "both"
		finding := convertToFinding(rc.Card, alsoInPassing, rc.Tier)
		if rc.Tier == ranking.TierNoise {
			finding.PassingJobCount = ranking.CountPassingJobs(allCards, rc.Card.NormalizedMsg)
		}
		findings = append(findings, finding)
	}
	return findings
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
