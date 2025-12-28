package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"destill-agent/src/contracts"
)

func TestNewTestHistory(t *testing.T) {
	// Use temp directory for test
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	// Verify database file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestNewTestHistory_DefaultPath(t *testing.T) {
	// Skip in CI where home directory might not be writable
	if os.Getenv("CI") != "" {
		t.Skip("skipping test in CI environment")
	}

	th, err := NewTestHistory("")
	if err != nil {
		t.Fatalf("failed to create TestHistory with default path: %v", err)
	}
	defer th.Close()
}

func TestRecordResult(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()
	result := TestResult{
		PipelineID:     "redpanda/redpanda",
		TestName:       "test_produce_consume",
		BuildNumber:    100,
		Passed:         false,
		FailureMessage: "AssertionError: expected 3 partitions",
		BuildURL:       "https://buildkite.com/redpanda/redpanda/builds/100",
	}

	err = th.RecordResult(ctx, result)
	if err != nil {
		t.Fatalf("failed to record result: %v", err)
	}

	// Verify it was stored using GetBuildResults
	results, err := th.GetBuildResults(ctx, "redpanda/redpanda", 100)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.PipelineID != "redpanda/redpanda" {
		t.Errorf("wrong pipeline_id: %s", r.PipelineID)
	}
	if r.TestName != "test_produce_consume" {
		t.Errorf("wrong test_name: %s", r.TestName)
	}
	if r.BuildNumber != 100 {
		t.Errorf("wrong build_number: %d", r.BuildNumber)
	}
	if r.Passed {
		t.Error("expected passed=false")
	}
	if r.FailureMessage != "AssertionError: expected 3 partitions" {
		t.Errorf("wrong failure_message: %s", r.FailureMessage)
	}
}

func TestRecordResult_Upsert(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Insert initial result
	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "test_x",
		BuildNumber: 1,
		Passed:      false,
	})
	if err != nil {
		t.Fatalf("failed to record initial result: %v", err)
	}

	// Update same record (same pipeline/test/build)
	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "test_x",
		BuildNumber: 1,
		Passed:      true, // Changed
	})
	if err != nil {
		t.Fatalf("failed to update result: %v", err)
	}

	// Should still have only 1 result, verified via GetBuildResults
	results, err := th.GetBuildResults(ctx, "org/pipeline", 1)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after upsert, got %d", len(results))
	}
	if !results[0].Passed {
		t.Error("expected result to be updated to passed=true")
	}
}

func TestCreatedAt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	specificTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "test_time",
		BuildNumber: 1,
		Passed:      true,
		CreatedAt:   specificTime,
	})
	if err != nil {
		t.Fatalf("failed to record result: %v", err)
	}

	results, err := th.GetBuildResults(ctx, "org/pipeline", 1)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}

	if !results[0].CreatedAt.Equal(specificTime) {
		t.Errorf("expected created_at %v, got %v", specificTime, results[0].CreatedAt)
	}
}

func TestGetProcessedBuilds(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record some test results across different builds
	results := []TestResult{
		{PipelineID: "org/pipeline", TestName: "test1", BuildNumber: 100, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test2", BuildNumber: 100, Passed: false},
		{PipelineID: "org/pipeline", TestName: "test1", BuildNumber: 101, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test1", BuildNumber: 102, Passed: true},
		{PipelineID: "other/pipeline", TestName: "test1", BuildNumber: 100, Passed: true},
	}

	for _, r := range results {
		if err := th.RecordResult(ctx, r); err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	// Get processed builds for org/pipeline
	processed, err := th.GetProcessedBuilds(ctx, "org/pipeline")
	if err != nil {
		t.Fatalf("failed to get processed builds: %v", err)
	}

	// Should have builds 100, 101, 102
	if len(processed) != 3 {
		t.Errorf("expected 3 processed builds, got %d", len(processed))
	}

	if !processed[100] || !processed[101] || !processed[102] {
		t.Errorf("expected builds 100, 101, 102 to be processed, got %v", processed)
	}

	// Should not include build 100 from other/pipeline
	processedOther, err := th.GetProcessedBuilds(ctx, "other/pipeline")
	if err != nil {
		t.Fatalf("failed to get processed builds: %v", err)
	}

	if len(processedOther) != 1 {
		t.Errorf("expected 1 processed build for other/pipeline, got %d", len(processedOther))
	}

	if !processedOther[100] {
		t.Errorf("expected build 100 to be processed for other/pipeline")
	}
}

func TestHasBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Initially should not have the build
	has, err := th.HasBuild(ctx, "org/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check build: %v", err)
	}
	if has {
		t.Error("expected HasBuild to return false for non-existent build")
	}

	// Record a result
	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "test1",
		BuildNumber: 100,
		Passed:      true,
	})
	if err != nil {
		t.Fatalf("failed to record result: %v", err)
	}

	// Now should have the build
	has, err = th.HasBuild(ctx, "org/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check build: %v", err)
	}
	if !has {
		t.Error("expected HasBuild to return true after recording result")
	}

	// Different pipeline should not have it
	has, err = th.HasBuild(ctx, "other/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check build: %v", err)
	}
	if has {
		t.Error("expected HasBuild to return false for different pipeline")
	}
}

func TestGetBuildResults(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record multiple results for the same build
	results := []TestResult{
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 100, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_b", BuildNumber: 100, Passed: false, FailureMessage: "failed"},
		{PipelineID: "org/pipeline", TestName: "test_c", BuildNumber: 100, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 101, Passed: true}, // Different build
	}

	for _, r := range results {
		if err := th.RecordResult(ctx, r); err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	// Get results for build 100
	buildResults, err := th.GetBuildResults(ctx, "org/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to get build results: %v", err)
	}

	if len(buildResults) != 3 {
		t.Errorf("expected 3 results for build 100, got %d", len(buildResults))
	}

	// Verify we got the right tests
	testNames := make(map[string]bool)
	for _, r := range buildResults {
		testNames[r.TestName] = true
		if r.BuildNumber != 100 {
			t.Errorf("expected build 100, got %d", r.BuildNumber)
		}
	}

	if !testNames["test_a"] || !testNames["test_b"] || !testNames["test_c"] {
		t.Errorf("expected test_a, test_b, test_c, got %v", testNames)
	}
}

// =============================================================================
// Flaky Detection Tests
// =============================================================================

func TestGetTestFlakeInfo_FlakyTest(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create history with 20% failure rate (above 10% threshold)
	// Builds 1-10: 8 passes, 2 failures = 20% failure rate
	for i := 1; i <= 10; i++ {
		passed := i != 3 && i != 7 // Fail on builds 3 and 7
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "flaky_test",
			BuildNumber: i,
			Passed:      passed,
			CreatedAt:   time.Date(2024, 1, i, 10, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	// Check flakiness for build 11 (excludes build 11)
	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "flaky_test", 11)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	if !info.IsFlaky {
		t.Error("expected test to be flaky (20% > 10% threshold)")
	}
	if info.TotalRuns != 10 {
		t.Errorf("expected TotalRuns=10, got %d", info.TotalRuns)
	}
	if info.FailedRuns != 2 {
		t.Errorf("expected FailedRuns=2, got %d", info.FailedRuns)
	}
	if info.FailureRate != 0.2 {
		t.Errorf("expected FailureRate=0.2, got %f", info.FailureRate)
	}
}

func TestGetTestFlakeInfo_NotFlaky_BelowThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create history with 5% failure rate (below 10% threshold)
	// 20 builds: 19 passes, 1 failure = 5% failure rate
	for i := 1; i <= 20; i++ {
		passed := i != 10 // Only fail on build 10
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "stable_test",
			BuildNumber: i,
			Passed:      passed,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "stable_test", 21)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	if info.IsFlaky {
		t.Error("expected test to NOT be flaky (5% < 10% threshold)")
	}
	if info.FailureRate != 0.05 {
		t.Errorf("expected FailureRate=0.05, got %f", info.FailureRate)
	}
}

func TestGetTestFlakeInfo_NotFlaky_NotEnoughSamples(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create history with only 4 samples (below FlakeMinSamples=5)
	// Even with 50% failure rate, should not be marked flaky
	for i := 1; i <= 4; i++ {
		passed := i%2 == 0 // 50% fail rate
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "new_test",
			BuildNumber: i,
			Passed:      passed,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "new_test", 5)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	if info.IsFlaky {
		t.Errorf("expected test to NOT be flaky (only %d samples < %d required)",
			info.TotalRuns, contracts.FlakeMinSamples)
	}
	if info.TotalRuns != 4 {
		t.Errorf("expected TotalRuns=4, got %d", info.TotalRuns)
	}
}

func TestGetTestFlakeInfo_ExcludesCurrentBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 6 builds: 5 passes + 1 failure on build 6
	for i := 1; i <= 6; i++ {
		passed := i != 6
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_exclude",
			BuildNumber: i,
			Passed:      passed,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	// Check for build 6 - should exclude build 6's failure
	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "test_exclude", 6)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	// Should only see 5 builds (1-5), all passing
	if info.TotalRuns != 5 {
		t.Errorf("expected TotalRuns=5 (excluding current build), got %d", info.TotalRuns)
	}
	if info.FailedRuns != 0 {
		t.Errorf("expected FailedRuns=0 (build 6 excluded), got %d", info.FailedRuns)
	}
	if info.IsFlaky {
		t.Error("expected test to NOT be flaky (0% failure rate when excluding current build)")
	}
}

func TestGetTestFlakeInfo_LastFailedAt(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create history with failures on specific dates
	failDate := time.Date(2024, 12, 15, 14, 30, 0, 0, time.UTC)
	for i := 1; i <= 10; i++ {
		passed := i != 5 && i != 8 // Fail on builds 5 and 8
		createdAt := time.Date(2024, 12, i, 10, 0, 0, 0, time.UTC)
		if i == 8 {
			createdAt = failDate // Most recent failure
		}
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_lastfailed",
			BuildNumber: i,
			Passed:      passed,
			CreatedAt:   createdAt,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "test_lastfailed", 11)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	if info.LastFailedAt.IsZero() {
		t.Error("expected LastFailedAt to be set")
	}
	// Should be the most recent failure (build 8)
	if !info.LastFailedAt.Equal(failDate) {
		t.Errorf("expected LastFailedAt=%v, got %v", failDate, info.LastFailedAt)
	}
}

func TestGetTestFlakeInfo_NoHistory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Query for a test with no history
	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "nonexistent_test", 1)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	if info.IsFlaky {
		t.Error("expected test with no history to NOT be flaky")
	}
	if info.TotalRuns != 0 {
		t.Errorf("expected TotalRuns=0, got %d", info.TotalRuns)
	}
	if info.FailedRuns != 0 {
		t.Errorf("expected FailedRuns=0, got %d", info.FailedRuns)
	}
	if !info.LastFailedAt.IsZero() {
		t.Error("expected LastFailedAt to be zero for test with no history")
	}
}

func TestGetTestFlakeInfo_WindowLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 30 builds of history
	// Old builds (1-10): all failing
	// Recent builds (11-30): all passing
	for i := 1; i <= 30; i++ {
		passed := i > 10 // First 10 fail, rest pass
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_window",
			BuildNumber: i,
			Passed:      passed,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	// Check for build 31 - should only look at last 20 builds (11-30)
	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "test_window", 31)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	// Window should only include builds 11-30 (all passing)
	if info.TotalRuns != contracts.FlakeWindowSize {
		t.Errorf("expected TotalRuns=%d (window limit), got %d", contracts.FlakeWindowSize, info.TotalRuns)
	}
	if info.FailedRuns != 0 {
		t.Errorf("expected FailedRuns=0 (old failures outside window), got %d", info.FailedRuns)
	}
	if info.IsFlaky {
		t.Error("expected test to NOT be flaky (failures outside window)")
	}
}

func TestGetTestFlakeInfo_IsolatedByPipeline(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Same test name in two different pipelines
	// Pipeline A: all failing (flaky)
	// Pipeline B: all passing (not flaky)
	for i := 1; i <= 10; i++ {
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline-a",
			TestName:    "shared_test_name",
			BuildNumber: i,
			Passed:      false, // All fail
		})
		if err != nil {
			t.Fatalf("failed to record result for pipeline-a: %v", err)
		}

		err = th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline-b",
			TestName:    "shared_test_name",
			BuildNumber: i,
			Passed:      true, // All pass
		})
		if err != nil {
			t.Fatalf("failed to record result for pipeline-b: %v", err)
		}
	}

	// Check pipeline A
	infoA, err := th.GetTestFlakeInfo(ctx, "org/pipeline-a", "shared_test_name", 11)
	if err != nil {
		t.Fatalf("failed to get flake info for pipeline-a: %v", err)
	}
	if !infoA.IsFlaky {
		t.Error("expected test to be flaky in pipeline-a (100% failure rate)")
	}

	// Check pipeline B
	infoB, err := th.GetTestFlakeInfo(ctx, "org/pipeline-b", "shared_test_name", 11)
	if err != nil {
		t.Fatalf("failed to get flake info for pipeline-b: %v", err)
	}
	if infoB.IsFlaky {
		t.Error("expected test to NOT be flaky in pipeline-b (0% failure rate)")
	}
}

func TestGetTestFlakeInfo_ExactThreshold(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create exactly 10% failure rate (at threshold)
	// 10 builds: 9 passes, 1 failure = 10%
	for i := 1; i <= 10; i++ {
		passed := i != 5 // One failure
		err := th.RecordResult(ctx, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "threshold_test",
			BuildNumber: i,
			Passed:      passed,
		})
		if err != nil {
			t.Fatalf("failed to record result: %v", err)
		}
	}

	info, err := th.GetTestFlakeInfo(ctx, "org/pipeline", "threshold_test", 11)
	if err != nil {
		t.Fatalf("failed to get flake info: %v", err)
	}

	// Exactly at threshold (10% == 10%) should be flaky
	if !info.IsFlaky {
		t.Error("expected test to be flaky (exactly at 10% threshold)")
	}
	if info.FailureRate != 0.1 {
		t.Errorf("expected FailureRate=0.1, got %f", info.FailureRate)
	}
}

func TestRecordResult_JobName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record a result with JobName
	err = th.RecordResult(ctx, TestResult{
		PipelineID:     "redpanda/redpanda",
		TestName:       "test_with_job",
		BuildNumber:    200,
		Passed:         false,
		FailureMessage: "test failed",
		BuildURL:       "https://example.com/build/200",
		JobName:        "ducktape-tests",
	})
	if err != nil {
		t.Fatalf("failed to record result: %v", err)
	}

	// Retrieve and verify JobName is preserved
	results, err := th.GetBuildResults(ctx, "redpanda/redpanda", 200)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.JobName != "ducktape-tests" {
		t.Errorf("expected JobName='ducktape-tests', got '%s'", r.JobName)
	}
	if r.TestName != "test_with_job" {
		t.Errorf("expected TestName='test_with_job', got '%s'", r.TestName)
	}
}

func TestRecordResult_MultipleJobsSameTest(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record same test from different jobs in same build
	// Note: SQLite UNIQUE constraint is on (pipeline_id, test_name, build_number)
	// so only the last insert wins
	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "shared_test",
		BuildNumber: 300,
		Passed:      false,
		JobName:     "job-1",
	})
	if err != nil {
		t.Fatalf("failed to record first result: %v", err)
	}

	// This will replace the previous one due to UNIQUE constraint
	err = th.RecordResult(ctx, TestResult{
		PipelineID:  "org/pipeline",
		TestName:    "shared_test",
		BuildNumber: 300,
		Passed:      false,
		JobName:     "job-2",
	})
	if err != nil {
		t.Fatalf("failed to record second result: %v", err)
	}

	results, err := th.GetBuildResults(ctx, "org/pipeline", 300)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}

	// Should only have 1 result (last one wins)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Last job recorded should be preserved
	if results[0].JobName != "job-2" {
		t.Errorf("expected JobName='job-2', got '%s'", results[0].JobName)
	}
}

// =============================================================================
// Finding History Tests
// =============================================================================

func TestRecordFinding(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()
	finding := FindingResult{
		PipelineID:  "org/pipeline",
		MessageHash: "abc123hash",
		BuildNumber: 100,
		JobName:     "test-job",
		JobPassed:   false,
		Severity:    "ERROR",
		Confidence:  0.85,
	}

	err = th.RecordFinding(ctx, finding)
	if err != nil {
		t.Fatalf("failed to record finding: %v", err)
	}

	// Verify using GetFindingHistory
	results, err := th.GetFindingHistory(ctx, "org/pipeline", "abc123hash", 0)
	if err != nil {
		t.Fatalf("failed to get finding history: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.MessageHash != "abc123hash" {
		t.Errorf("wrong message_hash: %s", r.MessageHash)
	}
	if r.JobPassed {
		t.Error("expected job_passed=false")
	}
	if r.Severity != "ERROR" {
		t.Errorf("wrong severity: %s", r.Severity)
	}
	if r.Confidence != 0.85 {
		t.Errorf("wrong confidence: %f", r.Confidence)
	}
}

func TestRecordFinding_Upsert(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Insert initial finding
	err = th.RecordFinding(ctx, FindingResult{
		PipelineID:  "org/pipeline",
		MessageHash: "hash1",
		BuildNumber: 1,
		JobName:     "job1",
		JobPassed:   false,
		Confidence:  0.5,
	})
	if err != nil {
		t.Fatalf("failed to record initial finding: %v", err)
	}

	// Update same record (same pipeline/hash/build/job)
	err = th.RecordFinding(ctx, FindingResult{
		PipelineID:  "org/pipeline",
		MessageHash: "hash1",
		BuildNumber: 1,
		JobName:     "job1",
		JobPassed:   true, // Changed
		Confidence:  0.9,  // Changed
	})
	if err != nil {
		t.Fatalf("failed to update finding: %v", err)
	}

	results, err := th.GetFindingHistory(ctx, "org/pipeline", "hash1", 0)
	if err != nil {
		t.Fatalf("failed to get history: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result after upsert, got %d", len(results))
	}
	if !results[0].JobPassed {
		t.Error("expected result to be updated to job_passed=true")
	}
	if results[0].Confidence != 0.9 {
		t.Errorf("expected confidence=0.9, got %f", results[0].Confidence)
	}
}

func TestGetFindingNoveltyInfo_Novel(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Query for a finding with no history
	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "never_seen_hash", 1)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if !info.IsNovel {
		t.Error("expected finding to be novel (never seen before)")
	}
	if info.TotalOccurrences != 0 {
		t.Errorf("expected TotalOccurrences=0, got %d", info.TotalOccurrences)
	}
	if info.SeenInPassingJobs {
		t.Error("expected SeenInPassingJobs=false for novel finding")
	}
}

func TestGetFindingNoveltyInfo_SeenBefore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record findings across multiple builds
	for i := 1; i <= 5; i++ {
		err := th.RecordFinding(ctx, FindingResult{
			PipelineID:  "org/pipeline",
			MessageHash: "recurring_hash",
			BuildNumber: i,
			JobName:     "job1",
			JobPassed:   false, // All in failing jobs
		})
		if err != nil {
			t.Fatalf("failed to record finding: %v", err)
		}
	}

	// Query for build 6 (excludes build 6, sees builds 1-5)
	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "recurring_hash", 6)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if info.IsNovel {
		t.Error("expected finding to NOT be novel (seen 5 times before)")
	}
	if info.TotalOccurrences != 5 {
		t.Errorf("expected TotalOccurrences=5, got %d", info.TotalOccurrences)
	}
	if info.FailingOccurs != 5 {
		t.Errorf("expected FailingOccurs=5, got %d", info.FailingOccurs)
	}
	if info.FirstSeenBuild != 1 {
		t.Errorf("expected FirstSeenBuild=1, got %d", info.FirstSeenBuild)
	}
	if info.LastSeenBuild != 5 {
		t.Errorf("expected LastSeenBuild=5, got %d", info.LastSeenBuild)
	}
}

func TestGetFindingNoveltyInfo_SeenInPassing(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record findings in both passing and failing jobs
	for i := 1; i <= 6; i++ {
		jobPassed := i%2 == 0 // Even builds pass
		err := th.RecordFinding(ctx, FindingResult{
			PipelineID:  "org/pipeline",
			MessageHash: "mixed_hash",
			BuildNumber: i,
			JobName:     "job1",
			JobPassed:   jobPassed,
		})
		if err != nil {
			t.Fatalf("failed to record finding: %v", err)
		}
	}

	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "mixed_hash", 7)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if !info.SeenInPassingJobs {
		t.Error("expected SeenInPassingJobs=true")
	}
	if info.PassingOccurs != 3 {
		t.Errorf("expected PassingOccurs=3, got %d", info.PassingOccurs)
	}
	if info.FailingOccurs != 3 {
		t.Errorf("expected FailingOccurs=3, got %d", info.FailingOccurs)
	}
}

func TestGetFindingNoveltyInfo_ExcludesCurrentBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record only one finding on build 5
	err = th.RecordFinding(ctx, FindingResult{
		PipelineID:  "org/pipeline",
		MessageHash: "exclude_test_hash",
		BuildNumber: 5,
		JobName:     "job1",
		JobPassed:   false,
	})
	if err != nil {
		t.Fatalf("failed to record finding: %v", err)
	}

	// Query for build 5 - should exclude it
	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "exclude_test_hash", 5)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if !info.IsNovel {
		t.Error("expected finding to be novel when current build is excluded")
	}
	if info.TotalOccurrences != 0 {
		t.Errorf("expected TotalOccurrences=0 (build 5 excluded), got %d", info.TotalOccurrences)
	}
}

func TestGetFindingNoveltyInfo_WindowLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 60 builds of history (more than FindingWindowSize=50)
	for i := 1; i <= 60; i++ {
		jobPassed := i <= 10 // First 10 pass, rest fail
		err := th.RecordFinding(ctx, FindingResult{
			PipelineID:  "org/pipeline",
			MessageHash: "window_hash",
			BuildNumber: i,
			JobName:     "job1",
			JobPassed:   jobPassed,
		})
		if err != nil {
			t.Fatalf("failed to record finding: %v", err)
		}
	}

	// Query for build 61 - should only look at last 50 builds (11-60)
	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "window_hash", 61)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if info.TotalOccurrences != contracts.FindingWindowSize {
		t.Errorf("expected TotalOccurrences=%d (window limit), got %d",
			contracts.FindingWindowSize, info.TotalOccurrences)
	}
	// Builds 11-60 are all failing, so no passing occurrences in window
	if info.SeenInPassingJobs {
		t.Error("expected SeenInPassingJobs=false (passing builds outside window)")
	}
}

func TestRecordFindingsFromCards(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	cards := []contracts.TriageCard{
		{
			MessageHash:     "hash1",
			JobName:         "job-a",
			Severity:        "ERROR",
			ConfidenceScore: 0.9,
			Metadata:        map[string]string{"job_state": "failed"},
		},
		{
			MessageHash:     "hash2",
			JobName:         "job-b",
			Severity:        "FATAL",
			ConfidenceScore: 0.95,
			Metadata:        map[string]string{"job_state": "passed"},
		},
		{
			// Missing MessageHash - should be skipped
			JobName:  "job-c",
			Metadata: map[string]string{"job_state": "failed"},
		},
		{
			// Missing job_state - should be skipped
			MessageHash: "hash3",
			JobName:     "job-d",
		},
	}

	err = th.RecordFindingsFromCards(ctx, "org/pipeline", 100, cards)
	if err != nil {
		t.Fatalf("failed to record findings from cards: %v", err)
	}

	// Should have 2 findings (hash1 and hash2)
	info1, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "hash1", 0)
	if err != nil {
		t.Fatalf("failed to get info for hash1: %v", err)
	}
	if info1.IsNovel {
		t.Error("expected hash1 to be recorded")
	}
	if info1.FailingOccurs != 1 {
		t.Errorf("expected hash1 FailingOccurs=1, got %d", info1.FailingOccurs)
	}

	info2, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "hash2", 0)
	if err != nil {
		t.Fatalf("failed to get info for hash2: %v", err)
	}
	if info2.IsNovel {
		t.Error("expected hash2 to be recorded")
	}
	if info2.PassingOccurs != 1 {
		t.Errorf("expected hash2 PassingOccurs=1, got %d", info2.PassingOccurs)
	}

	// hash3 should not exist (no job_state)
	info3, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "hash3", 0)
	if err != nil {
		t.Fatalf("failed to get info for hash3: %v", err)
	}
	if !info3.IsNovel {
		t.Error("expected hash3 to NOT be recorded (missing job_state)")
	}
}

func TestPruneOldFindings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 20 builds of history
	for i := 1; i <= 20; i++ {
		err := th.RecordFinding(ctx, FindingResult{
			PipelineID:  "org/pipeline",
			MessageHash: "prune_hash",
			BuildNumber: i,
			JobName:     "job1",
			JobPassed:   false,
		})
		if err != nil {
			t.Fatalf("failed to record finding: %v", err)
		}
	}

	// Prune keeping only last 5 builds
	err = th.PruneOldFindings(ctx, "org/pipeline", 5)
	if err != nil {
		t.Fatalf("failed to prune findings: %v", err)
	}

	// Should only have 5 findings left (builds 16-20)
	info, err := th.GetFindingNoveltyInfo(ctx, "org/pipeline", "prune_hash", 0)
	if err != nil {
		t.Fatalf("failed to get novelty info: %v", err)
	}

	if info.TotalOccurrences != 5 {
		t.Errorf("expected TotalOccurrences=5 after pruning, got %d", info.TotalOccurrences)
	}
	if info.FirstSeenBuild != 16 {
		t.Errorf("expected FirstSeenBuild=16 after pruning, got %d", info.FirstSeenBuild)
	}
	if info.LastSeenBuild != 20 {
		t.Errorf("expected LastSeenBuild=20 after pruning, got %d", info.LastSeenBuild)
	}
}

func TestHasFindingBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Initially should not have the build
	has, err := th.HasFindingBuild(ctx, "org/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check finding build: %v", err)
	}
	if has {
		t.Error("expected HasFindingBuild to return false for non-existent build")
	}

	// Record a finding
	err = th.RecordFinding(ctx, FindingResult{
		PipelineID:  "org/pipeline",
		MessageHash: "has_build_hash",
		BuildNumber: 100,
		JobName:     "job1",
		JobPassed:   false,
	})
	if err != nil {
		t.Fatalf("failed to record finding: %v", err)
	}

	// Now should have the build
	has, err = th.HasFindingBuild(ctx, "org/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check finding build: %v", err)
	}
	if !has {
		t.Error("expected HasFindingBuild to return true after recording finding")
	}

	// Different pipeline should not have it
	has, err = th.HasFindingBuild(ctx, "other/pipeline", 100)
	if err != nil {
		t.Fatalf("failed to check finding build: %v", err)
	}
	if has {
		t.Error("expected HasFindingBuild to return false for different pipeline")
	}
}

func TestLoadFindingNoveltyMap(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create findings with different characteristics
	// hash1: only in failing jobs
	// hash2: only in passing jobs
	// hash3: in both passing and failing jobs
	findings := []FindingResult{
		{PipelineID: "org/pipeline", MessageHash: "hash1", BuildNumber: 1, JobName: "job1", JobPassed: false},
		{PipelineID: "org/pipeline", MessageHash: "hash1", BuildNumber: 2, JobName: "job1", JobPassed: false},
		{PipelineID: "org/pipeline", MessageHash: "hash2", BuildNumber: 1, JobName: "job2", JobPassed: true},
		{PipelineID: "org/pipeline", MessageHash: "hash2", BuildNumber: 3, JobName: "job2", JobPassed: true},
		{PipelineID: "org/pipeline", MessageHash: "hash3", BuildNumber: 1, JobName: "job3", JobPassed: false},
		{PipelineID: "org/pipeline", MessageHash: "hash3", BuildNumber: 2, JobName: "job3", JobPassed: true},
		{PipelineID: "org/pipeline", MessageHash: "hash3", BuildNumber: 3, JobName: "job3", JobPassed: false},
	}

	for _, f := range findings {
		if err := th.RecordFinding(ctx, f); err != nil {
			t.Fatalf("failed to record finding: %v", err)
		}
	}

	// Load the map for build 10 (excludes nothing since no build 10)
	noveltyMap, err := th.LoadFindingNoveltyMap(ctx, "org/pipeline", 10)
	if err != nil {
		t.Fatalf("failed to load novelty map: %v", err)
	}

	// Check hash1: 2 failing occurrences
	if info, ok := noveltyMap["hash1"]; !ok {
		t.Error("expected hash1 in map")
	} else {
		if info.IsNovel {
			t.Error("hash1 should not be novel")
		}
		if info.SeenInPassingJobs {
			t.Error("hash1 should not be seen in passing jobs")
		}
		if info.FailingOccurs != 2 {
			t.Errorf("hash1 expected FailingOccurs=2, got %d", info.FailingOccurs)
		}
	}

	// Check hash2: 2 passing occurrences
	if info, ok := noveltyMap["hash2"]; !ok {
		t.Error("expected hash2 in map")
	} else {
		if !info.SeenInPassingJobs {
			t.Error("hash2 should be seen in passing jobs")
		}
		if info.PassingOccurs != 2 {
			t.Errorf("hash2 expected PassingOccurs=2, got %d", info.PassingOccurs)
		}
	}

	// Check hash3: mixed
	if info, ok := noveltyMap["hash3"]; !ok {
		t.Error("expected hash3 in map")
	} else {
		if !info.SeenInPassingJobs {
			t.Error("hash3 should be seen in passing jobs")
		}
		if info.PassingOccurs != 1 {
			t.Errorf("hash3 expected PassingOccurs=1, got %d", info.PassingOccurs)
		}
		if info.FailingOccurs != 2 {
			t.Errorf("hash3 expected FailingOccurs=2, got %d", info.FailingOccurs)
		}
		if info.FirstSeenBuild != 1 {
			t.Errorf("hash3 expected FirstSeenBuild=1, got %d", info.FirstSeenBuild)
		}
		if info.LastSeenBuild != 3 {
			t.Errorf("hash3 expected LastSeenBuild=3, got %d", info.LastSeenBuild)
		}
	}

	// Check novel hash (not in map)
	if _, ok := noveltyMap["never_seen"]; ok {
		t.Error("never_seen should not be in map")
	}
}

func TestLoadFindingNoveltyMap_ExcludesBuild(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Record findings on builds 1 and 2
	th.RecordFinding(ctx, FindingResult{
		PipelineID: "org/pipeline", MessageHash: "hash1", BuildNumber: 1, JobName: "job1", JobPassed: false,
	})
	th.RecordFinding(ctx, FindingResult{
		PipelineID: "org/pipeline", MessageHash: "hash1", BuildNumber: 2, JobName: "job1", JobPassed: true,
	})

	// Load excluding build 2
	noveltyMap, err := th.LoadFindingNoveltyMap(ctx, "org/pipeline", 2)
	if err != nil {
		t.Fatalf("failed to load novelty map: %v", err)
	}

	info := noveltyMap["hash1"]
	// Should only see build 1 (failing)
	if info.TotalOccurrences != 1 {
		t.Errorf("expected TotalOccurrences=1 (build 2 excluded), got %d", info.TotalOccurrences)
	}
	if info.SeenInPassingJobs {
		t.Error("should not be seen in passing jobs (build 2 excluded)")
	}
}

func TestLoadFindingNoveltyMap_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Load from empty database
	noveltyMap, err := th.LoadFindingNoveltyMap(ctx, "org/pipeline", 1)
	if err != nil {
		t.Fatalf("failed to load novelty map: %v", err)
	}

	if len(noveltyMap) != 0 {
		t.Errorf("expected empty map, got %d entries", len(noveltyMap))
	}
}
