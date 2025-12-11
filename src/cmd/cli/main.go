// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"destill-agent/src/broker"
	"destill-agent/src/cmd/analysis"
	"destill-agent/src/cmd/ingestion"
	"destill-agent/src/config"
	"destill-agent/src/contracts"
	"destill-agent/src/logger"
	"destill-agent/src/store"
	"destill-agent/src/tui"
)

var (
	// Shared message broker for all agents
	msgBroker broker.Broker
	// Application configuration
	appConfig *config.Config
	// Flag to track if we're in --detach mode (non-interactive, no TUI)
	isDetachMode bool
	// Context for agent lifecycle
	agentCtx    context.Context
	agentCancel context.CancelFunc
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
		// Skip initialization for view command (doesn't need broker or config)
		if cmd.Name() == "view" {
			return
		}

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
		msgBroker = broker.NewInMemoryBroker()

		// Only enable verbose broker logging in detach mode
		// (TUI mode needs quiet broker to prevent log interference)
		if isDetachMode {
			msgBroker.(*broker.InMemoryBroker).SetVerbose(true)
		}

		// Create context for agent lifecycle
		agentCtx, agentCancel = context.WithCancel(context.Background())
		startStreamPipeline()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		// Cancel agent context and clean up broker when done
		if agentCancel != nil {
			agentCancel()
		}
		if msgBroker != nil {
			msgBroker.Close()
		}
	},
}

// viewCmd represents the view command for querying findings from Postgres
var viewCmd = &cobra.Command{
	Use:   "view <request-id>",
	Short: "View findings from Postgres in TUI (distributed mode)",
	Long: `Queries Postgres for findings associated with a request ID and displays them
in an interactive TUI.

This command is for distributed mode where:
- Agents (destill-ingest, destill-analyze) are running separately
- Findings are stored in Postgres
- You have a request ID from a previous build submission

Example:
  destill view req-1733769623456789

Environment variables:
  POSTGRES_DSN - Required. Postgres connection string
                 Example: postgres://destill:destill@localhost:5432/destill?sslmode=disable`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		requestID := args[0]

		// Get Postgres DSN from environment
		postgresDSN := os.Getenv("POSTGRES_DSN")
		if postgresDSN == "" {
			fmt.Fprintln(os.Stderr, "ERROR: POSTGRES_DSN environment variable is required")
			fmt.Fprintln(os.Stderr, "Example: export POSTGRES_DSN=\"postgres://destill:destill@localhost:5432/destill?sslmode=disable\"")
			os.Exit(1)
		}

		// Connect to Postgres
		fmt.Printf("Connecting to Postgres...\n")
		postgresStore, err := store.NewPostgresStore(postgresDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to Postgres: %v\n", err)
			os.Exit(1)
		}
		defer postgresStore.Close()

		// Query findings
		ctx := context.Background()
		fmt.Printf("Querying findings for request: %s\n", requestID)
		findings, err := postgresStore.GetFindings(ctx, requestID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to query findings: %v\n", err)
			os.Exit(1)
		}

		if len(findings) == 0 {
			fmt.Printf("\nNo findings found for request: %s\n", requestID)
			fmt.Println("\nPossible reasons:")
			fmt.Println("  • Request ID doesn't exist (check: SELECT * FROM requests;)")
			fmt.Println("  • Analysis hasn't completed yet")
			fmt.Println("  • No errors were found in the build logs")
			os.Exit(0)
		}

		// Convert TriageCardV2 to TriageCard for TUI compatibility
		cards := convertToTriageCards(findings)

		fmt.Printf("\n✅ Found %d findings\n", len(cards))
		fmt.Println("Launching TUI...\n")

		// Launch TUI
		if err := tui.Start(cards); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

// convertToTriageCards converts TriageCardV2 (Postgres format) to TriageCard (TUI format)
func convertToTriageCards(findings []contracts.TriageCardV2) []contracts.TriageCard {
	cards := make([]contracts.TriageCard, len(findings))

	for i, finding := range findings {
		cards[i] = contracts.TriageCard{
			ID:              finding.ID,
			Source:          finding.Source,
			Timestamp:       finding.Timestamp,
			Severity:        finding.Severity,
			Message:         finding.NormalizedMsg,
			RawMessage:      finding.RawMessage,
			Metadata:        finding.Metadata,
			RequestID:       finding.RequestID,
			MessageHash:     finding.MessageHash,
			JobName:         finding.JobName,
			ConfidenceScore: finding.ConfidenceScore,
			PreContext:      strings.Join(finding.PreContext, "\n"),
			PostContext:     strings.Join(finding.PostContext, "\n"),
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
		ctx := context.Background()
		if err := msgBroker.Publish(ctx, "destill_requests", request.RequestID, data); err != nil {
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
		if len(initialCards) == 0 {
			// Streaming mode: pass broker to TUI for live updates
			fmt.Println("🚀 Launching TUI (cards will stream in as they're analyzed)...")
		} else {
			// Cache mode: no streaming, use pre-loaded cards
			fmt.Println("🚀 Launching TUI with cached data...")
		}

		// Brief pause to ensure any remaining log output completes before TUI starts
		time.Sleep(100 * time.Millisecond)

		// Use broker for streaming if no cache, otherwise nil
		var brokerForTUI broker.Broker
		if len(initialCards) == 0 {
			brokerForTUI = msgBroker
		}

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
		if err := ingestionAgent.Run(agentCtx); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := analysis.NewAgent(msgBroker, log)
	go func() {
		if err := analysisAgent.Run(agentCtx); err != nil && err != context.Canceled {
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
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(viewCmd)

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
