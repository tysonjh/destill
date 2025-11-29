// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"destill-agent/src/broker"
	"destill-agent/src/cmd/analysis"
	"destill-agent/src/cmd/ingestion"
	"destill-agent/src/config"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
	"destill-agent/src/tui"
)

var (
	// Shared message broker for all agents
	msgBroker contracts.MessageBroker
	// Application configuration
	appConfig *config.Config
	// Flag to track if we're in --wait mode (affects logging)
	isWaitMode bool
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

		// Check if we're in --wait mode
		waitFlag := cmd.Flags().Lookup("wait")
		isWaitMode = waitFlag != nil && waitFlag.Value.String() == "true"

		// Only enable verbose broker logging if NOT using --wait mode
		// (--wait mode will launch TUI, so we don't want logs interfering)
		if !isWaitMode {
			inMemoryBroker.SetVerbose(true)
		}

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
	Short: "Launch the TUI to view triage cards",
	Long: `Waits for triage cards to appear on the ci_failures_ranked topic and displays them
in an interactive TUI.

This command is useful when:
- Using a persistent message broker (Redis, Kafka, etc.) where data survives between processes
- Running alongside other processes that are producing triage cards
- Viewing results from previously submitted builds

For in-memory broker: Use 'destill build <url> --wait' instead, which runs the
complete pipeline in a single process.

Example:
  destill analyze`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Destill Analysis Mode")
		fmt.Println("=====================")
		fmt.Println("Waiting for triage cards (5 seconds)...")
		fmt.Println()

		// Collect triage cards from the ci_failures_ranked topic
		cards := collectTriageCards(5 * time.Second)

		if len(cards) == 0 {
			fmt.Println("⚠️  No failure cards collected.")
			fmt.Println()
			fmt.Println("💡 Tips:")
			fmt.Println("   • With in-memory broker: Use 'destill build <url> --wait'")
			fmt.Println("   • With persistent broker: Ensure builds have been submitted via 'destill build <url>'")
			return
		}

		// Sort cards by confidence score (descending), then by recurrence count
		sort.Slice(cards, func(i, j int) bool {
			if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
				return cards[i].ConfidenceScore > cards[j].ConfidenceScore
			}
			// Extract recurrence count from metadata for sorting
			countI := getRecurrenceCount(cards[i])
			countJ := getRecurrenceCount(cards[j])
			return countI > countJ
		})

		// Launch the TUI
		model := tui.NewTriageModel(cards)
		p := tea.NewProgram(model, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

// collectTriageCards subscribes to ci_failures_ranked and collects cards for a duration
func collectTriageCards(duration time.Duration) []contracts.TriageCard {
	var cards []contracts.TriageCard

	// Subscribe to the ranked failures topic
	rankChan, err := msgBroker.Subscribe("ci_failures_ranked")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error subscribing to ci_failures_ranked: %v\n", err)
		return cards
	}

	// Collect cards for the specified duration
	timeout := time.After(duration)

collectLoop:
	for {
		select {
		case msg := <-rankChan:
			var card contracts.TriageCard
			if err := json.Unmarshal(msg, &card); err != nil {
				fmt.Fprintf(os.Stderr, "Error unmarshaling triage card: %v\n", err)
				continue
			}
			cards = append(cards, card)

		case <-timeout:
			break collectLoop
		}
	}

	return cards
}

// getRecurrenceCount extracts the recurrence count from metadata
func getRecurrenceCount(card contracts.TriageCard) int {
	if card.Metadata == nil {
		return 1
	}
	if count, ok := card.Metadata["recurrence_count"]; ok {
		var c int
		fmt.Sscanf(count, "%d", &c)
		return c
	}
	return 1
}

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [build-url]",
	Short: "Submits an entire Buildkite build run for analysis.",
	Long: `Submits a Buildkite URL (e.g., https://buildkite.com/org/pipeline/builds/4091) 
to the destill_requests topic. The Ingestion Agent will then discover all job 
logs associated with that build and process them.

With --wait flag: Keeps the process running, collects results, and launches the TUI.
This is useful with the in-memory broker for immediate feedback.

Without --wait: Publishes the request and exits immediately. Requires a persistent
message broker (Redis, Kafka, etc.) to retain data between process invocations.

Example:
  destill build https://buildkite.com/org/pipeline/builds/4091 --wait`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]
		waitForResults, _ := cmd.Flags().GetBool("wait")

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

		if !waitForResults {
			// Fire-and-forget mode (production with persistent broker)
			fmt.Printf("✅ Submitted build request %s\n", request.RequestID)
			fmt.Printf("   Build URL: %s\n", buildURL)
			fmt.Println()
			fmt.Println("The pipeline will discover and process all job logs from this build.")
			fmt.Println()
			fmt.Println("💡 Tip: Use --wait flag to wait for results and launch the TUI.")
			fmt.Println("   (Required when using in-memory broker)")
			return
		}

		// Interactive mode (wait for results and show TUI)
		fmt.Println("Destill - CI/CD Failure Triage")
		fmt.Println("===============================")
		fmt.Printf("Build URL: %s\n", buildURL)
		fmt.Println()
		fmt.Println("📥 Fetching build metadata and job logs...")
		fmt.Println("🔍 Analyzing logs for failures...")
		fmt.Println("⏳ Waiting for pipeline to complete (10 seconds)...")
		fmt.Println()

		// Collect triage cards from the pipeline
		cards := collectTriageCards(10 * time.Second)

		if len(cards) == 0 {
			fmt.Println("✅ No failures detected in this build!")
			fmt.Println()
			return
		}

		fmt.Printf("📊 Found %d failure(s). Launching TUI...\n", len(cards))
		fmt.Println()

		// Sort cards by confidence score (descending), then by recurrence count
		sort.Slice(cards, func(i, j int) bool {
			if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
				return cards[i].ConfidenceScore > cards[j].ConfidenceScore
			}
			countI := getRecurrenceCount(cards[i])
			countJ := getRecurrenceCount(cards[j])
			return countI > countJ
		})

		// Brief pause to ensure any remaining log output completes before TUI starts
		time.Sleep(500 * time.Millisecond)

		// Launch the TUI
		model := tui.NewTriageModel(cards)
		p := tea.NewProgram(model, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
// In --wait mode (TUI), uses silent logger to prevent log output from interfering with the display.
func startStreamPipeline() {
	// Choose logger based on mode:
	// - Silent logger in --wait mode (prevents log pollution in TUI)
	// - Console logger in normal mode (useful for debugging and monitoring)
	var log logger.Logger
	if isWaitMode {
		log = logger.NewSilentLogger()
	} else {
		log = logger.NewConsoleLogger()
	}

	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingestion.NewAgent(msgBroker, appConfig.BuildkiteAPIToken, log)
	go func() {
		if err := ingestionAgent.Run(); err != nil {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := analysis.NewAgent(msgBroker, log)
	go func() {
		if err := analysisAgent.Run(); err != nil {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Analysis agent error: %v\n", err)
		}
	}()

	// Only log pipeline start in non-wait mode
	if !isWaitMode {
		log.Info("[Pipeline] Stream processing pipeline started")
	}
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(buildCmd)

	// Add --wait flag to build command
	buildCmd.Flags().BoolP("wait", "w", false, "Wait for pipeline to complete and launch TUI (required for in-memory broker)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
