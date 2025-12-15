// Package broker defines the interface for message brokers and provides implementations.
package broker

import "context"

// Broker abstracts message publishing and consumption.
// This interface supports both in-memory (legacy) and distributed (Redpanda/Kafka) implementations.
type Broker interface {
	// Publish sends a message to a topic with an optional key for partitioning.
	// For in-memory broker, key is ignored.
	// For Redpanda/Kafka, key is used for partition assignment.
	Publish(ctx context.Context, topic string, key string, value []byte) error

	// Subscribe returns a channel for consuming messages from a topic.
	// groupID is used for consumer group coordination in Kafka.
	// For in-memory broker, groupID is ignored.
	Subscribe(ctx context.Context, topic string, groupID string) (<-chan Message, error)

	// Close shuts down the broker connection gracefully.
	Close() error
}

// Message represents a consumed message from a broker.
type Message struct {
	Topic     string
	Key       string
	Value     []byte
	Offset    int64
	Partition int32
	Timestamp int64
}
