package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"destill-agent/src/contracts"
)

// TestValidateBuildURL tests URL validation
func TestValidateBuildURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid buildkite URL",
			url:     "https://buildkite.com/myorg/pipeline/builds/123",
			wantErr: false,
		},
		{
			name:    "valid github actions URL",
			url:     "https://github.com/owner/repo/actions/runs/123456",
			wantErr: false,
		},
		{
			name:    "invalid URL - not a URL",
			url:     "not-a-url",
			wantErr: true,
		},
		{
			name:    "invalid URL - wrong domain",
			url:     "https://example.com/builds/123",
			wantErr: true,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBuildURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBuildURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGenerateRequestID tests request ID generation
func TestGenerateRequestID(t *testing.T) {
	// Generate multiple IDs
	ids := make([]string, 100)
	for i := 0; i < 100; i++ {
		ids[i] = generateRequestID()
	}

	// Test 1: All IDs should be unique
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("generateRequestID() produced duplicate ID: %s", id)
		}
		seen[id] = true
	}

	// Test 2: ID format should match req-YYYYMMDDTHHmmss-XXXXXXXX
	for _, id := range ids {
		if !strings.HasPrefix(id, "req-") {
			t.Errorf("generateRequestID() ID doesn't start with 'req-': %s", id)
		}

		parts := strings.Split(id, "-")
		if len(parts) != 3 {
			t.Errorf("generateRequestID() ID doesn't have 3 parts: %s", id)
		}

		// Check timestamp format (YYYYMMDDTHHmmss = 15 chars)
		timestamp := parts[1]
		if len(timestamp) != 15 {
			t.Errorf("generateRequestID() timestamp wrong length: %s", timestamp)
		}

		// Check random suffix (8 hex chars)
		suffix := parts[2]
		if len(suffix) != 8 {
			t.Errorf("generateRequestID() suffix wrong length: %s", suffix)
		}
	}

	// Test 3: IDs should be sortable (lexicographically ordered by time)
	id1 := generateRequestID()
	time.Sleep(1 * time.Second) // Ensure different timestamps (need 1 second for YYYYMMDDTHHMMSS format)
	id2 := generateRequestID()

	if id1 >= id2 {
		t.Errorf("generateRequestID() IDs not sortable: %s >= %s", id1, id2)
	}
}

// TestCreateAnalysisRequest tests request creation
func TestCreateAnalysisRequest(t *testing.T) {
	buildURL := "https://buildkite.com/myorg/pipeline/builds/123"

	request, err := createAnalysisRequest(buildURL)
	if err != nil {
		t.Fatalf("createAnalysisRequest() unexpected error: %v", err)
	}

	// Test 1: Request ID should be set
	if request.RequestID == "" {
		t.Error("createAnalysisRequest() RequestID is empty")
	}

	// Test 2: Build URL should match
	if request.BuildURL != buildURL {
		t.Errorf("createAnalysisRequest() BuildURL = %s, want %s", request.BuildURL, buildURL)
	}

	// Test 3: Data should be valid JSON
	var payload struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}
	if err := json.Unmarshal(request.Data, &payload); err != nil {
		t.Fatalf("createAnalysisRequest() Data is not valid JSON: %v", err)
	}

	// Test 4: JSON payload should match request fields
	if payload.RequestID != request.RequestID {
		t.Errorf("createAnalysisRequest() JSON RequestID = %s, want %s", payload.RequestID, request.RequestID)
	}
	if payload.BuildURL != request.BuildURL {
		t.Errorf("createAnalysisRequest() JSON BuildURL = %s, want %s", payload.BuildURL, request.BuildURL)
	}
}

// TestLoadCachedCards tests cache loading
func TestLoadCachedCards(t *testing.T) {
	t.Run("empty cache file path", func(t *testing.T) {
		cards, err := loadCachedCards("")
		if err != nil {
			t.Errorf("loadCachedCards(\"\") unexpected error: %v", err)
		}
		if len(cards) != 0 {
			t.Errorf("loadCachedCards(\"\") returned %d cards, want 0", len(cards))
		}
	})

	t.Run("valid cache file", func(t *testing.T) {
		// Create temporary cache file
		tmpDir := t.TempDir()
		cacheFile := filepath.Join(tmpDir, "cache.json")

		testCards := []contracts.TriageCard{
			{
				ID:              "card-1",
				ConfidenceScore: 0.95,
				Severity:        "error",
				RawMessage:      "Test error 1",
			},
			{
				ID:              "card-2",
				ConfidenceScore: 0.85,
				Severity:        "warning",
				RawMessage:      "Test error 2",
			},
		}

		data, err := json.Marshal(testCards)
		if err != nil {
			t.Fatalf("Failed to marshal test data: %v", err)
		}

		if err := os.WriteFile(cacheFile, data, 0644); err != nil {
			t.Fatalf("Failed to write cache file: %v", err)
		}

		// Load the cache
		cards, err := loadCachedCards(cacheFile)
		if err != nil {
			t.Fatalf("loadCachedCards() unexpected error: %v", err)
		}

		if len(cards) != 2 {
			t.Errorf("loadCachedCards() returned %d cards, want 2", len(cards))
		}

		if cards[0].ID != "card-1" {
			t.Errorf("loadCachedCards() first card ID = %s, want card-1", cards[0].ID)
		}
	})

	t.Run("non-existent cache file", func(t *testing.T) {
		_, err := loadCachedCards("/path/does/not/exist.json")
		if err == nil {
			t.Error("loadCachedCards() expected error for non-existent file, got nil")
		}
	})

	t.Run("invalid JSON in cache file", func(t *testing.T) {
		tmpDir := t.TempDir()
		cacheFile := filepath.Join(tmpDir, "invalid.json")

		if err := os.WriteFile(cacheFile, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("Failed to write invalid cache file: %v", err)
		}

		_, err := loadCachedCards(cacheFile)
		if err == nil {
			t.Error("loadCachedCards() expected error for invalid JSON, got nil")
		}
	})
}

// TestSortCardsByPriority tests card sorting
func TestSortCardsByPriority(t *testing.T) {
	t.Run("sort by confidence score", func(t *testing.T) {
		cards := []contracts.TriageCard{
			{ID: "card-1", ConfidenceScore: 0.5},
			{ID: "card-2", ConfidenceScore: 0.9},
			{ID: "card-3", ConfidenceScore: 0.7},
		}

		sortCardsByPriority(cards)

		if cards[0].ID != "card-2" {
			t.Errorf("sortCardsByPriority() first card = %s, want card-2", cards[0].ID)
		}
		if cards[1].ID != "card-3" {
			t.Errorf("sortCardsByPriority() second card = %s, want card-3", cards[1].ID)
		}
		if cards[2].ID != "card-1" {
			t.Errorf("sortCardsByPriority() third card = %s, want card-1", cards[2].ID)
		}
	})

	t.Run("sort by recurrence count when confidence equal", func(t *testing.T) {
		cards := []contracts.TriageCard{
			{
				ID:              "card-1",
				ConfidenceScore: 0.9,
				Metadata:        map[string]string{"recurrence_count": "1"},
			},
			{
				ID:              "card-2",
				ConfidenceScore: 0.9,
				Metadata:        map[string]string{"recurrence_count": "5"},
			},
			{
				ID:              "card-3",
				ConfidenceScore: 0.9,
				Metadata:        map[string]string{"recurrence_count": "3"},
			},
		}

		sortCardsByPriority(cards)

		if cards[0].ID != "card-2" {
			t.Errorf("sortCardsByPriority() first card = %s, want card-2", cards[0].ID)
		}
		if cards[1].ID != "card-3" {
			t.Errorf("sortCardsByPriority() second card = %s, want card-3", cards[1].ID)
		}
		if cards[2].ID != "card-1" {
			t.Errorf("sortCardsByPriority() third card = %s, want card-1", cards[2].ID)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		cards := []contracts.TriageCard{}
		sortCardsByPriority(cards) // Should not panic
		if len(cards) != 0 {
			t.Errorf("sortCardsByPriority() modified empty slice")
		}
	})

	t.Run("single card", func(t *testing.T) {
		cards := []contracts.TriageCard{
			{ID: "card-1", ConfidenceScore: 0.5},
		}
		sortCardsByPriority(cards) // Should not panic
		if len(cards) != 1 || cards[0].ID != "card-1" {
			t.Errorf("sortCardsByPriority() modified single card slice")
		}
	})
}

// TestSetupLocalMode tests local mode setup (basic smoke test)
func TestSetupLocalMode(t *testing.T) {
	local, cleanup, err := setupLocalMode()
	if err != nil {
		t.Fatalf("setupLocalMode() unexpected error: %v", err)
	}
	defer cleanup()

	// Test 1: Broker should be initialized
	if local.broker == nil {
		t.Error("setupLocalMode() broker is nil")
	}

	// Test 2: Context should be initialized
	if local.ctx == nil {
		t.Error("setupLocalMode() context is nil")
	}

	// Test 3: Cleanup should not panic
	cleanup()
}

// BenchmarkGenerateRequestID benchmarks ID generation
func BenchmarkGenerateRequestID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateRequestID()
	}
}

// BenchmarkSortCardsByPriority benchmarks card sorting
func BenchmarkSortCardsByPriority(b *testing.B) {
	// Create test data
	cards := make([]contracts.TriageCard, 100)
	for i := 0; i < 100; i++ {
		cards[i] = contracts.TriageCard{
			ID:              string(rune(i)),
			ConfidenceScore: float64(i) / 100.0,
			Metadata:        map[string]string{"recurrence_count": "1"},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Make a copy for each iteration
		testCards := make([]contracts.TriageCard, len(cards))
		copy(testCards, cards)
		sortCardsByPriority(testCards)
	}
}
