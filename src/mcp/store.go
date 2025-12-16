package mcp

import (
	"sync"

	"destill-agent/src/contracts"
)

// FindingsStore is the interface for storing and retrieving findings.
// Matches the postgres schema structure from connect.yaml.
type FindingsStore interface {
	// Store saves tiered findings for a request.
	Store(requestID string, response TieredResponse)
	// Get retrieves a single finding by message hash.
	Get(requestID, messageHash string) (Finding, bool)
	// GetAll retrieves the full tiered response for a request.
	GetAll(requestID string) (TieredResponse, bool)
}

// InMemoryStore is a thread-safe in-memory implementation of FindingsStore.
// For SaaS, this will be replaced with postgres queries keyed on request_id/message_hash.
type InMemoryStore struct {
	mu       sync.RWMutex
	requests map[string]TieredResponse           // request_id -> full response
	findings map[string]map[string]Finding       // request_id -> message_hash -> finding
}

// NewInMemoryStore creates a new in-memory findings store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		requests: make(map[string]TieredResponse),
		findings: make(map[string]map[string]Finding),
	}
}

// Store saves tiered findings, indexed by message hash for drill-down.
func (s *InMemoryStore) Store(requestID string, response TieredResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.requests[requestID] = response

	// Build lookup index by message_hash
	findingsMap := make(map[string]Finding)
	for _, f := range response.Tier1UniqueFailures {
		findingsMap[f.ID] = f
	}
	for _, f := range response.Tier2FrequencySpikes {
		findingsMap[f.ID] = f
	}
	for _, f := range response.Tier3CommonNoise {
		findingsMap[f.ID] = f
	}
	s.findings[requestID] = findingsMap
}

// Get retrieves a finding by message hash (used as finding ID).
func (s *InMemoryStore) Get(requestID, messageHash string) (Finding, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if findingsMap, ok := s.findings[requestID]; ok {
		f, found := findingsMap[messageHash]
		return f, found
	}
	return Finding{}, false
}

// GetAll retrieves the full tiered response.
func (s *InMemoryStore) GetAll(requestID string) (TieredResponse, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.requests[requestID]
	return r, ok
}

// ExtractRequestID extracts the request_id from a build URL or generates one.
// In postgres, this maps to the request_id column used for partitioning.
func ExtractRequestID(cards []contracts.TriageCard) string {
	if len(cards) > 0 && cards[0].RequestID != "" {
		return cards[0].RequestID
	}
	return ""
}
