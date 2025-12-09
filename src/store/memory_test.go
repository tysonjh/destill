package store

import (
	"context"
	"testing"

	"destill-agent/src/contracts"
)

func TestMemoryStore_CreateAndGetRequest(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	requestID := "test-req-123"
	buildURL := "https://buildkite.com/org/pipeline/builds/123"

	// Create request
	if err := store.CreateRequest(ctx, requestID, buildURL); err != nil {
		t.Fatalf("CreateRequest failed: %v", err)
	}

	// Get request status
	status, err := store.GetRequestStatus(ctx, requestID)
	if err != nil {
		t.Fatalf("GetRequestStatus failed: %v", err)
	}

	if status.RequestID != requestID {
		t.Errorf("Expected request ID %s, got %s", requestID, status.RequestID)
	}
	if status.BuildURL != buildURL {
		t.Errorf("Expected build URL %s, got %s", buildURL, status.BuildURL)
	}
	if status.Status != "pending" {
		t.Errorf("Expected status 'pending', got %s", status.Status)
	}
}

func TestMemoryStore_UpdateRequestStatus(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	requestID := "test-req-456"

	// Create request
	store.CreateRequest(ctx, requestID, "https://example.com")

	// Update status
	newStatus := &contracts.RequestStatus{
		RequestID:       requestID,
		Status:          "completed",
		ChunksTotal:     10,
		ChunksProcessed: 10,
		FindingsCount:   5,
	}

	if err := store.UpdateRequestStatus(ctx, newStatus); err != nil {
		t.Fatalf("UpdateRequestStatus failed: %v", err)
	}

	// Verify update
	status, err := store.GetRequestStatus(ctx, requestID)
	if err != nil {
		t.Fatalf("GetRequestStatus failed: %v", err)
	}

	if status.Status != "completed" {
		t.Errorf("Expected status 'completed', got %s", status.Status)
	}
	if status.ChunksTotal != 10 {
		t.Errorf("Expected chunks total 10, got %d", status.ChunksTotal)
	}
	if status.FindingsCount != 5 {
		t.Errorf("Expected findings count 5, got %d", status.FindingsCount)
	}
}

func TestMemoryStore_SaveAndGetFindings(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()
	requestID := "test-req-789"

	// Save findings
	finding1 := &contracts.TriageCardV2{
		ID:              "finding-1",
		RequestID:       requestID,
		MessageHash:     "hash1",
		Severity:        "ERROR",
		ConfidenceScore: 0.9,
		RawMessage:      "Error occurred",
	}

	finding2 := &contracts.TriageCardV2{
		ID:              "finding-2",
		RequestID:       requestID,
		MessageHash:     "hash2",
		Severity:        "FATAL",
		ConfidenceScore: 0.95,
		RawMessage:      "Fatal error",
	}

	if err := store.SaveFinding(ctx, finding1); err != nil {
		t.Fatalf("SaveFinding 1 failed: %v", err)
	}
	if err := store.SaveFinding(ctx, finding2); err != nil {
		t.Fatalf("SaveFinding 2 failed: %v", err)
	}

	// Get findings
	findings, err := store.GetFindings(ctx, requestID)
	if err != nil {
		t.Fatalf("GetFindings failed: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("Expected 2 findings, got %d", len(findings))
	}

	// Verify findings
	if findings[0].ID != "finding-1" {
		t.Errorf("Expected finding ID 'finding-1', got %s", findings[0].ID)
	}
	if findings[1].ID != "finding-2" {
		t.Errorf("Expected finding ID 'finding-2', got %s", findings[1].ID)
	}
}

func TestMemoryStore_GetNonExistentRequest(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	_, err := store.GetRequestStatus(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent request")
	}
}

func TestMemoryStore_GetFindingsEmptyRequest(t *testing.T) {
	store := NewMemoryStore()
	defer store.Close()

	ctx := context.Background()

	findings, err := store.GetFindings(ctx, "non-existent")
	if err != nil {
		t.Fatalf("GetFindings failed: %v", err)
	}

	if len(findings) != 0 {
		t.Errorf("Expected 0 findings for non-existent request, got %d", len(findings))
	}
}

