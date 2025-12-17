// Package store defines the interface for persistent data storage.
package store

import (
	"context"
	"sync"

	"destill-agent/src/contracts"
)

// InMemoryStore is a thread-safe in-memory implementation of Store.
// Used for local mode and MCP server.
type InMemoryStore struct {
	mu       sync.RWMutex
	requests map[string][]contracts.TriageCard         // request_id -> cards
	byHash   map[string]map[string]contracts.TriageCard // request_id -> message_hash -> card
}

// NewInMemoryStore creates a new in-memory store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		requests: make(map[string][]contracts.TriageCard),
		byHash:   make(map[string]map[string]contracts.TriageCard),
	}
}

// Store saves findings for a request, indexed by message hash for lookup.
func (s *InMemoryStore) Store(ctx context.Context, requestID string, cards []contracts.TriageCard) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests[requestID] = cards

	// Build lookup index by message_hash
	hashMap := make(map[string]contracts.TriageCard)
	for _, card := range cards {
		hashMap[card.MessageHash] = card
	}
	s.byHash[requestID] = hashMap

	return nil
}

// GetFindings retrieves all findings for a request.
func (s *InMemoryStore) GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cards, ok := s.requests[requestID]
	if !ok {
		return nil, ErrNotFound{RequestID: requestID}
	}
	return cards, nil
}

// GetByHash retrieves a single finding by message hash.
func (s *InMemoryStore) GetByHash(ctx context.Context, requestID, messageHash string) (contracts.TriageCard, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	hashMap, ok := s.byHash[requestID]
	if !ok {
		return contracts.TriageCard{}, ErrNotFound{RequestID: requestID, MessageHash: messageHash}
	}

	card, ok := hashMap[messageHash]
	if !ok {
		return contracts.TriageCard{}, ErrNotFound{RequestID: requestID, MessageHash: messageHash}
	}

	return card, nil
}

// Close is a no-op for in-memory store.
func (s *InMemoryStore) Close() error {
	return nil
}
