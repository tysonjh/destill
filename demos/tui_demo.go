// Demo program to showcase the Destill TUI with sample data
package main

import (
	"fmt"
	"os"

	"destill-agent/src/contracts"
	"destill-agent/src/tui"
)

func main() {
	// Create sample triage cards with varied data
	cards := []contracts.TriageCard{
		{
			ID:              "card-001",
			Source:          "buildkite",
			Timestamp:       "2025-01-28T10:15:30Z",
			Severity:        "FATAL",
			Message:         "OutOfMemoryError: Java heap space at com.example.processor.DataHandler.process(DataHandler.java:142)",
			MessageHash:     "a7b3c9d2e1f4567890abcdef12345678",
			JobName:         "integration-tests",
			ConfidenceScore: 0.98,
			Metadata: map[string]string{
				"recurrence_count": "15",
				"build_number":     "4091",
				"organization":     "acme-corp",
				"pipeline":         "backend-api",
			},
		},
		{
			ID:              "card-002",
			Source:          "buildkite",
			Timestamp:       "2025-01-28T10:16:45Z",
			Severity:        "ERROR",
			Message:         "Connection timeout: failed to connect to database postgres://prod-db-01.example.com:5432 after 30s",
			MessageHash:     "f8e7d6c5b4a3928170fedcba98765432",
			JobName:         "e2e-tests",
			ConfidenceScore: 0.95,
			Metadata: map[string]string{
				"recurrence_count": "8",
				"build_number":     "4091",
				"organization":     "acme-corp",
				"pipeline":         "backend-api",
			},
		},
		{
			ID:              "card-003",
			Source:          "buildkite",
			Timestamp:       "2025-01-28T10:17:12Z",
			Severity:        "ERROR",
			Message:         "Test assertion failed: expected status code 200, got 503 in UserController.testCreateUser",
			MessageHash:     "1234567890abcdef1234567890abcdef",
			JobName:         "unit-tests",
			ConfidenceScore: 0.92,
			Metadata: map[string]string{
				"recurrence_count": "3",
				"build_number":     "4091",
				"organization":     "acme-corp",
				"pipeline":         "backend-api",
			},
		},
		{
			ID:              "card-004",
			Source:          "buildkite",
			Timestamp:       "2025-01-28T10:18:05Z",
			Severity:        "WARN",
			Message:         "Deprecated API usage detected: method 'getUserByEmail' will be removed in v3.0",
			MessageHash:     "abcdefabcdefabcdefabcdefabcdefab",
			JobName:         "lint-check",
			ConfidenceScore: 0.75,
			Metadata: map[string]string{
				"recurrence_count": "1",
				"build_number":     "4091",
				"organization":     "acme-corp",
				"pipeline":         "backend-api",
			},
		},
		{
			ID:              "card-005",
			Source:          "buildkite",
			Timestamp:       "2025-01-28T10:19:22Z",
			Severity:        "ERROR",
			Message:         "Compilation error: undefined reference to 'handleAuthCallback' in module authentication/oauth",
			MessageHash:     "9876543210fedcba9876543210fedcba",
			JobName:         "build",
			ConfidenceScore: 0.88,
			Metadata: map[string]string{
				"recurrence_count": "2",
				"build_number":     "4091",
				"organization":     "acme-corp",
				"pipeline":         "backend-api",
			},
		},
	}

	// Run the TUI program
	if err := tui.Start(cards); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}
