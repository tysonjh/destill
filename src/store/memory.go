// Package store provides an in-memory store implementation.
package store

import (
	"context"
	"fmt"
	"sync"

	"destill-agent/src/contracts"
)

// MemoryStore is an in-memory implementation of Store.
// Useful for testing and legacy mode.
type MemoryStore struct {
	mu       sync.RWMutex
	requests map[string]*contracts.RequestStatus
	findings map[string][]contracts.TriageCardV2 // requestID -> findings
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		requests: make(map[string]*contracts.RequestStatus),
		findings: make(map[string][]contracts.TriageCardV2),
	}
}

// CreateRequest creates a new analysis request record.
func (s *MemoryStore) CreateRequest(ctx context.Context, requestID string, buildURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests[requestID] = &contracts.RequestStatus{
		RequestID: requestID,
		BuildURL:  buildURL,
		Status:    "pending",
	}

	return nil
}

// GetRequestStatus returns the status of a request.
func (s *MemoryStore) GetRequestStatus(ctx context.Context, requestID string) (*contracts.RequestStatus, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status, exists := s.requests[requestID]
	if !exists {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	// Return a copy
	statusCopy := *status
	return &statusCopy, nil
}

// UpdateRequestStatus updates the status of a request.
func (s *MemoryStore) UpdateRequestStatus(ctx context.Context, status *contracts.RequestStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.requests[status.RequestID]; !exists {
		return fmt.Errorf("request not found: %s", status.RequestID)
	}

	s.requests[status.RequestID] = status
	return nil
}

// SaveFinding saves a single finding.
func (s *MemoryStore) SaveFinding(ctx context.Context, finding *contracts.TriageCardV2) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.findings[finding.RequestID] = append(s.findings[finding.RequestID], *finding)
	return nil
}

// GetFindings retrieves all findings for a request.
func (s *MemoryStore) GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCardV2, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	findings, exists := s.findings[requestID]
	if !exists {
		return []contracts.TriageCardV2{}, nil
	}

	// Return a copy
	result := make([]contracts.TriageCardV2, len(findings))
	copy(result, findings)
	return result, nil
}

// Close closes the store (no-op for memory store).
func (s *MemoryStore) Close() error {
	return nil
}

