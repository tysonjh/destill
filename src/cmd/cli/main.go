// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the user interface for interacting with the log triage system.
package main

import (
	"flag"
	"fmt"
	"os"

	"destill-agent/src/contracts"
)

// InMemoryBroker is a simple in-memory implementation of MessageBroker for demonstration.
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

func main() {
	// Define CLI flags
	command := flag.String("cmd", "help", "Command to execute: ingest, analyze, help")
	message := flag.String("message", "", "Log message to ingest")
	severity := flag.String("severity", "INFO", "Log severity level")
	flag.Parse()

	// Ensure the broker implements the interface
	var broker contracts.MessageBroker = NewInMemoryBroker()
	defer broker.Close()

	switch *command {
	case "ingest":
		if *message == "" {
			fmt.Println("Error: --message is required for ingest command")
			os.Exit(1)
		}
		card := contracts.TriageCard{
			ID:       "cli-001",
			Source:   "cli",
			Severity: *severity,
			Message:  *message,
		}
		if err := broker.Publish("logs", card); err != nil {
			fmt.Printf("Error publishing message: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Ingested log: %s [%s]\n", card.Message, card.Severity)
	case "analyze":
		fmt.Println("Analysis feature: Ready to analyze triage cards")
	case "help":
		fmt.Println("Destill CLI - Log Triage Tool")
		fmt.Println("Usage:")
		fmt.Println("  --cmd ingest --message <msg> --severity <level>  : Ingest a log message")
		fmt.Println("  --cmd analyze                                     : Run analysis")
		fmt.Println("  --cmd help                                        : Show this help")
	default:
		fmt.Printf("Unknown command: %s\n", *command)
		os.Exit(1)
	}
}
