// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/spf13/cobra"

	"destill-agent/src/broker"
	"destill-agent/src/contracts"
)

var (
	// Shared message broker for all agents
	msgBroker contracts.MessageBroker
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "destill",
	Short: "Destill - A log triage tool for CI/CD pipelines",
	Long: `Destill is a decoupled, agent-based log triage tool that helps
analyze and categorize CI/CD build failures.

It uses a stream processing architecture with:
- Ingestion Agent: Consumes requests and fetches raw logs
- Analysis Agent: Processes logs and produces ranked failure cards`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize the broker before any command runs
		inMemoryBroker := broker.NewInMemoryBroker()
		inMemoryBroker.SetVerbose(true)
		msgBroker = inMemoryBroker

		// Start the stream pipeline
		startStreamPipeline()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Clean up broker when done
		if msgBroker != nil {
			msgBroker.Close()
		}
	},
}

// analyzeCmd represents the analyze command
var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Launch the analysis TUI",
	Long: `Launches the Bubble Tea TUI application for interactive log analysis.
This command starts the full stream processing pipeline and presents
the ranked failure cards in an interactive terminal interface.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Destill Analysis Mode")
		fmt.Println("=====================")
		fmt.Println("Stream pipeline is running...")
		fmt.Println("TUI application will be implemented here.")
		fmt.Println("")
		fmt.Println("Press Ctrl+C to exit.")

		// Keep the application running
		// In the future, this will launch the Bubble Tea TUI
		select {}
	},
}

// IngestionAgent consumes requests and publishes raw log data via a MessageBroker.
type IngestionAgent struct {
	msgBroker contracts.MessageBroker
}

// NewIngestionAgent creates a new IngestionAgent with the given broker.
func NewIngestionAgent(msgBroker contracts.MessageBroker) *IngestionAgent {
	return &IngestionAgent{msgBroker: msgBroker}
}

// Run starts the ingestion agent's main loop.
func (a *IngestionAgent) Run() {
	requestChannel, err := a.msgBroker.Subscribe("destill_requests")
	if err != nil {
		fmt.Printf("[IngestionAgent] Error subscribing: %v\n", err)
		return
	}

	fmt.Println("[IngestionAgent] Started - listening on 'destill_requests'")

	for message := range requestChannel {
		fmt.Printf("[IngestionAgent] Received request: %d bytes\n", len(message))
		// Process and publish to ci_logs_raw
		if err := a.msgBroker.Publish("ci_logs_raw", message); err != nil {
			fmt.Printf("[IngestionAgent] Error publishing: %v\n", err)
		}
	}
}

// AnalysisAgent subscribes to raw logs and performs analysis.
type AnalysisAgent struct {
	msgBroker contracts.MessageBroker
}

// NewAnalysisAgent creates a new AnalysisAgent with the given broker.
func NewAnalysisAgent(msgBroker contracts.MessageBroker) *AnalysisAgent {
	return &AnalysisAgent{msgBroker: msgBroker}
}

// Run starts the analysis agent's main loop.
func (a *AnalysisAgent) Run() {
	logChannel, err := a.msgBroker.Subscribe("ci_logs_raw")
	if err != nil {
		fmt.Printf("[AnalysisAgent] Error subscribing: %v\n", err)
		return
	}

	fmt.Println("[AnalysisAgent] Started - listening on 'ci_logs_raw'")

	for message := range logChannel {
		go a.processLogChunk(message)
	}
}

// processLogChunk handles an incoming raw log chunk.
func (a *AnalysisAgent) processLogChunk(message []byte) {
	fmt.Printf("[AnalysisAgent] Processing log chunk: %d bytes\n", len(message))
	// Placeholder: Full analysis pipeline will be implemented here
	// 1. Deserialize message
	// 2. Normalize log
	// 3. Calculate message hash
	// 4. Publish to ci_failures_ranked
}

// startStreamPipeline launches the Ingestion and Analysis agents as Go routines.
func startStreamPipeline() {
	var wg sync.WaitGroup

	// Start Ingestion Agent
	ingestionAgent := NewIngestionAgent(msgBroker)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ingestionAgent.Run()
	}()

	// Start Analysis Agent
	analysisAgent := NewAnalysisAgent(msgBroker)
	wg.Add(1)
	go func() {
		defer wg.Done()
		analysisAgent.Run()
	}()

	fmt.Println("[Pipeline] Stream processing pipeline started")
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
