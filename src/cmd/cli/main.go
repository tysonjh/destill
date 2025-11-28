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
	"destill-agent/src/cmd/analysis"
	"destill-agent/src/cmd/ingestion"
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

// submitCmd represents the submit command
var submitCmd = &cobra.Command{
	Use:   "submit [log-url] [job-name]",
	Short: "Submits a raw log URL to the pipeline for analysis.",
	Long: `Submits a raw log URL to the destill_requests topic to kick off the
streaming pipeline. The Ingestion Agent will pick up the request,
fetch the log content, and publish it for analysis.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		logURL := args[0]
		jobName := args[1]

		// Create the request payload
		request := struct {
			RequestID string `json:"request_id"`
			JobName   string `json:"job_name"`
			LogURL    string `json:"log_url"`
		}{
			RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
			JobName:   jobName,
			LogURL:    logURL,
		}

		// Marshal the request
		data, err := json.Marshal(request)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling request: %v\n", err)
			os.Exit(1)
		}

		// Publish to destill_requests topic
		if err := msgBroker.Publish("destill_requests", data); err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing request: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Submitted request %s for job '%s'\n", request.RequestID, jobName)
		fmt.Printf("Log URL: %s\n", logURL)
		fmt.Println("The pipeline will process this request asynchronously.")
	},
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
func startStreamPipeline() {
	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingestion.NewAgent(msgBroker)
	go func() {
		if err := ingestionAgent.Run(); err != nil {
			fmt.Printf("[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := analysis.NewAgent(msgBroker)
	go func() {
		if err := analysisAgent.Run(); err != nil {
			fmt.Printf("[Pipeline] Analysis agent error: %v\n", err)
		}
	}()

	fmt.Println("[Pipeline] Stream processing pipeline started")
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(submitCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
