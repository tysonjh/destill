package tui

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

func TestTUI_ReceivesTestResults(t *testing.T) {
	// Create in-memory broker
	msgBroker := broker.NewInMemoryBroker()
	ctx := context.Background()

	// Subscribe BEFORE publishing (like the TUI does)
	channels, err := SubscribeToBroker(msgBroker)
	if err != nil {
		t.Fatalf("SubscribeToBroker failed: %v", err)
	}
	defer channels.Cancel()

	// Simulate publishing test results (like ArtifactAgent does)
	testResults := []contracts.TestResult{
		{
			RequestID:      "req-123",
			PipelineID:     "test/pipeline",
			BuildNumber:    100,
			JobName:        "test-job",
			TestName:       "TestSomething",
			ClassName:      "com.example.Test",
			Passed:         true,
			Duration:       1.5,
		},
		{
			RequestID:      "req-123",
			PipelineID:     "test/pipeline",
			BuildNumber:    100,
			JobName:        "test-job",
			TestName:       "TestFailing",
			ClassName:      "com.example.Test",
			Passed:         false,
			FailureMessage: "Expected 1 but got 2",
			Duration:       0.5,
		},
	}

	// Publish test results
	for _, result := range testResults {
		data, _ := json.Marshal(result)
		if err := msgBroker.Publish(ctx, contracts.TopicTestResults, result.RequestID, data); err != nil {
			t.Fatalf("Publish failed: %v", err)
		}
	}

	// Verify we receive them
	received := 0
	timeout := time.After(1 * time.Second)

	for received < len(testResults) {
		select {
		case msg := <-channels.TestResultsChan:
			var result contracts.TestResult
			if err := json.Unmarshal(msg.Value, &result); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			received++
			t.Logf("Received test result: %s (passed=%v)", result.TestName, result.Passed)
		case <-timeout:
			t.Fatalf("Timeout waiting for test results. Received %d of %d", received, len(testResults))
		}
	}

	if received != len(testResults) {
		t.Errorf("Expected %d test results, got %d", len(testResults), received)
	}
}

func TestUpdateTestSummary(t *testing.T) {
	styles := DefaultStyles()
	model := MainModel{
		styles:       styles,
		summaryModel: NewSummaryModel(styles),
		testResults:  []contracts.TestResult{},
	}

	// Add some test results
	model.testResults = []contracts.TestResult{
		{TestName: "Test1", Passed: true},
		{TestName: "Test2", Passed: true},
		{TestName: "Test3", Passed: false, FailureMessage: "failed"},
		{TestName: "Test4", Passed: false, FailureMessage: "also failed"},
	}

	// Update summary
	model.updateTestSummary()

	// Verify summary model was updated
	if !model.summaryModel.hasTestResults {
		t.Error("Expected hasTestResults to be true")
	}

	summary := model.summaryModel.testSummary
	if summary == nil {
		t.Fatal("Expected testSummary to be set")
	}

	if summary.TotalTests != 4 {
		t.Errorf("Expected TotalTests=4, got %d", summary.TotalTests)
	}
	if summary.PassedCount != 2 {
		t.Errorf("Expected PassedCount=2, got %d", summary.PassedCount)
	}
	if summary.FailedCount != 2 {
		t.Errorf("Expected FailedCount=2, got %d", summary.FailedCount)
	}
	if len(summary.NovelFailures) != 2 {
		t.Errorf("Expected 2 NovelFailures, got %d", len(summary.NovelFailures))
	}
}
