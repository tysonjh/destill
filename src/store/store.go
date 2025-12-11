// Package store defines the interface for persistent data storage.
package store

import (
	"context"
	"destill-agent/src/contracts"
)

// Store defines the interface for querying persisted findings.
// Note: Findings are persisted via Redpanda Connect (Kafka → Postgres sink),
// not through this interface. This interface is for read operations only.
type Store interface {
	// GetFindings retrieves all findings for a request
	GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCard, error)

	// Close closes the store connection
	Close() error
}
