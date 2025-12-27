package contracts

// TopicTestResults is the topic for test result messages.
const TopicTestResults = "destill.test.results"

// TestResult represents a single test execution from JUnit XML.
// Published to: destill.test.results
// Key: {request_id}
type TestResult struct {
	RequestID      string  `json:"request_id"`
	PipelineID     string  `json:"pipeline_id"`     // e.g., "redpanda/redpanda"
	BuildNumber    int     `json:"build_number"`
	BuildURL       string  `json:"build_url"`
	JobName        string  `json:"job_name"`
	TestName       string  `json:"test_name"`       // Full test name (classname.name)
	ClassName      string  `json:"class_name"`      // Test class
	Passed         bool    `json:"passed"`
	FailureMessage string  `json:"failure_message,omitempty"`
	Duration       float64 `json:"duration"`        // Seconds
}

// TestSummary aggregates test results for a build.
type TestSummary struct {
	RequestID     string        `json:"request_id"`
	PipelineID    string        `json:"pipeline_id"`
	BuildNumber   int           `json:"build_number"`
	TotalTests    int           `json:"total_tests"`
	PassedCount   int           `json:"passed_count"`
	FailedCount   int           `json:"failed_count"`
	SkippedCount  int           `json:"skipped_count"`
	NovelFailures []TestFailure `json:"novel_failures,omitempty"`
	FlakyFailures []TestFailure `json:"flaky_failures,omitempty"`
}

// TestFailure represents a failed test with flakiness info.
type TestFailure struct {
	TestName       string `json:"test_name"`
	FailureMessage string `json:"failure_message,omitempty"`
	IsFlaky        bool   `json:"is_flaky"`
	HistoryRuns    int    `json:"history_runs"`       // Total runs in history window
	HistoryFails   int    `json:"history_fails"`      // Failed runs in history window
	LastFailedAt   string `json:"last_failed_at"`     // RFC3339 timestamp of last failure
}
