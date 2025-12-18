package flaky

import (
	"context"
	"path/filepath"
	"testing"

	"destill-agent/src/contracts"
	"destill-agent/src/store"
)

func setupTestHistory(t *testing.T) *store.TestHistory {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	history, err := store.NewTestHistory(dbPath)
	if err != nil {
		t.Fatalf("failed to create test history: %v", err)
	}
	return history
}

func TestIsFlaky_NotEnoughSamples(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Add only 3 results (below MinSamples=5)
	for i := 1; i <= 3; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_new",
			BuildNumber: i,
			Passed:      false, // All fail, but not enough samples
		})
	}

	detector := NewDetector(history)
	isFlaky, _, totalRuns := detector.IsFlaky(ctx, "org/pipeline", "test_new")

	if isFlaky {
		t.Error("should not be flaky with insufficient samples")
	}
	if totalRuns != 3 {
		t.Errorf("expected 3 total runs, got %d", totalRuns)
	}
}

func TestIsFlaky_BelowThreshold(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Add 20 results with only 1 failure (5% failure rate, below 10% threshold)
	for i := 1; i <= 20; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_stable",
			BuildNumber: i,
			Passed:      i != 10, // Only build 10 fails
		})
	}

	detector := NewDetector(history)
	isFlaky, failureRate, _ := detector.IsFlaky(ctx, "org/pipeline", "test_stable")

	if isFlaky {
		t.Error("should not be flaky with 5% failure rate")
	}
	if failureRate != 0.05 {
		t.Errorf("expected 0.05 failure rate, got %f", failureRate)
	}
}

func TestIsFlaky_AboveThreshold(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Add 20 results with 4 failures (20% failure rate, above 10% threshold)
	for i := 1; i <= 20; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_flaky",
			BuildNumber: i,
			Passed:      i%5 != 0, // Builds 5, 10, 15, 20 fail
		})
	}

	detector := NewDetector(history)
	isFlaky, failureRate, _ := detector.IsFlaky(ctx, "org/pipeline", "test_flaky")

	if !isFlaky {
		t.Error("should be flaky with 20% failure rate")
	}
	if failureRate != 0.2 {
		t.Errorf("expected 0.2 failure rate, got %f", failureRate)
	}
}

func TestIsFlaky_ExactThreshold(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Add 20 results with 2 failures (10% failure rate, exactly at threshold)
	for i := 1; i <= 20; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_borderline",
			BuildNumber: i,
			Passed:      i != 5 && i != 15, // Builds 5 and 15 fail
		})
	}

	detector := NewDetector(history)
	isFlaky, failureRate, _ := detector.IsFlaky(ctx, "org/pipeline", "test_borderline")

	if !isFlaky {
		t.Error("should be flaky at exactly 10% threshold")
	}
	if failureRate != 0.1 {
		t.Errorf("expected 0.1 failure rate, got %f", failureRate)
	}
}

func TestGetFlakyTests(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Create a mix of stable and flaky tests
	for i := 1; i <= 20; i++ {
		// Stable test - never fails
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_stable",
			BuildNumber: i,
			Passed:      true,
		})

		// Flaky test - fails 25% of time
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_flaky",
			BuildNumber: i,
			Passed:      i%4 != 0, // Fails on 4, 8, 12, 16, 20
		})

		// Another pipeline - should not appear
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "other/pipeline",
			TestName:    "test_other_flaky",
			BuildNumber: i,
			Passed:      i%2 != 0, // 50% failure
		})
	}

	detector := NewDetector(history)
	flakyTests, err := detector.GetFlakyTests(ctx, "org/pipeline")
	if err != nil {
		t.Fatalf("failed to get flaky tests: %v", err)
	}

	if len(flakyTests) != 1 {
		t.Errorf("expected 1 flaky test, got %d", len(flakyTests))
	}

	if _, ok := flakyTests["test_flaky"]; !ok {
		t.Error("test_flaky should be in flaky tests")
	}

	if _, ok := flakyTests["test_stable"]; ok {
		t.Error("test_stable should not be in flaky tests")
	}
}

func TestClassifyFailures(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Set up history: test_flaky has high failure rate, test_novel is new
	for i := 1; i <= 20; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_flaky",
			BuildNumber: i,
			Passed:      i%4 != 0, // 25% failure rate
		})
	}

	detector := NewDetector(history)

	// Current build has both tests failing
	results := []contracts.TestResult{
		{TestName: "test_flaky", Passed: false, FailureMessage: "flaky failure"},
		{TestName: "test_novel", Passed: false, FailureMessage: "novel failure"},
		{TestName: "test_passing", Passed: true}, // Should be ignored
	}

	novel, flaky := detector.ClassifyFailures(ctx, "org/pipeline", results)

	if len(novel) != 1 {
		t.Errorf("expected 1 novel failure, got %d", len(novel))
	}
	if len(flaky) != 1 {
		t.Errorf("expected 1 flaky failure, got %d", len(flaky))
	}

	if novel[0].TestName != "test_novel" {
		t.Errorf("expected test_novel in novel, got %s", novel[0].TestName)
	}
	if flaky[0].TestName != "test_flaky" {
		t.Errorf("expected test_flaky in flaky, got %s", flaky[0].TestName)
	}

	if flaky[0].IsFlaky != true {
		t.Error("test_flaky should have IsFlaky=true")
	}
	if novel[0].IsFlaky != false {
		t.Error("test_novel should have IsFlaky=false")
	}
}

func TestBuildTestSummary(t *testing.T) {
	history := setupTestHistory(t)
	defer history.Close()

	ctx := context.Background()

	// Set up some history
	for i := 1; i <= 10; i++ {
		history.RecordResult(ctx, store.TestResult{
			PipelineID:  "org/pipeline",
			TestName:    "test_flaky",
			BuildNumber: i,
			Passed:      i%3 != 0, // ~33% failure rate
		})
	}

	detector := NewDetector(history)

	results := []contracts.TestResult{
		{TestName: "test_a", Passed: true},
		{TestName: "test_b", Passed: true},
		{TestName: "test_flaky", Passed: false, FailureMessage: "failed again"},
		{TestName: "test_new_fail", Passed: false, FailureMessage: "new failure"},
	}

	summary := detector.BuildTestSummary(ctx, "req-123", "org/pipeline", 11, results)

	if summary.TotalTests != 4 {
		t.Errorf("expected 4 total tests, got %d", summary.TotalTests)
	}
	if summary.PassedCount != 2 {
		t.Errorf("expected 2 passed, got %d", summary.PassedCount)
	}
	if summary.FailedCount != 2 {
		t.Errorf("expected 2 failed, got %d", summary.FailedCount)
	}
	if len(summary.NovelFailures) != 1 {
		t.Errorf("expected 1 novel failure, got %d", len(summary.NovelFailures))
	}
	if len(summary.FlakyFailures) != 1 {
		t.Errorf("expected 1 flaky failure, got %d", len(summary.FlakyFailures))
	}
}

func TestFormatFailureRateFull(t *testing.T) {
	tests := []struct {
		rate      float64
		totalRuns int
		want      string
	}{
		{0.20, 20, "4/20 failures"},
		{0.05, 20, "1/20 failure"},
		{0.0, 10, "0/10 failures"},
		{1.0, 5, "5/5 failures"},
	}

	for _, tt := range tests {
		got := FormatFailureRateFull(tt.rate, tt.totalRuns)
		if got != tt.want {
			t.Errorf("FormatFailureRateFull(%f, %d) = %q, want %q", tt.rate, tt.totalRuns, got, tt.want)
		}
	}
}
