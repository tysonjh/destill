// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

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

		// Start the stream pipeline (agents run as persistent goroutines)
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
// This is a blocking call that runs until the channel is closed.
func (a *IngestionAgent) Run() {
	requestChannel, err := a.msgBroker.Subscribe("destill_requests")
	if err != nil {
		fmt.Printf("[IngestionAgent] Error subscribing: %v\n", err)
		return
	}

	fmt.Println("[IngestionAgent] Started - listening on 'destill_requests'")

	for message := range requestChannel {
		if err := a.processRequest(message); err != nil {
			fmt.Printf("[IngestionAgent] Error processing request: %v\n", err)
		}
	}
}

// processRequest handles an incoming request and creates a LogChunk.
func (a *IngestionAgent) processRequest(message []byte) error {
	// Parse the incoming request
	var request struct {
		RequestID string `json:"request_id"`
		JobName   string `json:"job_name"`
		LogURL    string `json:"log_url"`
	}

	if err := json.Unmarshal(message, &request); err != nil {
		return fmt.Errorf("failed to unmarshal request: %w", err)
	}

	fmt.Printf("[IngestionAgent] Processing request %s for job %s\n", request.RequestID, request.JobName)

	// Create a LogChunk from the request
	logChunk := contracts.LogChunk{
		ID:        fmt.Sprintf("chunk-%d", time.Now().UnixNano()),
		RequestID: request.RequestID,
		JobName:   request.JobName,
		Content:   fmt.Sprintf("Placeholder log content for %s", request.LogURL),
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Marshal and publish to ci_logs_raw
	data, err := json.Marshal(logChunk)
	if err != nil {
		return fmt.Errorf("failed to marshal log chunk: %w", err)
	}

	if err := a.msgBroker.Publish("ci_logs_raw", data); err != nil {
		return fmt.Errorf("failed to publish to ci_logs_raw: %w", err)
	}

	fmt.Printf("[IngestionAgent] Published log chunk to 'ci_logs_raw'\n")
	return nil
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
// This is a blocking call that runs until the channel is closed.
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

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
func startStreamPipeline() {
	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := NewIngestionAgent(msgBroker)
	go ingestionAgent.Run()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := NewAnalysisAgent(msgBroker)
	go analysisAgent.Run()

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
