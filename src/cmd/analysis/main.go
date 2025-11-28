// Package main provides the Analysis Agent for the Destill log triage tool.
// This agent subscribes to TriageCards and performs log analysis.
package main

import (
	"fmt"
	"strings"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

// AnalysisAgent subscribes to TriageCards and performs analysis.
type AnalysisAgent struct {
	msgBroker contracts.MessageBroker
}

// NewAnalysisAgent creates a new AnalysisAgent with the given broker.
func NewAnalysisAgent(msgBroker contracts.MessageBroker) *AnalysisAgent {
	return &AnalysisAgent{msgBroker: msgBroker}
}

// Start begins listening for TriageCards on the specified topic.
func (a *AnalysisAgent) Start(topic string) error {
	return a.msgBroker.Subscribe(topic, a.analyze)
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

	// Create broker using the shared implementation
	var msgBroker contracts.MessageBroker = broker.NewInMemoryBroker()
	defer msgBroker.Close()

	agent := NewAnalysisAgent(msgBroker)

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
		if err := msgBroker.Publish("triage", card); err != nil {
			fmt.Printf("Error publishing test card: %v\n", err)
		}
	}

	fmt.Println("Analysis Agent completed.")
}
