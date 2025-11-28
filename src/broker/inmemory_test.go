// Package broker provides implementations of the MessageBroker interface.
package broker

import (
	"sync"
	"testing"
	"time"
)

// TestPublishDeliverToSubscriber verifies a message is published and received successfully.
func TestPublishDeliverToSubscriber(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	// Subscribe to topic
	ch, err := broker.Subscribe("test-topic")
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Publish message
	testMsg := []byte("hello world")
	if err := broker.Publish("test-topic", testMsg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Receive message with timeout
	select {
	case received := <-ch:
		if string(received) != string(testMsg) {
			t.Errorf("Expected %q, got %q", testMsg, received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

// TestTopicIsolation verifies subscribers on different topics do not receive wrong messages.
func TestTopicIsolation(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	// Subscribe to two different topics
	chA, err := broker.Subscribe("topic-a")
	if err != nil {
		t.Fatalf("Subscribe to topic-a failed: %v", err)
	}
	chB, err := broker.Subscribe("topic-b")
	if err != nil {
		t.Fatalf("Subscribe to topic-b failed: %v", err)
	}

	// Publish to topic-a only
	testMsg := []byte("message for topic-a")
	if err := broker.Publish("topic-a", testMsg); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	// Topic A should receive message
	select {
	case received := <-chA:
		if string(received) != string(testMsg) {
			t.Errorf("Expected %q, got %q", testMsg, received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for message on topic-a")
	}

	// Topic B should NOT receive any message
	select {
	case msg := <-chB:
		t.Errorf("Topic B should not receive message, but got: %q", msg)
	case <-time.After(100 * time.Millisecond):
		// Expected: no message received
	}
}

// TestConcurrentPublishSubscribe verifies the sync.RWMutex correctly protects the subscribers map.
func TestConcurrentPublishSubscribe(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	const numGoroutines = 50
	var wg sync.WaitGroup

	// Half goroutines publish, half subscribe
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_ = broker.Publish("concurrent-topic", []byte("msg"))
				}
			}(i)
		} else {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_, _ = broker.Subscribe("concurrent-topic")
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

	// Subscribe to two different topics
	ch1, err := broker.Subscribe("topic-1")
	if err != nil {
		t.Fatalf("Subscribe to topic-1 failed: %v", err)
	}
	ch2, err := broker.Subscribe("topic-2")
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

	err := broker.Publish("topic", []byte("msg"))
	if err == nil {
		t.Error("Expected error when publishing to closed broker")
	}
}

// TestSubscribeAfterClose verifies subscribing after close returns error.
func TestSubscribeAfterClose(t *testing.T) {
	broker := NewInMemoryBroker()
	broker.Close()

	_, err := broker.Subscribe("topic")
	if err == nil {
		t.Error("Expected error when subscribing to closed broker")
	}
}

// TestMultipleSubscribersSameTopic verifies all subscribers receive the same message.
func TestMultipleSubscribersSameTopic(t *testing.T) {
	broker := NewInMemoryBroker()
	defer broker.Close()

	// Create 3 subscribers on same topic
	ch1, _ := broker.Subscribe("shared-topic")
	ch2, _ := broker.Subscribe("shared-topic")
	ch3, _ := broker.Subscribe("shared-topic")

	// Publish message
	testMsg := []byte("broadcast message")
	broker.Publish("shared-topic", testMsg)

	// All subscribers should receive the message
	for i, ch := range []<-chan []byte{ch1, ch2, ch3} {
		select {
		case received := <-ch:
			if string(received) != string(testMsg) {
				t.Errorf("Subscriber %d: expected %q, got %q", i, testMsg, received)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Subscriber %d: timeout waiting for message", i)
		}
	}
}
