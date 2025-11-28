// Package main provides the Analysis Agent for the Destill log triage tool.
// This agent subscribes to TriageCards and performs log analysis.
package main

import (
	"fmt"
	"strings"

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

// AnalysisAgent subscribes to TriageCards and performs analysis.
type AnalysisAgent struct {
	broker contracts.MessageBroker
}

// NewAnalysisAgent creates a new AnalysisAgent with the given broker.
func NewAnalysisAgent(broker contracts.MessageBroker) *AnalysisAgent {
	return &AnalysisAgent{broker: broker}
}

// Start begins listening for TriageCards on the specified topic.
func (a *AnalysisAgent) Start(topic string) error {
	return a.broker.Subscribe(topic, a.analyze)
}

// analyze processes a TriageCard and performs analysis.
func (a *AnalysisAgent) analyze(card contracts.TriageCard) error {
	fmt.Printf("[Analysis] Processing card %s from %s\n", card.ID, card.Source)

	// Simple analysis: categorize by severity
	switch strings.ToUpper(card.Severity) {
	case "ERROR":
		fmt.Printf("  [ALERT] Error detected: %s\n", card.Message)
	case "WARN":
		fmt.Printf("  [WARNING] Potential issue: %s\n", card.Message)
	case "INFO":
		fmt.Printf("  [INFO] Normal log: %s\n", card.Message)
	default:
		fmt.Printf("  [UNKNOWN] Unrecognized severity: %s\n", card.Severity)
	}

	return nil
}

func main() {
	fmt.Println("Destill Analysis Agent starting...")

	// Create broker and agent
	var broker contracts.MessageBroker = NewInMemoryBroker()
	defer broker.Close()

	agent := NewAnalysisAgent(broker)

	// Subscribe to triage topic
	if err := agent.Start("triage"); err != nil {
		fmt.Printf("Error starting analysis agent: %v\n", err)
		return
	}

	// Simulate receiving some cards for demonstration
	testCards := []contracts.TriageCard{
		{ID: "test-001", Source: "syslog", Severity: "INFO", Message: "System boot complete"},
		{ID: "test-002", Source: "app", Severity: "ERROR", Message: "Database connection failed"},
		{ID: "test-003", Source: "app", Severity: "WARN", Message: "Response time above threshold"},
	}

	for _, card := range testCards {
		if err := broker.Publish("triage", card); err != nil {
			fmt.Printf("Error publishing test card: %v\n", err)
		}
	}

	fmt.Println("Analysis Agent completed.")
}
