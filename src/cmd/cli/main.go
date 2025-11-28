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
	"destill-agent/src/config"
	"destill-agent/src/contracts"
)

var (
	// Shared message broker for all agents
	msgBroker contracts.MessageBroker
	// Application configuration
	appConfig *config.Config
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
		// Load configuration from environment variables
		var err error
		appConfig, err = config.LoadFromEnv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Please set the BUILDKITE_API_TOKEN environment variable")
			os.Exit(1)
		}

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

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [build-url]",
	Short: "Submits an entire Buildkite build run for analysis.",
	Long: `Submits a Buildkite URL (e.g., https://buildkite.com/org/pipeline/builds/4091) 
to the destill_requests topic. The Ingestion Agent will then discover all job 
logs associated with that build and process them.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]

		// Create the request payload
		request := struct {
			RequestID string `json:"request_id"`
			BuildURL  string `json:"build_url"`
		}{
			RequestID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
			BuildURL:  buildURL,
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

		fmt.Printf("Submitted build request %s\n", request.RequestID)
		fmt.Printf("Build URL: %s\n", buildURL)
		fmt.Println("The pipeline will discover and process all job logs from this build.")
	},
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
func startStreamPipeline() {
	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingestion.NewAgent(msgBroker, appConfig.BuildkiteAPIToken)
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
	rootCmd.AddCommand(buildCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
