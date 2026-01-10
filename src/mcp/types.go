// Package mcp provides the MCP server implementation for LLM-optimized build analysis.
package mcp

import "destill-agent/src/contracts"

// MaxTestExamples is the maximum number of test name examples to include.
const MaxTestExamples = 3

// MaxMessageLength is the maximum length for error messages in summaries.
const MaxMessageLength = 100

// TieredResponse is the MCP tool response structure.
type TieredResponse struct {
	Build                BuildInfo `json:"build"`
	Tier1UniqueFailures  []Finding `json:"tier_1_unique_failures"`
	Tier2FrequencySpikes []Finding `json:"tier_2_frequency_spikes"`
	Tier3CommonNoise     []Finding `json:"tier_3_common_noise"`
}

// BuildInfo contains build metadata.
type BuildInfo struct {
	URL        string `json:"url"`
	Number     string `json:"number,omitempty"`
	Status     string `json:"state"`
	Branch     string `json:"branch,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Message    string `json:"message,omitempty"`
	Source     string `json:"source,omitempty"` // Build trigger: webhook, api, schedule, ui
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Duration   string `json:"duration,omitempty"` // Human-readable duration

	// Job counts and lists
	FailedJobs      []string `json:"failed_jobs,omitempty"`
	FailedWithTests []string `json:"failed_with_tests,omitempty"` // Failed jobs that ran tests
	FailedNoTests   []string `json:"failed_no_tests,omitempty"`   // Failed jobs with no tests (likely infra)
	PassedCount     int      `json:"passed_count"`
	OtherCount      int      `json:"other_count,omitempty"` // canceled, skipped, etc.
	Timestamp       string   `json:"timestamp"`
}

// TestSummary is a token-efficient test summary for MCP responses.
// Contains counts and limited examples instead of full arrays.
type TestSummary struct {
	Total         int      `json:"total"`
	Passed        int      `json:"passed"`
	Failed        int      `json:"failed"`
	NovelCount    int      `json:"novel_count"`              // New failures
	NovelExamples []string `json:"novel_examples,omitempty"` // Up to 3 test names
	FlakyCount    int      `json:"flaky_count"`              // Known flaky tests
	FlakyExamples []string `json:"flaky_examples,omitempty"` // Up to 3 test names
}

// Finding is a sanitized, LLM-ready error finding.
type Finding struct {
	ID         string   `json:"id"` // MessageHash for drill-down
	Message    string   `json:"msg"`
	Severity   string   `json:"sev"`
	Confidence float64  `json:"conf"`
	Job        string   `json:"job"`
	Novel      bool     `json:"novel,omitempty"`      // Never seen before in build history
	InPassing  bool     `json:"in_passing,omitempty"` // Also appears in passing jobs
	Pre        []string `json:"pre,omitempty"`        // Pre-context lines
	Post       []string `json:"post,omitempty"`       // Post-context lines
}

// FindingSummary is a lightweight finding for the manifest response.
// Contains just enough info for Claude to decide which findings to drill into.
type FindingSummary struct {
	ID       string  `json:"id"`
	Tier     int     `json:"tier"`
	Message  string  `json:"msg"` // Truncated to ~100 chars
	Severity string  `json:"sev"`
	Job      string  `json:"job"`
}

// ManifestResponse is the response from analyze_build.
// Tier 1 findings are fully expanded (they're the likely root causes).
// Tier 2-3 findings are summarized for optional drill-down.
type ManifestResponse struct {
	RequestID string        `json:"request_id"`
	Build     BuildInfo     `json:"build"`
	Tests     *TestSummary  `json:"tests,omitempty"`
	Findings  []Finding     `json:"findings"`       // Top tier 1 findings with context
	Other     []FindingSummary `json:"other,omitempty"` // Tier 2-3 summaries
}

// ExtractRequestID extracts the request_id from triage cards.
func ExtractRequestID(cards []contracts.TriageCard) string {
	if len(cards) > 0 && cards[0].RequestID != "" {
		return cards[0].RequestID
	}
	return ""
}
