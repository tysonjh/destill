// Package broker provides adapters for legacy interface compatibility.
package broker

import (
	"context"
)

// LegacyAdapter adapts the new Broker interface to the old MessageBroker interface.
// This allows legacy code to continue working while using the new broker implementation.
type LegacyAdapter struct {
	broker Broker
	ctx    context.Context
}

// NewLegacyAdapter creates an adapter that wraps a new Broker for legacy code.
func NewLegacyAdapter(broker Broker) *LegacyAdapter {
	return &LegacyAdapter{
		broker: broker,
		ctx:    context.Background(),
	}
}

// Publish implements the legacy MessageBroker.Publish interface.
func (a *LegacyAdapter) Publish(topic string, message []byte) error {
	// Use empty key for legacy compatibility
	return a.broker.Publish(a.ctx, topic, "", message)
}

// Subscribe implements the legacy MessageBroker.Subscribe interface.
func (a *LegacyAdapter) Subscribe(topic string) (<-chan []byte, error) {
	// Use a default group ID for legacy compatibility
	msgChan, err := a.broker.Subscribe(a.ctx, topic, "legacy-consumer")
	if err != nil {
		return nil, err
	}

	// Convert Message channel to []byte channel
	legacyChan := make(chan []byte, 100)

	go func() {
		defer close(legacyChan)
		for msg := range msgChan {
			select {
			case legacyChan <- msg.Value:
			case <-a.ctx.Done():
				return
			}
		}
	}()

	return legacyChan, nil
}

// Close implements the legacy MessageBroker.Close interface.
func (a *LegacyAdapter) Close() error {
	return a.broker.Close()
}

