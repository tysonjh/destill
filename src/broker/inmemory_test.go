// Package broker provides implementations of the Broker interface.
package broker

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestPublishDeliverToSubscriber verifies a message is published and received successfully.
func TestPublishDeliverToSubscriber(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()

	// Subscribe to topic
	ch, err := broker.Subscribe(ctx, "test-topic", "test-group")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish message
	testMsg := []byte("hello world")
	if err := broker.Publish(ctx, "test-topic", "key", testMsg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive message with timeout
	select {
	case msg := <-ch:
		if string(msg.Value) != string(testMsg) {
			t.Errorf("Expected %q, got %q", testMsg, msg.Value)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestTopicIsolation verifies subscribers on different topics do not receive wrong messages.
func TestTopicIsolation(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()

	// Subscribe to two different topics
	chA, err := broker.Subscribe(ctx, "topic-a", "group-a")
	if err != nil {
		t.Fatalf("Subscribe to topic-a failed: %v", err)
	}
	chB, err := broker.Subscribe(ctx, "topic-b", "group-b")
	if err != nil {
		t.Fatalf("Subscribe to topic-b failed: %v", err)
	}

	// Publish to topic-a only
	testMsg := []byte("message for topic-a")
	if err := broker.Publish(ctx, "topic-a", "key", testMsg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Topic A should receive message
	select {
	case msg := <-chA:
		if string(msg.Value) != string(testMsg) {
			t.Errorf("Expected %q, got %q", testMsg, msg.Value)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message on topic-a")
	}

	// Topic B should NOT receive any message
	select {
	case msg := <-chB:
		t.Errorf("Topic B should not receive message, but got: %q", msg.Value)
	case <-time.After(100 * time.Millisecond):
		// Expected: no message received
	}
}

// TestConcurrentPublishSubscribe verifies the sync.RWMutex correctly protects the subscribers map.
func TestConcurrentPublishSubscribe(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()
	const numGoroutines = 50
	var wg sync.WaitGroup

	// Half goroutines publish, half subscribe
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_ = broker.Publish(ctx, "concurrent-topic", "key", []byte("msg"))
				}
			}(i)
		} else {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_, _ = broker.Subscribe(ctx, "concurrent-topic", "group")
				}
			}(i)
		}
	}

	// Wait for all goroutines to complete without panic
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - no race conditions
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout - possible deadlock in concurrent access")
	}
}

// TestCloseGracefulShutdown verifies broker.Close() correctly closes all subscriber channels.
func TestCloseGracefulShutdown(t *testing.T) {
	broker := NewInMemoryBroker()

	ctx := context.Background()

	// Subscribe to two different topics
	ch1, err := broker.Subscribe(ctx, "topic-1", "group-1")
	if err != nil {
		t.Fatalf("Subscribe to topic-1 failed: %v", err)
	}
	ch2, err := broker.Subscribe(ctx, "topic-2", "group-2")
	if err != nil {
		t.Fatalf("Subscribe to topic-2 failed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Start goroutines that read from channels
	go func() {
		defer wg.Done()
		for range ch1 {
			// Drain channel
		}
	}()
	go func() {
		defer wg.Done()
		for range ch2 {
			// Drain channel
		}
	}()

	// Close broker
	if err := broker.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait for goroutines to exit (channels should be closed)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success - both goroutines exited
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout - goroutines did not exit, channels may not be closed")
	}
}

// TestPublishAfterClose verifies publishing after close returns error.
func TestPublishAfterClose(t *testing.T) {
	broker := NewInMemoryBroker()
	broker.Close()

	ctx := context.Background()
	err := broker.Publish(ctx, "topic", "key", []byte("msg"))
	if err == nil {
		t.Error("Expected error when publishing to closed broker")
	}
}

// TestSubscribeAfterClose verifies subscribing after close returns error.
func TestSubscribeAfterClose(t *testing.T) {
	broker := NewInMemoryBroker()
	broker.Close()

	ctx := context.Background()
	_, err := broker.Subscribe(ctx, "topic", "group")
	if err == nil {
		t.Error("Expected error when subscribing to closed broker")
	}
}

// TestMultipleSubscribersSameTopic verifies all subscribers receive the same message.
func TestMultipleSubscribersSameTopic(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	ctx := context.Background()

	// Create 3 subscribers on same topic
	ch1, _ := broker.Subscribe(ctx, "shared-topic", "group1")
	ch2, _ := broker.Subscribe(ctx, "shared-topic", "group2")
	ch3, _ := broker.Subscribe(ctx, "shared-topic", "group3")

	// Publish message
	testMsg := []byte("broadcast message")
	if err := broker.Publish(ctx, "shared-topic", "key", testMsg); err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// All subscribers should receive the message
	for i, ch := range []<-chan Message{ch1, ch2, ch3} {
		select {
		case msg := <-ch:
			if string(msg.Value) != string(testMsg) {
				t.Errorf("Subscriber %d: expected %q, got %q", i, testMsg, msg.Value)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Subscriber %d: timeout waiting for message", i)
		}
	}
}
