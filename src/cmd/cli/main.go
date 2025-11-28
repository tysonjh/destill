// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the user interface for interacting with the log triage system.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

func main() {
	// Define CLI flags
	command := flag.String("cmd", "help", "Command to execute: ingest, analyze, help")
	message := flag.String("message", "", "Log message to ingest")
	severity := flag.String("severity", "INFO", "Log severity level")
	flag.Parse()

	// Create broker using the shared implementation
	var msgBroker contracts.MessageBroker = broker.NewInMemoryBroker()
	defer msgBroker.Close()

	switch *command {
	case "ingest":
		if *message == "" {
			fmt.Println("Error: --message is required for ingest command")
			os.Exit(1)
		}
		card := contracts.TriageCard{
			ID:       fmt.Sprintf("cli-%d", time.Now().UnixNano()),
			Source:   "cli",
			Severity: *severity,
			Message:  *message,
		}
		if err := msgBroker.Publish("logs", card); err != nil {
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
