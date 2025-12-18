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

	// Verify it was stored
	results, err := th.GetRecentResults(ctx, "redpanda/redpanda", "test_produce_consume", 10)
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

func TestRecordResults_Batch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()
	results := []TestResult{
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 1, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_b", BuildNumber: 1, Passed: false, FailureMessage: "failed"},
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 2, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_b", BuildNumber: 2, Passed: true},
	}

	err = th.RecordResults(ctx, results)
	if err != nil {
		t.Fatalf("failed to record results: %v", err)
	}

	// Verify test_a has 2 results
	testAResults, err := th.GetRecentResults(ctx, "org/pipeline", "test_a", 10)
	if err != nil {
		t.Fatalf("failed to get test_a results: %v", err)
	}
	if len(testAResults) != 2 {
		t.Errorf("expected 2 results for test_a, got %d", len(testAResults))
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

	// Should still have only 1 result
	results, err := th.GetRecentResults(ctx, "org/pipeline", "test_x", 10)
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

func TestGetTestStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 20 results: 4 failures (20% failure rate)
	var results []TestResult
	for i := 1; i <= 20; i++ {
		passed := true
		if i%5 == 0 { // builds 5, 10, 15, 20 fail
			passed = false
		}
		results = append(results, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_flaky",
			BuildNumber: i,
			Passed:      passed,
		})
	}

	err = th.RecordResults(ctx, results)
	if err != nil {
		t.Fatalf("failed to record results: %v", err)
	}

	stats, err := th.GetTestStats(ctx, "org/pipeline", "test_flaky", 20)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalRuns != 20 {
		t.Errorf("expected 20 total runs, got %d", stats.TotalRuns)
	}
	if stats.FailedRuns != 4 {
		t.Errorf("expected 4 failed runs, got %d", stats.FailedRuns)
	}
	if stats.FailureRate != 0.2 {
		t.Errorf("expected 0.2 failure rate, got %f", stats.FailureRate)
	}
}

func TestGetTestStats_WindowLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Create 30 results: first 10 all fail, last 20 all pass
	var results []TestResult
	for i := 1; i <= 30; i++ {
		passed := i > 10 // builds 1-10 fail, 11-30 pass
		results = append(results, TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_window",
			BuildNumber: i,
			Passed:      passed,
		})
	}

	err = th.RecordResults(ctx, results)
	if err != nil {
		t.Fatalf("failed to record results: %v", err)
	}

	// Window of 20 should only see the last 20 (builds 11-30, all passing)
	stats, err := th.GetTestStats(ctx, "org/pipeline", "test_window", 20)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.TotalRuns != 20 {
		t.Errorf("expected 20 total runs, got %d", stats.TotalRuns)
	}
	if stats.FailedRuns != 0 {
		t.Errorf("expected 0 failed runs in window, got %d", stats.FailedRuns)
	}
}

func TestGetAllTestStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	results := []TestResult{
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 1, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_a", BuildNumber: 2, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_b", BuildNumber: 1, Passed: false},
		{PipelineID: "org/pipeline", TestName: "test_b", BuildNumber: 2, Passed: false},
		{PipelineID: "other/pipeline", TestName: "test_c", BuildNumber: 1, Passed: true},
	}

	err = th.RecordResults(ctx, results)
	if err != nil {
		t.Fatalf("failed to record results: %v", err)
	}

	allStats, err := th.GetAllTestStats(ctx, "org/pipeline", 20)
	if err != nil {
		t.Fatalf("failed to get all stats: %v", err)
	}

	if len(allStats) != 2 {
		t.Fatalf("expected 2 tests for org/pipeline, got %d", len(allStats))
	}

	// Find stats for each test
	statsMap := make(map[string]TestStats)
	for _, s := range allStats {
		statsMap[s.TestName] = s
	}

	if statsMap["test_a"].FailureRate != 0 {
		t.Errorf("test_a should have 0%% failure rate")
	}
	if statsMap["test_b"].FailureRate != 1.0 {
		t.Errorf("test_b should have 100%% failure rate")
	}
}

func TestGetRecentResults_Ordering(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	th, err := NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create TestHistory: %v", err)
	}
	defer th.Close()

	ctx := context.Background()

	// Insert in random order
	results := []TestResult{
		{PipelineID: "org/pipeline", TestName: "test_x", BuildNumber: 5, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_x", BuildNumber: 2, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_x", BuildNumber: 8, Passed: true},
		{PipelineID: "org/pipeline", TestName: "test_x", BuildNumber: 1, Passed: true},
	}

	err = th.RecordResults(ctx, results)
	if err != nil {
		t.Fatalf("failed to record results: %v", err)
	}

	// Should come back in descending build_number order
	recent, err := th.GetRecentResults(ctx, "org/pipeline", "test_x", 10)
	if err != nil {
		t.Fatalf("failed to get recent results: %v", err)
	}

	expectedOrder := []int{8, 5, 2, 1}
	for i, r := range recent {
		if r.BuildNumber != expectedOrder[i] {
			t.Errorf("position %d: expected build %d, got %d", i, expectedOrder[i], r.BuildNumber)
		}
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

	results, err := th.GetRecentResults(ctx, "org/pipeline", "test_time", 10)
	if err != nil {
		t.Fatalf("failed to get results: %v", err)
	}

	if !results[0].CreatedAt.Equal(specificTime) {
		t.Errorf("expected created_at %v, got %v", specificTime, results[0].CreatedAt)
	}
}
