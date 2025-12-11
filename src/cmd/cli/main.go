// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"destill-agent/src/analyze"
	"destill-agent/src/broker"
	"destill-agent/src/config"
	"destill-agent/src/contracts"
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
	"destill-agent/src/store"
	"destill-agent/src/tui"
)

var (
	// Shared message broker for all agents
	msgBroker broker.Broker
	// Application configuration
	appConfig *config.Config
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

		// Initialize the broker before any command runs
		msgBroker = broker.NewInMemoryBroker()

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
	Use:   "view <request-id-or-url>",
	Short: "View findings from Postgres in TUI (distributed mode)",
	Long: `Queries Postgres for findings and displays them in an interactive TUI.

This command is for distributed mode where:
- Agents (destill-ingest, destill-analyze) are running separately
- Findings are stored in Postgres
- You have a request ID from a previous build submission OR a build URL

If you provide a build URL, it will automatically find the most recent request
for that build.

Examples:
  destill view req-1733769623456789
  destill view https://buildkite.com/org/pipeline/builds/123

Environment variables:
  POSTGRES_DSN - Required. Postgres connection string
                 Example: postgres://destill:destill@localhost:5432/destill?sslmode=disable`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		arg := args[0]

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

		ctx := context.Background()

		// Detect if arg is a URL or request ID
		var requestID string
		if isURL(arg) {
			// It's a build URL - find the latest request
			fmt.Printf("Looking up latest request for build URL...\n")
			requestID, err = postgresStore.GetLatestRequestByBuildURL(ctx, arg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to find request for build URL: %v\n", err)
				fmt.Fprintln(os.Stderr, "\nPossible reasons:")
				fmt.Fprintln(os.Stderr, "  • No request exists for this build URL")
				fmt.Fprintln(os.Stderr, "  • Use 'destill submit <url>' to submit a new analysis")
				os.Exit(1)
			}
			fmt.Printf("Found request: %s\n", requestID)
		} else {
			// It's a request ID
			requestID = arg
		}

		// Query findings
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

		fmt.Printf("\n✅ Found %d findings\n", len(findings))
		fmt.Println("Launching TUI...")

		// Launch TUI with TriageCardV2 directly
		if err := tui.Start(findings); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

// isURL checks if a string looks like a URL
func isURL(s string) bool {
	return len(s) > 4 && (s[:4] == "http" || s[:5] == "https")
}

// getRecurrenceCount extracts the recurrence count from metadata
func getRecurrenceCount(metadata map[string]string) int {
	if metadata == nil {
		return 1
	}
	if count, ok := metadata["recurrence_count"]; ok {
		var c int
		if _, err := fmt.Sscanf(count, "%d", &c); err == nil {
			return c
		}
	}
	return 1
}

// analyzeCmd represents the analyze command (local mode)
var analyzeCmd = &cobra.Command{
	Use:   "analyze [build-url]",
	Short: "Analyze a Buildkite build locally with streaming TUI",
	Long: `Analyzes a Buildkite build in local mode using in-memory processing.
All analysis happens in a single process with agents running as goroutines.

By default: Launches the TUI immediately. Cards appear in real-time as they are
analyzed. Press 'r' to refresh/re-rank the list when new cards arrive.

With --json: Outputs findings as JSON instead of launching TUI.

With --cache: Load previously saved cards from a JSON file for fast iteration
during development.

This is the simplest mode - no infrastructure required, just the CLI binary.

Example:
  destill analyze https://buildkite.com/org/pipeline/builds/4091
  destill analyze https://buildkite.com/org/pipeline/builds/4091 --json
  destill analyze https://buildkite.com/org/pipeline/builds/4091 --cache build.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		cacheFile, _ := cmd.Flags().GetString("cache")

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

		// Publish to destill.requests topic
		ctx := context.Background()
		if err := msgBroker.Publish(ctx, contracts.TopicRequests, request.RequestID, data); err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing request: %v\n", err)
			os.Exit(1)
		}

		// Check for cache flag
		var initialCards []contracts.TriageCardV2
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
				countI := getRecurrenceCount(initialCards[i].Metadata)
				countJ := getRecurrenceCount(initialCards[j].Metadata)
				return countI > countJ
			})
		}

		// JSON output mode
		if jsonOutput {
			// TODO: Implement JSON output - would need to collect cards from broker
			fmt.Fprintln(os.Stderr, "ERROR: --json flag not yet implemented")
			fmt.Fprintln(os.Stderr, "Use TUI mode (default) or --cache for now")
			os.Exit(1)
		}

		// Launch the streaming TUI
		if len(initialCards) == 0 {
			fmt.Println("🚀 Launching TUI (cards will stream in as they're analyzed)...")
		} else {
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
	},
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
// Uses silent logger to prevent log output from interfering with TUI display.
func startStreamPipeline() {
	// Use silent logger to prevent log pollution in TUI mode
	log := logger.NewSilentLogger()

	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingest.NewAgent(msgBroker, appConfig.BuildkiteAPIToken, log)
	go func() {
		if err := ingestionAgent.Run(agentCtx); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Ingestion agent error: %v\n", err)
		}
	}()

	// Start Analysis Agent as a persistent goroutine
	analysisAgent := analyze.NewAgent(msgBroker, log)
	go func() {
		if err := analysisAgent.Run(agentCtx); err != nil && err != context.Canceled {
			// Error logging always goes to stderr even in silent mode
			fmt.Fprintf(os.Stderr, "[Pipeline] Analysis agent error: %v\n", err)
		}
	}()
}

// submitCmd represents the submit command (distributed mode)
var submitCmd = &cobra.Command{
	Use:   "submit [build-url]",
	Short: "Submit a build for analysis in distributed mode",
	Long: `Submits a Buildkite build URL for analysis in distributed mode.
This command publishes the request to Redpanda and returns immediately.

Requires:
- destill-ingest agent running (processes requests and fetches logs)
- destill-analyze agent running (analyzes logs and produces findings)
- Redpanda broker running
- Postgres database running

The request is queued and processed asynchronously by the agents.
Use 'destill view <request-id>' to see results once processing is complete.

Example:
  destill submit https://buildkite.com/org/pipeline/builds/4091

Environment variables:
  BUILDKITE_API_TOKEN - Required. Buildkite API token
  REDPANDA_BROKERS    - Required. Comma-separated broker addresses
  POSTGRES_DSN        - Required. Postgres connection string`,
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

		// Publish to destill.requests topic
		ctx := context.Background()
		if err := msgBroker.Publish(ctx, contracts.TopicRequests, request.RequestID, data); err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing request: %v\n", err)
			os.Exit(1)
		}

		// Print success message
		fmt.Printf("✅ Submitted analysis request: %s\n", request.RequestID)
		fmt.Printf("   Build URL: %s\n\n", buildURL)
		fmt.Println("📊 The ingest and analyze agents will process this build.")
		fmt.Println("   Findings will be stored in Postgres.")
		fmt.Printf("\nView results: destill view %s\n", request.RequestID)
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(viewCmd)

	// Add flags to analyze command
	analyzeCmd.Flags().BoolP("json", "j", false, "Output findings as JSON instead of launching TUI")
	analyzeCmd.Flags().StringP("cache", "c", "", "Cache file path to load triage cards (speeds up iteration)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
