// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

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
	// Flag to track if we're in --detach mode (non-interactive, no TUI)
	isDetachMode bool
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

		// Check if we're in --detach mode (non-interactive)
		detachFlag := cmd.Flags().Lookup("detach")
		isDetachMode = detachFlag != nil && detachFlag.Value.String() == "true"

		// Initialize the broker before any command runs
		inMemoryBroker := broker.NewInMemoryBroker()

		// Only enable verbose broker logging in detach mode
		// (TUI mode needs quiet broker to prevent log interference)
		if isDetachMode {
			inMemoryBroker.SetVerbose(true)
		}

		msgBroker = inMemoryBroker
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
	Short: "Launch the TUI to view existing triage cards (requires persistent broker)",
	Long: `Waits for triage cards to appear on the ci_failures_ranked topic and displays them
in an interactive TUI.

This command is useful when:
- Using a persistent message broker (Redpanda, Kafka, etc.) where data survives between processes
- Running alongside other processes that are producing triage cards
- Viewing results from previously submitted builds

For in-memory broker: Use 'destill build <url>' instead, which runs the
complete pipeline in a single process with streaming TUI.

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
			fmt.Println("   • With in-memory broker: Use 'destill build <url>' (TUI is default)")
			fmt.Println("   • With persistent broker: Ensure builds have been submitted via 'destill build <url> --detach'")
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
		if err := tui.Start(cards); err != nil {
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

// groupCardsByHash combines triage cards with the same message hash
// Returns a deduplicated list with recurrence counts
func groupCardsByHash(cards []contracts.TriageCard) []contracts.TriageCard {
	hashMap := make(map[string]*contracts.TriageCard)

	for _, card := range cards {
		if existing, found := hashMap[card.MessageHash]; found {
			// Increment recurrence count
			count := getRecurrenceCount(*existing) + 1
			if existing.Metadata == nil {
				existing.Metadata = make(map[string]string)
			}
			existing.Metadata["recurrence_count"] = fmt.Sprintf("%d", count)
		} else {
			// First occurrence - create a copy and initialize count
			cardCopy := card
			if cardCopy.Metadata == nil {
				cardCopy.Metadata = make(map[string]string)
			}
			cardCopy.Metadata["recurrence_count"] = "1"
			hashMap[card.MessageHash] = &cardCopy
		}
	}

	// Convert map to slice
	grouped := make([]contracts.TriageCard, 0, len(hashMap))
	for _, card := range hashMap {
		grouped = append(grouped, *card)
	}

	return grouped
}

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build [build-url]",
	Short: "Analyzes a Buildkite build and launches the triage TUI.",
	Long: `Submits a Buildkite URL (e.g., https://buildkite.com/org/pipeline/builds/4091) 
for analysis and launches the interactive TUI in streaming mode.

By default: Launches the TUI immediately. Cards appear in real-time as they are 
analyzed. Press 'r' to refresh/re-rank the list when new cards arrive.

With --detach: Publishes the request and exits immediately without launching TUI.
Useful for CI/automation. Requires a persistent message broker (Redpanda, Kafka)
to retain data between process invocations.

With --cache: Load previously saved cards from a JSON file for fast iteration
during development.

Example:
  destill build https://buildkite.com/org/pipeline/builds/4091
  destill build https://buildkite.com/org/pipeline/builds/4091 --cache build.json
  destill build https://buildkite.com/org/pipeline/builds/4091 --detach`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]
		detachMode, _ := cmd.Flags().GetBool("detach")

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

		if detachMode {
			// Detached mode (for CI/automation with persistent broker)
			fmt.Printf("✅ Submitted build request %s\n", request.RequestID)
			fmt.Printf("   Build URL: %s\n", buildURL)
			fmt.Println()
			fmt.Println("The pipeline will discover and process all job logs from this build.")
			fmt.Println()
			fmt.Println("💡 Note: Using --detach mode. Results will be available in the message broker.")
			fmt.Println("   Use 'destill view' to see results later (requires persistent broker).")
			return
		}

		// Check for cache flag
		cacheFile, _ := cmd.Flags().GetString("cache")
		var initialCards []contracts.TriageCard

		if cacheFile != "" {
			// Try to load from cache
			if data, err := os.ReadFile(cacheFile); err == nil {
				if err := json.Unmarshal(data, &initialCards); err == nil {
					fmt.Printf("📂 Loaded %d cards from cache: %s\n", len(initialCards), cacheFile)
				}
			}
		}

		// Sort initial cards if loaded from cache
		if len(initialCards) > 0 {
			sort.Slice(initialCards, func(i, j int) bool {
				if initialCards[i].ConfidenceScore != initialCards[j].ConfidenceScore {
					return initialCards[i].ConfidenceScore > initialCards[j].ConfidenceScore
				}
				countI := getRecurrenceCount(initialCards[i])
				countJ := getRecurrenceCount(initialCards[j])
				return countI > countJ
			})
		}

		// Launch the streaming TUI
		// - If cache provided: starts with cached cards, no streaming
		// - Otherwise: starts empty and streams cards as they arrive
		var brokerForTUI contracts.MessageBroker
		if len(initialCards) == 0 {
			// Streaming mode: pass broker to TUI for live updates
			brokerForTUI = msgBroker
			fmt.Println("🚀 Launching TUI (cards will stream in as they're analyzed)...")
		} else {
			// Cache mode: no streaming, use pre-loaded cards
			fmt.Println("🚀 Launching TUI with cached data...")
		}

		// Brief pause to ensure any remaining log output completes before TUI starts
		time.Sleep(100 * time.Millisecond)

		if err := tui.StartWithBroker(brokerForTUI, initialCards); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}

		// Save to cache after TUI exits (if streaming mode and cache flag set)
		// Note: This would require the TUI to return the final cards, which we don't do yet
		// For now, cache is only used for loading, not saving in streaming mode
	},
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
// In TUI mode (default), uses silent logger to prevent log output from interfering with the display.
func startStreamPipeline() {
	// Choose logger based on mode:
	// - Silent logger in TUI mode (prevents log pollution)
	// - Console logger in detach mode (useful for debugging and monitoring)
	var log logger.Logger
	if isDetachMode {
		log = logger.NewConsoleLogger()
	} else {
		log = logger.NewSilentLogger()
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

	// Only log pipeline start in detach mode (TUI mode needs quiet startup)
	if isDetachMode {
		log.Info("[Pipeline] Stream processing pipeline started")
	}
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(buildCmd)

	// Add --detach flag to build command (TUI is now the default)
	buildCmd.Flags().BoolP("detach", "d", false, "Detach mode: submit request and exit without launching TUI")
	// Add --cache flag for faster iteration during development
	buildCmd.Flags().StringP("cache", "c", "", "Cache file path to save/load triage cards (speeds up iteration)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
