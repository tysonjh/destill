// Package pipeline provides the legacy in-memory pipeline implementation.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

// LegacyPipeline implements Pipeline using in-memory broker.
// This is the current behavior - all processing in one process.
// Note: This is a stub for Phase 2. Full integration with agents happens in Phase 6.
type LegacyPipeline struct {
	broker         *broker.InMemoryBroker
	buildkiteToken string
}

// NewLegacyPipeline creates a new legacy pipeline.
// Note: Agent integration will be added in Phase 6.
func NewLegacyPipeline(cfg *Config) (*LegacyPipeline, error) {
	// Create in-memory broker
	memBroker := broker.NewInMemoryBroker()

	return &LegacyPipeline{
		broker:         memBroker,
		buildkiteToken: cfg.BuildkiteToken,
	}, nil
}

// Submit submits a build URL for analysis.
func (p *LegacyPipeline) Submit(ctx context.Context, buildURL string) (string, error) {
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	request := struct {
		RequestID string `json:"request_id"`
		BuildURL  string `json:"build_url"`
	}{
		RequestID: requestID,
		BuildURL:  buildURL,
	}

	data, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Publish to destill_requests topic (legacy topic name)
	if err := p.broker.Publish(ctx, "destill_requests", requestID, data); err != nil {
		return "", fmt.Errorf("failed to publish request: %w", err)
	}

	return requestID, nil
}

// Status returns the current status of a request.
// For legacy mode, status tracking is limited.
func (p *LegacyPipeline) Status(ctx context.Context, requestID string) (*contracts.RequestStatus, error) {
	// Legacy mode doesn't track status - return a basic response
	return &contracts.RequestStatus{
		RequestID: requestID,
		Status:    "processing",
	}, nil
}

// Stream returns a channel of findings for streaming to TUI.
func (p *LegacyPipeline) Stream(ctx context.Context, requestID string) (<-chan contracts.TriageCard, error) {
	// Subscribe to ci_failures_ranked topic (legacy topic name)
	msgChan, err := p.broker.Subscribe(ctx, "ci_failures_ranked", "tui-consumer")
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to findings: %w", err)
	}

	// Convert Message channel to TriageCard channel
	cardChan := make(chan contracts.TriageCard, 100)

	go func() {
		defer close(cardChan)
		for {
			select {
			case msg, ok := <-msgChan:
				if !ok {
					return
				}

				var card contracts.TriageCard
				if err := json.Unmarshal(msg.Value, &card); err != nil {
					fmt.Printf("[LegacyPipeline] Failed to unmarshal triage card: %v\n", err)
					continue
				}

				select {
				case cardChan <- card:
				case <-ctx.Done():
					return
				}

			case <-ctx.Done():
				return
			}
		}
	}()

	return cardChan, nil
}

// Close shuts down the pipeline.
func (p *LegacyPipeline) Close() error {
	return p.broker.Close()
}

