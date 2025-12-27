package mcp

import (
	"destill-agent/src/contracts"
	"destill-agent/src/ranking"
	"destill-agent/src/sanitize"
)

// Context line limits per tier.
// Context lines for MCP responses - minimal to reduce token usage.
// Full context is available via get_finding_details.
const (
	MCPPreContext  = 2
	MCPPostContext = 3
)

// Default finding limits per tier.
const (
	DefaultTier1Limit = 5
	DefaultTier3Limit = 3
)

// CardToFinding converts a single TriageCard to a Finding for drill-down.
// Used by get_finding_details when retrieving a single card from the store.
// Full context is included (not truncated like manifest findings).
func CardToFinding(card contracts.TriageCard) Finding {
	return Finding{
		ID:         card.MessageHash,
		Message:    sanitize.Clean(card.RawMessage),
		Severity:   card.Severity,
		Confidence: card.ConfidenceScore,
		Job:        card.JobName,
		Pre:        sanitize.CleanLines(card.PreContext),
		Post:       sanitize.CleanLines(card.PostContext),
	}
}

// convertToFinding converts a TriageCard to an LLM-ready Finding.
// Context is truncated to reduce token usage.
func convertToFinding(card contracts.TriageCard, alsoInPassing bool) Finding {
	// Truncate context for LLM response
	preContext := truncatePreContext(card.PreContext, MCPPreContext)
	postContext := truncateContext(card.PostContext, MCPPostContext)

	return Finding{
		ID:         card.MessageHash,
		Message:    sanitize.Clean(card.RawMessage),
		Severity:   card.Severity,
		Confidence: card.ConfidenceScore,
		Job:        card.JobName,
		InPassing:  alsoInPassing,
		Pre:        sanitize.CleanLines(preContext),
		Post:       sanitize.CleanLines(postContext),
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
	unique := convertRankedToFindings(tiered.Unique, jobStates, tier1Limit)
	noise := convertRankedToFindings(tiered.Noise, jobStates, tier3Limit)

	return TieredResponse{
		Tier1UniqueFailures:  unique,
		Tier2FrequencySpikes: nil, // Not yet implemented
		Tier3CommonNoise:     noise,
	}
}

// convertRankedToFindings converts RankedCards to Findings with a limit.
func convertRankedToFindings(ranked []ranking.RankedCard, jobStates map[string]string, limit int) []Finding {
	var findings []Finding
	for _, rc := range ranked {
		if len(findings) >= limit {
			break
		}
		alsoInPassing := jobStates[rc.Card.NormalizedMsg] == "both"
		findings = append(findings, convertToFinding(rc.Card, alsoInPassing))
	}
	return findings
}

// ToManifest converts a TieredResponse to a ManifestResponse.
// Tier 1 findings are fully expanded with compression applied.
// Tier 2-3 findings are converted to lightweight summaries.
// testSummary is optional - pass nil if no test results available.
func ToManifest(requestID string, response TieredResponse, testSummary *contracts.TestSummary) ManifestResponse {
	// Compress and include tier 1 findings
	findings := make([]Finding, len(response.Tier1UniqueFailures))
	for i, f := range response.Tier1UniqueFailures {
		findings[i] = compressFinding(f)
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
		RequestID: requestID,
		Build:     response.Build,
		Tests:     convertTestSummary(testSummary),
		Findings:  findings,
		Other:     other,
	}
}

// convertTestSummary converts contracts.TestSummary to token-efficient mcp.TestSummary.
func convertTestSummary(ts *contracts.TestSummary) *TestSummary {
	if ts == nil {
		return nil
	}

	result := &TestSummary{
		Total:      ts.TotalTests,
		Passed:     ts.PassedCount,
		Failed:     ts.FailedCount,
		NovelCount: len(ts.NovelFailures),
		FlakyCount: len(ts.FlakyFailures),
	}

	// Add up to MaxTestExamples for each category
	for i, f := range ts.NovelFailures {
		if i >= MaxTestExamples {
			break
		}
		result.NovelExamples = append(result.NovelExamples, f.TestName)
	}
	for i, f := range ts.FlakyFailures {
		if i >= MaxTestExamples {
			break
		}
		result.FlakyExamples = append(result.FlakyExamples, f.TestName)
	}

	return result
}

// compressFinding applies log compression to a Finding.
func compressFinding(f Finding) Finding {
	return Finding{
		ID:         f.ID,
		Message:    CompressLine(f.Message),
		Severity:   f.Severity,
		Confidence: f.Confidence,
		Job:        f.Job,
		InPassing:  f.InPassing,
		Pre:        CompressContextLines(f.Pre),
		Post:       CompressContextLines(f.Post),
	}
}

// toSummary converts a Finding to a FindingSummary.
func toSummary(f Finding, tier int) FindingSummary {
	msg := f.Message
	if len(msg) > MaxMessageLength {
		msg = msg[:MaxMessageLength-3] + "..."
	}
	return FindingSummary{
		ID:       f.ID,
		Tier:     tier,
		Message:  msg,
		Severity: f.Severity,
		Job:      f.Job,
	}
}
