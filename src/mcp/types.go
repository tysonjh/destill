// Package mcp provides the MCP server implementation for LLM-optimized build analysis.
package mcp

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
	Timestamp       string   `json:"timestamp"`
}

// Finding is a sanitized, LLM-ready error finding.
type Finding struct {
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
