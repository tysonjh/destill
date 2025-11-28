// Package main provides the Ingestion Agent for the Destill log triage tool.
// This agent is responsible for ingesting log data and publishing TriageCards.
package main

import (
	"fmt"
	"time"

	"destill-agent/src/contracts"
)

// InMemoryBroker is a simple in-memory implementation of MessageBroker.
type InMemoryBroker struct {
	handlers map[string][]func(contracts.TriageCard) error
}

// NewInMemoryBroker creates a new InMemoryBroker instance.
func NewInMemoryBroker() *InMemoryBroker {
	return &InMemoryBroker{
		handlers: make(map[string][]func(contracts.TriageCard) error),
	}
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
	fmt.Printf("[InMemoryBroker] Published to topic '%s': %+v\n", topic, card)
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

// IngestionAgent processes log data and publishes TriageCards via a MessageBroker.
type IngestionAgent struct {
	broker contracts.MessageBroker
}

// NewIngestionAgent creates a new IngestionAgent with the given broker.
func NewIngestionAgent(broker contracts.MessageBroker) *IngestionAgent {
	return &IngestionAgent{broker: broker}
}

// Ingest processes a raw log message and publishes it as a TriageCard.
func (a *IngestionAgent) Ingest(source, severity, message string) error {
	card := contracts.TriageCard{
		ID:        fmt.Sprintf("ingest-%d", time.Now().UnixNano()),
		Source:    source,
		Timestamp: time.Now().Format(time.RFC3339),
		Severity:  severity,
		Message:   message,
		Metadata:  make(map[string]string),
	}
	return a.broker.Publish("triage", card)
}

func main() {
	fmt.Println("Destill Ingestion Agent starting...")

	// Create broker and agent
	var broker contracts.MessageBroker = NewInMemoryBroker()
	defer broker.Close()

	agent := NewIngestionAgent(broker)

	// Example: Ingest some sample logs
	samples := []struct {
		source   string
		severity string
		message  string
	}{
		{"syslog", "INFO", "System started successfully"},
		{"app", "WARN", "High memory usage detected"},
		{"app", "ERROR", "Connection timeout to database"},
	}

	for _, s := range samples {
		if err := agent.Ingest(s.source, s.severity, s.message); err != nil {
			fmt.Printf("Error ingesting log: %v\n", err)
		}
	}

	fmt.Println("Ingestion Agent completed.")
}
