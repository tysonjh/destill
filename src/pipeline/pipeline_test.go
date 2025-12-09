package pipeline

import (
	"testing"
	
	"destill-agent/src/contracts"
)

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected Mode
	}{
		{
			name: "Legacy mode - no brokers",
			config: &Config{
				RedpandaBrokers: []string{},
				BuildkiteToken:  "test-token",
			},
			expected: LegacyMode,
		},
		{
			name: "Legacy mode - nil brokers",
			config: &Config{
				RedpandaBrokers: nil,
				BuildkiteToken:  "test-token",
			},
			expected: LegacyMode,
		},
		{
			name: "Agentic mode - with brokers",
			config: &Config{
				RedpandaBrokers: []string{"localhost:19092"},
				PostgresDSN:     "postgres://user:pass@localhost/db",
				BuildkiteToken:  "test-token",
			},
			expected: AgenticMode,
		},
		{
			name: "Agentic mode - multiple brokers",
			config: &Config{
				RedpandaBrokers: []string{"broker1:9092", "broker2:9092"},
				PostgresDSN:     "postgres://user:pass@localhost/db",
				BuildkiteToken:  "test-token",
			},
			expected: AgenticMode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode := DetectMode(tt.config)
			if mode != tt.expected {
				t.Errorf("Expected mode %v, got %v", tt.expected, mode)
			}
		})
	}
}

func TestRequestStatus(t *testing.T) {
	status := &contracts.RequestStatus{
		RequestID:       "req-123",
		BuildURL:        "https://example.com/build/123",
		Status:          "processing",
		ChunksTotal:     10,
		ChunksProcessed: 5,
		FindingsCount:   3,
	}

	if status.RequestID != "req-123" {
		t.Errorf("Expected request ID 'req-123', got %s", status.RequestID)
	}
	if status.Status != "processing" {
		t.Errorf("Expected status 'processing', got %s", status.Status)
	}
	if status.ChunksTotal != 10 {
		t.Errorf("Expected chunks total 10, got %d", status.ChunksTotal)
	}
	if status.ChunksProcessed != 5 {
		t.Errorf("Expected chunks processed 5, got %d", status.ChunksProcessed)
	}
	if status.FindingsCount != 3 {
		t.Errorf("Expected findings count 3, got %d", status.FindingsCount)
	}
}

