// Package mcp provides the MCP server implementation for LLM-optimized build analysis.
package mcp

import "destill-agent/src/contracts"

// TieredResponse is the MCP tool response structure.
type TieredResponse struct {
	Build                BuildInfo `json:"build"`
	Tier1UniqueFailures  []Finding `json:"tier_1_unique_failures"`
	Tier2FrequencySpikes []Finding `json:"tier_2_frequency_spikes"`
	Tier3CommonNoise     []Finding `json:"tier_3_common_noise"`
}

// BuildInfo contains build metadata.
type BuildInfo struct {
	URL             string   `json:"url"`
	Status          string   `json:"status"`
	FailedJobs      []string `json:"failed_jobs"`
	PassedJobsCount int      `json:"passed_jobs_count"`
	OtherJobsCount  int      `json:"other_jobs_count,omitempty"` // canceled, skipped, in_progress, etc.
	Timestamp       string   `json:"timestamp"`
}

// Finding is a sanitized, LLM-ready error finding.
type Finding struct {
	ID                string   `json:"id"` // MessageHash - stable identifier for drill-down
	Message           string   `json:"message"`
	Severity          string   `json:"severity"`
	Confidence        float64  `json:"confidence"`
	Job               string   `json:"job"`
	JobState          string   `json:"job_state"`
	Recurrence        int      `json:"recurrence"`
	AlsoInPassingJobs bool     `json:"also_in_passing_jobs"`
	PreContext        []string `json:"pre_context"`
	PostContext       []string `json:"post_context"`

	// Tier 2 specific
	RecurrenceThisBuild int `json:"recurrence_this_build,omitempty"`
	AvgRecurrence       int `json:"avg_recurrence,omitempty"`

	// Tier 3 specific
	PassingJobCount int `json:"passing_job_count,omitempty"`
}

// FindingSummary is a lightweight finding for the manifest response.
// Contains just enough info for Claude to decide which findings to drill into.
type FindingSummary struct {
	ID         string  `json:"id"`
	Tier       int     `json:"tier"`
	Message    string  `json:"message"`    // Truncated to ~100 chars
	Severity   string  `json:"severity"`
	Confidence float64 `json:"confidence"`
	Job        string  `json:"job"`
}

// ManifestResponse is the response from analyze_build.
// Tier 1 findings are fully expanded (they're the likely root causes).
// Tier 2-3 findings are summarized for optional drill-down.
type ManifestResponse struct {
	RequestID     string           `json:"request_id"`
	Build         BuildInfo        `json:"build"`
	Tier1Findings []Finding        `json:"tier_1_findings"`
	OtherFindings []FindingSummary `json:"other_findings"`
}

// ExtractRequestID extracts the request_id from triage cards.
func ExtractRequestID(cards []contracts.TriageCard) string {
	if len(cards) > 0 && cards[0].RequestID != "" {
		return cards[0].RequestID
	}
	return ""
}
