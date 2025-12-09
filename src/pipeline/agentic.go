// Package pipeline provides the agentic distributed pipeline implementation.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/store"
)

// AgenticPipeline implements Pipeline using Redpanda + Postgres.
// This is the new distributed architecture.
type AgenticPipeline struct {
	broker broker.Broker
	store  store.Store
}

// NewAgenticPipeline creates a new agentic pipeline.
func NewAgenticPipeline(cfg *Config) (*AgenticPipeline, error) {
	// Create Redpanda broker
	redpandaBroker, err := broker.NewRedpandaBroker(cfg.RedpandaBrokers)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redpanda broker: %w", err)
	}

	// Create Postgres store
	postgresStore, err := store.NewPostgresStore(cfg.PostgresDSN)
	if err != nil {
		redpandaBroker.Close()
		return nil, fmt.Errorf("failed to create Postgres store: %w", err)
	}

	return &AgenticPipeline{
		broker: redpandaBroker,
		store:  postgresStore,
	}, nil
}

// Submit submits a build URL for analysis.
func (p *AgenticPipeline) Submit(ctx context.Context, buildURL string) (string, error) {
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	request := contracts.AnalysisRequest{
		RequestID: requestID,
		BuildURL:  buildURL,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Publish to destill.requests topic
	if err := p.broker.Publish(ctx, contracts.TopicRequests, requestID, data); err != nil {
		return "", fmt.Errorf("failed to publish request: %w", err)
	}

	// Create request record in Postgres
	if err := p.store.CreateRequest(ctx, requestID, buildURL); err != nil {
		return "", fmt.Errorf("failed to create request record: %w", err)
	}

	return requestID, nil
}

// Status returns the current status of a request from Postgres.
func (p *AgenticPipeline) Status(ctx context.Context, requestID string) (*contracts.RequestStatus, error) {
	return p.store.GetRequestStatus(ctx, requestID)
}

// Stream returns nil for agentic mode - use Store.GetFindings instead.
// Agentic mode doesn't support streaming; findings are queried from Postgres.
func (p *AgenticPipeline) Stream(ctx context.Context, requestID string) (<-chan contracts.TriageCard, error) {
	return nil, fmt.Errorf("streaming not supported in agentic mode - use Store.GetFindings instead")
}

// Close shuts down the pipeline.
func (p *AgenticPipeline) Close() error {
	if err := p.broker.Close(); err != nil {
		return err
	}
	return p.store.Close()
}

