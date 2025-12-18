// Package flaky provides flaky test detection via historical analysis.
package flaky

import (
	"context"

	"destill-agent/src/contracts"
	"destill-agent/src/store"
)

const (
	// WindowSize is the number of recent builds to consider for flakiness.
	WindowSize = 20

	// FlakeThreshold is the minimum failure rate to consider a test flaky.
	// 0.10 = 10% of runs failed = flaky
	FlakeThreshold = 0.10

	// MinSamples is the minimum number of runs required to make a judgment.
	MinSamples = 5
)

// Detector analyzes test history to identify flaky tests.
type Detector struct {
	history *store.TestHistory
}

// NewDetector creates a new flaky test detector.
func NewDetector(history *store.TestHistory) *Detector {
	return &Detector{history: history}
}

// IsFlaky checks if a specific test is flaky based on historical data.
// Returns (isFlaky, failureRate, totalRuns).
func (d *Detector) IsFlaky(ctx context.Context, pipelineID, testName string) (bool, float64, int) {
	stats, err := d.history.GetTestStats(ctx, pipelineID, testName, WindowSize)
	if err != nil {
		return false, 0, 0
	}

	// Need minimum samples to make a judgment
	if stats.TotalRuns < MinSamples {
		return false, stats.FailureRate, stats.TotalRuns
	}

	// Flaky if failure rate exceeds threshold
	isFlaky := stats.FailureRate >= FlakeThreshold
	return isFlaky, stats.FailureRate, stats.TotalRuns
}

// GetFlakyTests returns all flaky tests for a pipeline.
func (d *Detector) GetFlakyTests(ctx context.Context, pipelineID string) (map[string]FlakyInfo, error) {
	allStats, err := d.history.GetAllTestStats(ctx, pipelineID, WindowSize)
	if err != nil {
		return nil, err
	}

	flakyTests := make(map[string]FlakyInfo)
	for _, stats := range allStats {
		if stats.TotalRuns >= MinSamples && stats.FailureRate >= FlakeThreshold {
			flakyTests[stats.TestName] = FlakyInfo{
				FailureRate: stats.FailureRate,
				TotalRuns:   stats.TotalRuns,
				FailedRuns:  stats.FailedRuns,
			}
		}
	}

	return flakyTests, nil
}

// FlakyInfo contains information about a flaky test.
type FlakyInfo struct {
	FailureRate float64
	TotalRuns   int
	FailedRuns  int
}

// ClassifyFailures classifies test failures as novel or flaky.
// Takes test results from a single build and returns categorized failures.
func (d *Detector) ClassifyFailures(ctx context.Context, pipelineID string, results []contracts.TestResult) (novel, flaky []contracts.TestFailure) {
	for _, r := range results {
		if r.Passed {
			continue
		}

		isFlaky, failureRate, totalRuns := d.IsFlaky(ctx, pipelineID, r.TestName)

		failure := contracts.TestFailure{
			TestName:       r.TestName,
			FailureMessage: r.FailureMessage,
			FailureRate:    failureRate,
			IsFlaky:        isFlaky,
		}

		// Novel if: not enough history OR failure rate below threshold
		if totalRuns < MinSamples || !isFlaky {
			novel = append(novel, failure)
		} else {
			flaky = append(flaky, failure)
		}
	}

	return novel, flaky
}

// BuildTestSummary creates a TestSummary from test results with flakiness classification.
func (d *Detector) BuildTestSummary(ctx context.Context, requestID, pipelineID string, buildNumber int, results []contracts.TestResult) contracts.TestSummary {
	summary := contracts.TestSummary{
		RequestID:   requestID,
		PipelineID:  pipelineID,
		BuildNumber: buildNumber,
	}

	for _, r := range results {
		summary.TotalTests++
		if r.Passed {
			summary.PassedCount++
		} else {
			summary.FailedCount++
		}
	}

	// Classify failures
	summary.NovelFailures, summary.FlakyFailures = d.ClassifyFailures(ctx, pipelineID, results)

	return summary
}

// FormatFailureRate formats a failure rate as a human-readable string.
// e.g., 0.15 -> "3/20" if we know the window size
func FormatFailureRate(rate float64, totalRuns int) string {
	failed := int(rate * float64(totalRuns))
	return formatFraction(failed, totalRuns)
}

func formatFraction(num, denom int) string {
	if denom == 0 {
		return "0/0"
	}
	return string(rune('0'+num%10)) + "/" + string(rune('0'+denom%10))
}

// FormatFailureRateFull formats as "X/Y failures" for display.
func FormatFailureRateFull(rate float64, totalRuns int) string {
	failed := int(rate*float64(totalRuns) + 0.5) // Round
	if failed == 1 {
		return "1/" + itoa(totalRuns) + " failure"
	}
	return itoa(failed) + "/" + itoa(totalRuns) + " failures"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
