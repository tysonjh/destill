// Package broker provides implementations of the MessageBroker interface.
package broker

import (
	"fmt"
	"sync"
)

// InMemoryBroker is a channel-based implementation of MessageBroker.
// It simulates a Kafka-like stream for local development.
type InMemoryBroker struct {
	mu          sync.RWMutex
	subscribers map[string][]chan []byte
	verbose     bool
	closed      bool
}

// NewInMemoryBroker creates a new InMemoryBroker instance.
func NewInMemoryBroker() *InMemoryBroker {
	return &InMemoryBroker{
		subscribers: make(map[string][]chan []byte),
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
func (b *InMemoryBroker) Publish(topic string, message []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker is closed")
	}

	if b.verbose {
		fmt.Printf("[InMemoryBroker] Publishing to topic '%s': %d bytes\n", topic, len(message))
	}

	if channels, ok := b.subscribers[topic]; ok {
		for _, ch := range channels {
			select {
			case ch <- message:
			default:
				// Channel buffer full, skip to avoid blocking
				if b.verbose {
					fmt.Printf("[InMemoryBroker] Warning: channel buffer full for topic '%s'\n", topic)
				}
			}
		}
	}

	return nil
}

// Subscribe creates and returns a channel for receiving messages from the specified topic.
func (b *InMemoryBroker) Subscribe(topic string) (<-chan []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("broker is closed")
	}

	// Create a buffered channel for this subscriber
	ch := make(chan []byte, 100)
	b.subscribers[topic] = append(b.subscribers[topic], ch)

	if b.verbose {
		fmt.Printf("[InMemoryBroker] New subscriber for topic '%s'\n", topic)
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
