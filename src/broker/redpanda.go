// Package broker provides Redpanda/Kafka broker implementation.
package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"
)

// RedpandaBroker is a Kafka-compatible broker implementation using franz-go.
type RedpandaBroker struct {
	client    *kgo.Client
	brokers   []string
	mu        sync.RWMutex
	consumers map[string]*kgo.Client // topic+groupID -> consumer client
	closed    bool
}

// NewRedpandaBroker creates a new RedpandaBroker instance.
// brokers is a slice of broker addresses (e.g., ["localhost:19092"]).
func NewRedpandaBroker(brokers []string) (*RedpandaBroker, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("at least one broker address is required")
	}

	// Create producer client
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.AllowAutoTopicCreation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka client: %w", err)
	}

	return &RedpandaBroker{
		client:    client,
		brokers:   brokers,
		consumers: make(map[string]*kgo.Client),
		closed:    false,
	}, nil
}

// Publish sends a message to a topic with the specified key.
// Implements the Broker interface.
func (b *RedpandaBroker) Publish(ctx context.Context, topic string, key string, value []byte) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return fmt.Errorf("broker is closed")
	}

	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: value,
	}

	// Synchronous produce for simplicity
	results := b.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("failed to produce message: %w", err)
	}

	return nil
}

// Subscribe creates a consumer for the specified topic and consumer group.
// Returns a channel that will receive messages.
// Implements the Broker interface.
func (b *RedpandaBroker) Subscribe(ctx context.Context, topic string, groupID string) (<-chan Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, fmt.Errorf("broker is closed")
	}

	consumerKey := fmt.Sprintf("%s:%s", topic, groupID)

	// Check if consumer already exists
	if _, exists := b.consumers[consumerKey]; exists {
		return nil, fmt.Errorf("consumer already exists for topic %s and group %s", topic, groupID)
	}

	// Create consumer client
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(b.brokers...),
		kgo.ConsumerGroup(groupID),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()), // Start from beginning
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	b.consumers[consumerKey] = consumer

	// Create message channel
	msgChan := make(chan Message, 100)

	// Start consuming in a goroutine
	go b.consumeLoop(ctx, consumer, msgChan)

	return msgChan, nil
}

// consumeLoop continuously polls for messages and sends them to the channel.
func (b *RedpandaBroker) consumeLoop(ctx context.Context, consumer *kgo.Client, msgChan chan<- Message) {
	defer close(msgChan)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			fetches := consumer.PollFetches(ctx)
			if fetches.IsClientClosed() {
				return
			}

			// Handle any errors
			if errs := fetches.Errors(); len(errs) > 0 {
				// Log errors but continue
				for _, err := range errs {
					fmt.Printf("[RedpandaBroker] Fetch error: %v\n", err.Err)
				}
				continue
			}

			// Process records
			fetches.EachRecord(func(record *kgo.Record) {
				msg := Message{
					Topic:     record.Topic,
					Key:       string(record.Key),
					Value:     record.Value,
					Offset:    record.Offset,
					Partition: record.Partition,
					Timestamp: record.Timestamp.UnixMilli(),
				}

				select {
				case msgChan <- msg:
				case <-ctx.Done():
					return
				}
			})
		}
	}
}

// Close shuts down the broker and all consumer connections.
func (b *RedpandaBroker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	// Close all consumers
	for _, consumer := range b.consumers {
		consumer.Close()
	}
	b.consumers = make(map[string]*kgo.Client)

	// Close producer client
	b.client.Close()

	return nil
}
