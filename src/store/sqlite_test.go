package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
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
			info.TotalRuns, FlakeMinSamples)
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
	if info.TotalRuns != FlakeWindowSize {
		t.Errorf("expected TotalRuns=%d (window limit), got %d", FlakeWindowSize, info.TotalRuns)
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
