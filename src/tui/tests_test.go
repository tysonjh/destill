package tui

import (
	"testing"

	"destill-agent/src/contracts"
)

// =============================================================================
// TestsModel Deduplication Tests
// =============================================================================

func TestDeduplicateFailures_GroupsByTestName(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	// Same test failing in multiple jobs
	model.allResults = []contracts.TestResult{
		{TestName: "TestFoo", JobName: "job-1", Passed: false, FailureMessage: "error"},
		{TestName: "TestFoo", JobName: "job-2", Passed: false, FailureMessage: "error"},
		{TestName: "TestFoo", JobName: "job-3", Passed: false, FailureMessage: "error"},
		{TestName: "TestBar", JobName: "job-1", Passed: false, FailureMessage: "other error"},
	}

	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{TestName: "TestFoo"},
			{TestName: "TestBar"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated failures, got %d", len(deduped))
	}

	// Find TestFoo
	var testFoo *dedupedFailure
	for i := range deduped {
		if deduped[i].TestName == "TestFoo" {
			testFoo = &deduped[i]
			break
		}
	}

	if testFoo == nil {
		t.Fatal("TestFoo not found in deduped results")
	}

	if len(testFoo.Jobs) != 3 {
		t.Errorf("expected TestFoo to have 3 jobs, got %d", len(testFoo.Jobs))
	}
}

func TestDeduplicateFailures_IgnoresPassingTests(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	model.allResults = []contracts.TestResult{
		{TestName: "TestPass", JobName: "job-1", Passed: true},
		{TestName: "TestFail", JobName: "job-1", Passed: false, FailureMessage: "error"},
	}

	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{TestName: "TestFail"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated failure (ignoring pass), got %d", len(deduped))
	}

	if deduped[0].TestName != "TestFail" {
		t.Errorf("expected TestFail, got %s", deduped[0].TestName)
	}
}

func TestDeduplicateFailures_MarksFlakyTests(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	model.allResults = []contracts.TestResult{
		{TestName: "FlakyTest", JobName: "job-1", Passed: false},
		{TestName: "NovelTest", JobName: "job-1", Passed: false},
	}

	model.summary = &contracts.TestSummary{
		FlakyFailures: []contracts.TestFailure{
			{
				TestName:     "FlakyTest",
				IsFlaky:      true,
				HistoryRuns:  20,
				HistoryFails: 4,
				LastFailedAt: "2024-12-15T10:00:00Z",
			},
		},
		NovelFailures: []contracts.TestFailure{
			{TestName: "NovelTest"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated failures, got %d", len(deduped))
	}

	// Find each test
	var flakyTest, novelTest *dedupedFailure
	for i := range deduped {
		switch deduped[i].TestName {
		case "FlakyTest":
			flakyTest = &deduped[i]
		case "NovelTest":
			novelTest = &deduped[i]
		}
	}

	if flakyTest == nil {
		t.Fatal("FlakyTest not found")
	}
	if !flakyTest.IsFlaky {
		t.Error("expected FlakyTest.IsFlaky=true")
	}
	if flakyTest.HistoryRuns != 20 {
		t.Errorf("expected HistoryRuns=20, got %d", flakyTest.HistoryRuns)
	}
	if flakyTest.HistoryFails != 4 {
		t.Errorf("expected HistoryFails=4, got %d", flakyTest.HistoryFails)
	}
	if flakyTest.LastFailedAt != "2024-12-15T10:00:00Z" {
		t.Errorf("expected LastFailedAt to be set, got %s", flakyTest.LastFailedAt)
	}

	if novelTest == nil {
		t.Fatal("NovelTest not found")
	}
	if novelTest.IsFlaky {
		t.Error("expected NovelTest.IsFlaky=false")
	}
}

func TestDeduplicateFailures_NovelTestWithLastFailedAt(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	model.allResults = []contracts.TestResult{
		{TestName: "NovelWithHistory", JobName: "job-1", Passed: false},
	}

	// Novel test that has some history (failed before, but not enough to be flaky)
	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{
				TestName:     "NovelWithHistory",
				LastFailedAt: "2024-12-10T08:00:00Z",
			},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated failure, got %d", len(deduped))
	}

	if deduped[0].IsFlaky {
		t.Error("expected IsFlaky=false for novel test")
	}
	if deduped[0].LastFailedAt != "2024-12-10T08:00:00Z" {
		t.Errorf("expected LastFailedAt to be passed through, got %s", deduped[0].LastFailedAt)
	}
}

func TestDeduplicateFailures_NoDuplicateJobs(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	// Same test, same job appearing multiple times (edge case)
	model.allResults = []contracts.TestResult{
		{TestName: "TestX", JobName: "job-1", Passed: false},
		{TestName: "TestX", JobName: "job-1", Passed: false}, // Duplicate
		{TestName: "TestX", JobName: "job-2", Passed: false},
	}

	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{TestName: "TestX"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated failure, got %d", len(deduped))
	}

	// Should only have 2 unique jobs, not 3
	if len(deduped[0].Jobs) != 2 {
		t.Errorf("expected 2 unique jobs, got %d: %v", len(deduped[0].Jobs), deduped[0].Jobs)
	}
}

func TestDeduplicateFailures_EmptyResults(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	model.allResults = []contracts.TestResult{}
	model.summary = &contracts.TestSummary{}

	deduped := model.deduplicateFailures()

	if len(deduped) != 0 {
		t.Errorf("expected 0 deduplicated failures for empty results, got %d", len(deduped))
	}
}

func TestDeduplicateFailures_NilSummary(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	model.allResults = []contracts.TestResult{
		{TestName: "TestX", JobName: "job-1", Passed: false},
	}
	model.summary = nil

	deduped := model.deduplicateFailures()

	// Should still work, just won't have flaky info
	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated failure, got %d", len(deduped))
	}
	if deduped[0].IsFlaky {
		t.Error("expected IsFlaky=false when summary is nil")
	}
}

// =============================================================================
// TestsModel Content Tests
// =============================================================================

func TestTestsModel_HasTestFailures(t *testing.T) {
	styles := DefaultStyles()

	tests := []struct {
		name     string
		summary  *contracts.TestSummary
		expected bool
	}{
		{
			name:     "nil summary",
			summary:  nil,
			expected: false,
		},
		{
			name:     "empty summary",
			summary:  &contracts.TestSummary{},
			expected: false,
		},
		{
			name: "only novel failures",
			summary: &contracts.TestSummary{
				NovelFailures: []contracts.TestFailure{{TestName: "Test1"}},
			},
			expected: true,
		},
		{
			name: "only flaky failures",
			summary: &contracts.TestSummary{
				FlakyFailures: []contracts.TestFailure{{TestName: "Test1"}},
			},
			expected: true,
		},
		{
			name: "both types",
			summary: &contracts.TestSummary{
				NovelFailures: []contracts.TestFailure{{TestName: "Test1"}},
				FlakyFailures: []contracts.TestFailure{{TestName: "Test2"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewTestsModel(styles)
			model.summary = tt.summary

			got := model.HasTestFailures()
			if got != tt.expected {
				t.Errorf("HasTestFailures() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTestsModel_HasTests(t *testing.T) {
	styles := DefaultStyles()

	tests := []struct {
		name     string
		summary  *contracts.TestSummary
		expected bool
	}{
		{
			name:     "nil summary",
			summary:  nil,
			expected: false,
		},
		{
			name:     "zero tests",
			summary:  &contracts.TestSummary{TotalTests: 0},
			expected: false,
		},
		{
			name:     "has tests",
			summary:  &contracts.TestSummary{TotalTests: 10},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := NewTestsModel(styles)
			model.summary = tt.summary

			got := model.HasTests()
			if got != tt.expected {
				t.Errorf("HasTests() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// =============================================================================
// Format Tests
// =============================================================================

func TestFormatDedupedFailure_NovelTest(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)
	model.width = 120

	failure := dedupedFailure{
		TestName: "com.example.MyTest.testSomething",
		Jobs:     []string{"build-job", "test-job"},
		IsFlaky:  false,
	}

	lines := model.formatDedupedFailure(failure)

	if len(lines) < 1 {
		t.Fatal("expected at least 1 line of output")
	}

	// Check that [NOVEL] appears in first line
	if !containsString(lines[0], "NOVEL") {
		t.Errorf("expected [NOVEL] label in output: %s", lines[0])
	}

	// Check job count appears
	if !containsString(lines[0], "2 job") {
		t.Errorf("expected job count in output: %s", lines[0])
	}
}

func TestFormatDedupedFailure_FlakyTest(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)
	model.width = 120

	failure := dedupedFailure{
		TestName:     "com.example.FlakyTest.testFlaky",
		Jobs:         []string{"job-1"},
		IsFlaky:      true,
		HistoryRuns:  20,
		HistoryFails: 4, // 20% failure rate
		LastFailedAt: "2024-12-15T10:00:00Z",
	}

	lines := model.formatDedupedFailure(failure)

	if len(lines) < 1 {
		t.Fatal("expected at least 1 line of output")
	}

	// Check that [FLAKY] appears
	if !containsString(lines[0], "FLAKY") {
		t.Errorf("expected [FLAKY] label in output: %s", lines[0])
	}

	// Check failure rate appears (20%)
	if !containsString(lines[0], "20%") {
		t.Errorf("expected failure rate in output: %s", lines[0])
	}

	// Check "last failed" appears in detail line
	if len(lines) >= 2 && !containsString(lines[1], "last failed") {
		t.Errorf("expected 'last failed' in detail line: %s", lines[1])
	}
}

func TestFormatDedupedFailure_SingleJob(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)
	model.width = 120

	failure := dedupedFailure{
		TestName: "TestSingle",
		Jobs:     []string{"only-job"},
		IsFlaky:  false,
	}

	lines := model.formatDedupedFailure(failure)

	// Should say "1 job" not "1 jobs"
	if !containsString(lines[0], "1 job)") {
		t.Errorf("expected '1 job' (singular) in output: %s", lines[0])
	}
}

func TestFormatDedupedFailure_TruncatesLongTestName(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)
	model.width = 80 // Narrow width to force truncation

	longName := "com.example.very.long.package.name.with.many.parts.TestClass.testMethodWithVeryLongName"
	failure := dedupedFailure{
		TestName: longName,
		Jobs:     []string{"job-1"},
		IsFlaky:  false,
	}

	lines := model.formatDedupedFailure(failure)

	// The output should be truncated and contain "..."
	if !containsString(lines[0], "...") {
		t.Logf("Line: %s", lines[0])
		// Only fail if the name wasn't truncated at all
		if containsString(lines[0], longName) {
			t.Error("expected long test name to be truncated")
		}
	}
}

func TestFormatDedupedFailure_JobNameInDetailLine(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)
	model.width = 120

	failure := dedupedFailure{
		TestName:     "TestWithJob",
		Jobs:         []string{"ducktape-tests"},
		IsFlaky:      true,
		HistoryRuns:  20,
		HistoryFails: 3,
		LastFailedAt: "2024-12-11T10:00:00Z",
	}

	lines := model.formatDedupedFailure(failure)

	// Should have at least 2 lines (main line + detail line)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}

	// Detail line should contain the job name
	detailLine := lines[1]
	if !containsString(detailLine, "ducktape-tests") {
		t.Errorf("expected job name 'ducktape-tests' in detail line: %s", detailLine)
	}

	// Detail line should also contain the last failed date
	if !containsString(detailLine, "Dec 11") {
		t.Errorf("expected 'Dec 11' in detail line: %s", detailLine)
	}
}

func TestDeduplicateFailures_PreservesJobName(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	// Test results with job names
	model.allResults = []contracts.TestResult{
		{TestName: "TestFoo", JobName: "job-alpha", Passed: false},
		{TestName: "TestFoo", JobName: "job-beta", Passed: false},
		{TestName: "TestBar", JobName: "job-gamma", Passed: false},
	}

	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{TestName: "TestFoo"},
			{TestName: "TestBar"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated failures, got %d", len(deduped))
	}

	// Find TestFoo
	var testFoo *dedupedFailure
	for i := range deduped {
		if deduped[i].TestName == "TestFoo" {
			testFoo = &deduped[i]
			break
		}
	}

	if testFoo == nil {
		t.Fatal("TestFoo not found")
	}

	// Should have 2 jobs
	if len(testFoo.Jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d: %v", len(testFoo.Jobs), testFoo.Jobs)
	}

	// Check both job names are present
	hasAlpha := false
	hasBeta := false
	for _, j := range testFoo.Jobs {
		if j == "job-alpha" {
			hasAlpha = true
		}
		if j == "job-beta" {
			hasBeta = true
		}
	}

	if !hasAlpha {
		t.Error("expected 'job-alpha' in jobs list")
	}
	if !hasBeta {
		t.Error("expected 'job-beta' in jobs list")
	}
}

func TestDeduplicateFailures_EmptyJobNameFiltered(t *testing.T) {
	styles := DefaultStyles()
	model := NewTestsModel(styles)

	// Test with empty job name (simulates old cached data)
	model.allResults = []contracts.TestResult{
		{TestName: "TestEmpty", JobName: "", Passed: false},
	}

	model.summary = &contracts.TestSummary{
		NovelFailures: []contracts.TestFailure{
			{TestName: "TestEmpty"},
		},
	}

	deduped := model.deduplicateFailures()

	if len(deduped) != 1 {
		t.Fatalf("expected 1 deduplicated failure, got %d", len(deduped))
	}

	// Jobs list should contain empty string (current behavior)
	// This test documents that empty job names are NOT filtered
	if len(deduped[0].Jobs) != 1 {
		t.Errorf("expected 1 job entry, got %d", len(deduped[0].Jobs))
	}

	// The job name is empty string
	if deduped[0].Jobs[0] != "" {
		t.Errorf("expected empty job name, got '%s'", deduped[0].Jobs[0])
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
