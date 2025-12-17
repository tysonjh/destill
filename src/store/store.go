// Package store defines the interface for persistent data storage.
package store

import (
	"context"

	"destill-agent/src/contracts"
)

// Store defines the unified interface for findings storage.
// TriageCard is the canonical data model. Tiering happens at read-time.
//
// Implementations:
//   - InMemoryStore: For local mode and MCP server
//   - PostgresStore: For distributed mode with Kafka sink
type Store interface {
	// GetFindings retrieves all findings for a request.
	GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCard, error)

	// GetByHash retrieves a single finding by message hash.
	GetByHash(ctx context.Context, requestID, messageHash string) (contracts.TriageCard, error)

	// Store saves findings for a request.
	Store(ctx context.Context, requestID string, cards []contracts.TriageCard) error

	// Close closes the store connection.
	Close() error
}

// ErrNotFound is returned when a finding is not found.
type ErrNotFound struct {
	RequestID   string
	MessageHash string
}

func (e ErrNotFound) Error() string {
	if e.MessageHash != "" {
		return "finding not found: request_id=" + e.RequestID + ", message_hash=" + e.MessageHash
	}
	return "request not found: " + e.RequestID
}
