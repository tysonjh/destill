// Package broker provides implementations of the MessageBroker interface.
package broker

import (
	"fmt"

	"destill-agent/src/contracts"
)

// InMemoryBroker is a simple in-memory implementation of MessageBroker.
type InMemoryBroker struct {
	handlers map[string][]func(contracts.TriageCard) error
	verbose  bool
}

// NewInMemoryBroker creates a new InMemoryBroker instance.
func NewInMemoryBroker() *InMemoryBroker {
	return &InMemoryBroker{
		handlers: make(map[string][]func(contracts.TriageCard) error),
		verbose:  false,
	}
}

// SetVerbose enables or disables verbose logging.
func (b *InMemoryBroker) SetVerbose(verbose bool) {
	b.verbose = verbose
}

// Publish sends a TriageCard to all handlers subscribed to the topic.
func (b *InMemoryBroker) Publish(topic string, card contracts.TriageCard) error {
	if handlers, ok := b.handlers[topic]; ok {
		for _, handler := range handlers {
			if err := handler(card); err != nil {
				return err
			}
		}
	}
	if b.verbose {
		fmt.Printf("[InMemoryBroker] Published to topic '%s': %+v\n", topic, card)
	}
	return nil
}

// Subscribe registers a handler for the specified topic.
func (b *InMemoryBroker) Subscribe(topic string, handler func(contracts.TriageCard) error) error {
	b.handlers[topic] = append(b.handlers[topic], handler)
	return nil
}

// Close is a no-op for the in-memory broker.
func (b *InMemoryBroker) Close() error {
	return nil
}
