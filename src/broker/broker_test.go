package broker

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryBroker_PublishSubscribe(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()
	topic := "test-topic"
	key := "test-key"
	value := []byte("test message")

	// Subscribe before publishing
	msgChan, err := broker.Subscribe(ctx, topic, "test-group")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish message
	if err := broker.Publish(ctx, topic, key, value); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive message
	select {
	case msg := <-msgChan:
		if msg.Topic != topic {
			t.Errorf("Expected topic %s, got %s", topic, msg.Topic)
		}
		if msg.Key != key {
			t.Errorf("Expected key %s, got %s", key, msg.Key)
		}
		if string(msg.Value) != string(value) {
			t.Errorf("Expected value %s, got %s", string(value), string(msg.Value))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestInMemoryBroker_MultipleSubscribers(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()
	topic := "test-topic"

	// Create two subscribers
	sub1, err := broker.Subscribe(ctx, topic, "group1")
	if err != nil {
		t.Fatalf("Subscribe 1 failed: %v", err)
	}

	sub2, err := broker.Subscribe(ctx, topic, "group2")
	if err != nil {
		t.Fatalf("Subscribe 2 failed: %v", err)
	}

	// Publish message
	value := []byte("broadcast message")
	if err := broker.Publish(ctx, topic, "key", value); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Both subscribers should receive the message
	for i, sub := range []<-chan Message{sub1, sub2} {
		select {
		case msg := <-sub:
			if string(msg.Value) != string(value) {
				t.Errorf("Subscriber %d: expected value %s, got %s", i+1, string(value), string(msg.Value))
			}
		case <-time.After(1 * time.Second):
			t.Fatalf("Subscriber %d: timeout waiting for message", i+1)
		}
	}
}

func TestInMemoryBroker_ClosedBroker(t *testing.T) {
	broker := NewInMemoryBroker()
	broker.Close()

	ctx := context.Background()

	// Publishing to closed broker should fail
	err := broker.Publish(ctx, "test", "key", []byte("value"))
	if err == nil {
		t.Error("Expected error when publishing to closed broker")
	}

	// Subscribing to closed broker should fail
	_, err = broker.Subscribe(ctx, "test", "group")
	if err == nil {
		t.Error("Expected error when subscribing to closed broker")
	}
}
