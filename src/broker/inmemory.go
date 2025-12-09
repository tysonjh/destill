// Package broker provides implementations of the Broker interface.
package broker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemoryBroker is a channel-based implementation of Broker.
// It simulates a Kafka-like stream for local development.
type InMemoryBroker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Message
	verbose     bool
	closed      bool
}

// NewInMemoryBroker creates a new InMemoryBroker instance.
func NewInMemoryBroker() *InMemoryBroker {
	return &InMemoryBroker{
		subscribers: make(map[string][]chan Message),
		verbose:     false,
		closed:      false,
	}
}

// SetVerbose enables or disables verbose logging.
func (b *InMemoryBroker) SetVerbose(verbose bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.verbose = verbose
}

// Publish sends a message to all subscribers of the specified topic.
// Implements the Broker interface.
func (b *InMemoryBroker) Publish(ctx context.Context, topic string, key string, value []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker is closed")
	}

	if b.verbose {
		fmt.Printf("[InMemoryBroker] Publishing to topic '%s': %d bytes (key: %s)\n", topic, len(value), key)
	}

	msg := Message{
		Topic:     topic,
		Key:       key,
		Value:     value,
		Offset:    0, // Not tracked in memory
		Partition: 0, // Single partition in memory
		Timestamp: time.Now().UnixMilli(),
	}

	if channels, ok := b.subscribers[topic]; ok {
		for _, ch := range channels {
			select {
			case ch <- msg:
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Channel buffer full - log warning but continue
				// This is acceptable for local development; production would use backpressure
				if b.verbose {
					fmt.Printf("[InMemoryBroker] Warning: channel buffer full for topic '%s', message dropped\n", topic)
				}
			}
		}
	}

	return nil
}

// Subscribe creates and returns a channel for receiving messages from the specified topic.
// Implements the Broker interface. groupID is ignored for in-memory broker.
func (b *InMemoryBroker) Subscribe(ctx context.Context, topic string, groupID string) (<-chan Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("broker is closed")
	}

	// Create a buffered channel for this subscriber
	ch := make(chan Message, 100)
	b.subscribers[topic] = append(b.subscribers[topic], ch)

	if b.verbose {
		fmt.Printf("[InMemoryBroker] New subscriber for topic '%s' (group: %s)\n", topic, groupID)
	}

	return ch, nil
}

// Close shuts down the broker and closes all subscriber channels.
func (b *InMemoryBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all subscriber channels
	for topic, channels := range b.subscribers {
		for _, ch := range channels {
			close(ch)
		}
		delete(b.subscribers, topic)
	}

	return nil
}
