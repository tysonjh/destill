package store

import (
	"context"
	"errors"
	"testing"

	"destill-agent/src/contracts"
)

func TestInMemoryStore(t *testing.T) {
	st := NewInMemoryStore()
	ctx := context.Background()

	// Create test cards
	cards := []contracts.TriageCard{
		{
			RequestID:       "req-123",
			MessageHash:     "hash-1",
			RawMessage:      "error 1",
			NormalizedMsg:   "error 1",
			Severity:        "ERROR",
			ConfidenceScore: 0.9,
			JobName:         "job-1",
		},
		{
			RequestID:       "req-123",
			MessageHash:     "hash-2",
			RawMessage:      "error 2",
			NormalizedMsg:   "error 2",
			Severity:        "ERROR",
			ConfidenceScore: 0.8,
			JobName:         "job-1",
		},
	}

	// Test Store
	err := st.Store(ctx, "req-123", cards)
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Test GetFindings
	found, err := st.GetFindings(ctx, "req-123")
	if err != nil {
		t.Fatalf("GetFindings() error = %v", err)
	}
	if len(found) != 2 {
		t.Errorf("GetFindings() returned %d cards, want 2", len(found))
	}

	// Test GetFindings - not found
	_, err = st.GetFindings(ctx, "req-999")
	var notFound ErrNotFound
	if !errors.As(err, &notFound) {
		t.Errorf("GetFindings() for missing request should return ErrNotFound, got %v", err)
	}

	// Test GetByHash - found
	card, err := st.GetByHash(ctx, "req-123", "hash-1")
	if err != nil {
		t.Fatalf("GetByHash() error = %v", err)
	}
	if card.RawMessage != "error 1" {
		t.Errorf("GetByHash() message = %q, want %q", card.RawMessage, "error 1")
	}

	// Test GetByHash - wrong request
	_, err = st.GetByHash(ctx, "req-999", "hash-1")
	if !errors.As(err, &notFound) {
		t.Errorf("GetByHash() for missing request should return ErrNotFound, got %v", err)
	}

	// Test GetByHash - wrong hash
	_, err = st.GetByHash(ctx, "req-123", "hash-999")
	if !errors.As(err, &notFound) {
		t.Errorf("GetByHash() for missing hash should return ErrNotFound, got %v", err)
	}

	// Test Close
	if err := st.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestErrNotFound(t *testing.T) {
	// Test with only request ID
	err := ErrNotFound{RequestID: "req-123"}
	if err.Error() != "request not found: req-123" {
		t.Errorf("ErrNotFound.Error() = %q, want %q", err.Error(), "request not found: req-123")
	}

	// Test with request ID and message hash
	err = ErrNotFound{RequestID: "req-123", MessageHash: "hash-456"}
	if err.Error() != "finding not found: request_id=req-123, message_hash=hash-456" {
		t.Errorf("ErrNotFound.Error() = %q, want %q", err.Error(), "finding not found: request_id=req-123, message_hash=hash-456")
	}
}
