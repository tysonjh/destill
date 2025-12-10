// Package store defines the interface for persistent data storage.
package store

import (
	"context"
	"destill-agent/src/contracts"
)

// Store defines the interface for persisting findings and request status.
type Store interface {
	// CreateRequest creates a new analysis request record
	CreateRequest(ctx context.Context, requestID string, buildURL string) error

	// GetRequestStatus returns the status of a request
	GetRequestStatus(ctx context.Context, requestID string) (*contracts.RequestStatus, error)

	// UpdateRequestStatus updates the status of a request
	UpdateRequestStatus(ctx context.Context, status *contracts.RequestStatus) error

	// SaveFinding saves a single finding
	SaveFinding(ctx context.Context, finding *contracts.TriageCardV2) error

	// GetFindings retrieves all findings for a request
	GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCardV2, error)

	// Close closes the store connection
	Close() error
}

