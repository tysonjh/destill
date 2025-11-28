// Package main provides the Ingestion Agent for the Destill log triage tool.
// This agent is responsible for ingesting log data and publishing TriageCards.
package main

import (
	"fmt"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

// IngestionAgent processes log data and publishes TriageCards via a MessageBroker.
type IngestionAgent struct {
	msgBroker contracts.MessageBroker
}

// NewIngestionAgent creates a new IngestionAgent with the given broker.
func NewIngestionAgent(msgBroker contracts.MessageBroker) *IngestionAgent {
	return &IngestionAgent{msgBroker: msgBroker}
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
	return a.msgBroker.Publish("triage", card)
}

func main() {
	fmt.Println("Destill Ingestion Agent starting...")

	// Create broker using the shared implementation
	inMemoryBroker := broker.NewInMemoryBroker()
	inMemoryBroker.SetVerbose(true)
	var msgBroker contracts.MessageBroker = inMemoryBroker
	defer msgBroker.Close()

	agent := NewIngestionAgent(msgBroker)

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
