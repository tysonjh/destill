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

	"destill-agent/src/analyze"
	"destill-agent/src/broker"
	"destill-agent/src/contracts"
	"destill-agent/src/ingest"
	"destill-agent/src/logger"
	"destill-agent/src/provider"
	"destill-agent/src/store"
	"destill-agent/src/tui"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "destill",
	Short: "Destill - A build failure triage tool for CI/CD pipelines",
	Long: `Destill is a build failure triage tool for CI/CD pipelines that can be 
run as a single, in-memory tool, or deployed as a series of binaries with 
redpanda and postgres.`,
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
			// TODO: when no error is found, we should know this definitively and tell the user.
			fmt.Printf("\nNo findings found for request: %s\n", requestID)
			fmt.Println("\nPossible reasons:")
			fmt.Println("  • Request ID doesn't exist (check: SELECT * FROM requests;)")
			fmt.Println("  • Analysis hasn't completed yet")
			fmt.Println("  • No errors were found in the build logs")
			os.Exit(0)
		}

		fmt.Printf("\n✅ Found %d findings\n", len(findings))
		fmt.Println("Launching TUI...")

		// Launch TUI with TriageCard directly
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
	Short: "Analyze a CI/CD build locally with streaming TUI",
	Long: `Analyzes a CI/CD build in local mode using in-memory processing.
All analysis happens in a single process with agents running as goroutines.

Supports:
  - Buildkite: https://buildkite.com/org/pipeline/builds/123 (requires BUILDKITE_API_TOKEN)
  - GitHub Actions: https://github.com/owner/repo/actions/runs/456 (requires GITHUB_TOKEN)

By default: Launches the TUI immediately. Cards appear in real-time as they are
analyzed. Press 'r' to refresh/re-rank the list when new cards arrive.

With --json: Outputs findings as JSON instead of launching TUI.

With --cache: Load previously saved cards from a JSON file for fast iteration
during development.

This is the simplest mode - no infrastructure required, just the CLI binary.

Examples:
  destill analyze https://buildkite.com/org/pipeline/builds/4091
  destill analyze https://github.com/owner/repo/actions/runs/123456
  destill analyze https://buildkite.com/org/pipeline/builds/4091 --json
  destill analyze https://buildkite.com/org/pipeline/builds/4091 --cache build.json`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		cacheFile, _ := cmd.Flags().GetString("cache")

		// Validate build URL
		if err := validateBuildURL(buildURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// 1. Setup: Create local mode infrastructure
		mode := NewLocalMode()
		defer mode.Close()

		// 2. Submit: Publish analysis request
		if _, err := mode.SubmitAnalysis(buildURL); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to submit analysis: %v\n", err)
			os.Exit(1)
		}

		// 3. Display: Show results in requested format
		if jsonOutput {
			// JSON output: collect and display findings
			if err := displayJSON(mode.Broker()); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		} else {
			// TUI output: load cache (if any) and display interactively
			initialCards, err := loadCachedCards(cacheFile)
			if err != nil {
				// Non-fatal - just log and continue without cache
				fmt.Fprintf(os.Stderr, "Warning: failed to load cache: %v\n", err)
				initialCards = []contracts.TriageCard{}
			}
			if len(initialCards) > 0 {
				fmt.Printf("📂 Loaded %d cards from cache: %s\n", len(initialCards), cacheFile)
			}

			if err := displayTUI(mode.Broker(), initialCards); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// collectAndOutputJSON subscribes to findings and collects results until idle timeout.
// The request must already be published before calling this function.
func collectAndOutputJSON(ctx context.Context, msgBroker broker.Broker) error {
	// Subscribe to findings
	cardChan, err := msgBroker.Subscribe(ctx, contracts.TopicAnalysisFindings, "json-output-consumer")
	if err != nil {
		return fmt.Errorf("failed to subscribe to findings: %w", err)
	}

	// Initialize as empty slice (not nil) so JSON marshals to [] not null
	cards := []contracts.TriageCard{}

	// Use a timeout to detect when analysis is complete
	// If no new findings arrive for this duration, we consider analysis done
	idleTimeout := 10 * time.Second
	fmt.Fprintf(os.Stderr, "Waiting for findings (will timeout after %v of inactivity)...\n", idleTimeout)
	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()

	// Collect findings until idle timeout
collectLoop:
	for {
		select {
		case msg, ok := <-cardChan:
			if !ok {
				// Channel closed, we're done
				break collectLoop
			}

			var card contracts.TriageCard
			if err := json.Unmarshal(msg.Value, &card); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to unmarshal card: %v\n", err)
				continue
			}
			cards = append(cards, card)
			fmt.Fprintf(os.Stderr, "\rCollecting findings... %d received", len(cards))

			// Reset timer on each new finding
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)

		case <-timer.C:
			// No findings for idleTimeout period, analysis is complete
			break collectLoop
		}
	}

	fmt.Fprintf(os.Stderr, "\nCollected %d findings\n", len(cards))

	// Sort by confidence score (descending)
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
			return cards[i].ConfidenceScore > cards[j].ConfidenceScore
		}
		countI := getRecurrenceCount(cards[i].Metadata)
		countJ := getRecurrenceCount(cards[j].Metadata)
		return countI > countJ
	})

	// Print job summary header to stderr (before JSON output)
	printJobSummary(cards)

	// Output as JSON
	output, err := json.MarshalIndent(cards, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal findings to JSON: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

// printJobSummary outputs a summary of jobs by status to stderr.
// This helps users quickly identify which jobs failed without parsing the full JSON.
func printJobSummary(cards []contracts.TriageCard) {
	// Track unique jobs and their states
	type jobInfo struct {
		name   string
		state  string
		status string
	}
	jobMap := make(map[string]jobInfo)

	for _, card := range cards {
		jobName := card.JobName
		if jobName == "" {
			continue
		}

		// Only record each job once (first occurrence)
		if _, exists := jobMap[jobName]; !exists {
			state := card.Metadata["job_state"]
			status := card.Metadata["exit_status"]
			jobMap[jobName] = jobInfo{name: jobName, state: state, status: status}
		}
	}

	if len(jobMap) == 0 {
		return
	}

	// Count by state
	var failedJobs []string
	var passedJobs []string

	for _, info := range jobMap {
		if info.state == "failed" || (info.status != "" && info.status != "0") {
			failedJobs = append(failedJobs, info.name)
		} else {
			passedJobs = append(passedJobs, info.name)
		}
	}

	// Sort for consistent output
	sort.Strings(failedJobs)
	sort.Strings(passedJobs)

	// Print summary
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "Job Summary: %d failed, %d passed\n", len(failedJobs), len(passedJobs))

	if len(failedJobs) > 0 {
		fmt.Fprintf(os.Stderr, "Failed jobs:\n")
		for _, name := range failedJobs {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", name)
		}
	}

	fmt.Fprintf(os.Stderr, "\n")
}

// startStreamPipeline launches the Ingestion and Analysis agents as persistent Go routines.
// The agents run indefinitely until the broker is closed.
// Uses silent logger to prevent log output from interfering with TUI display.
func startStreamPipeline(msgBroker broker.Broker, agentCtx context.Context) {
	// Use silent logger to prevent log pollution in TUI mode
	log := logger.NewSilentLogger()

	// Start Ingestion Agent as a persistent goroutine
	ingestionAgent := ingest.NewAgent(msgBroker, log)
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
	Long: `Submits a CI/CD build URL for analysis in distributed mode.
This command publishes the request to Redpanda and returns immediately.

Supports:
  - Buildkite: https://buildkite.com/org/pipeline/builds/123 (requires BUILDKITE_API_TOKEN)
  - GitHub Actions: https://github.com/owner/repo/actions/runs/456 (requires GITHUB_TOKEN)

Requires:
- destill-ingest agent running (processes requests and fetches logs)
- destill-analyze agent running (analyzes logs and produces findings)
- Redpanda broker running
- Postgres database running

The request is queued and processed asynchronously by the agents.
Use 'destill view <request-id>' to see results once processing is complete.

Examples:
  destill submit https://buildkite.com/org/pipeline/builds/4091
  destill submit https://github.com/owner/repo/actions/runs/123456

Environment variables:
  BUILDKITE_API_TOKEN - Required for Buildkite builds
  GITHUB_TOKEN        - Required for GitHub Actions builds
  REDPANDA_BROKERS    - Required. Comma-separated broker addresses
  POSTGRES_DSN        - Required. Postgres connection string`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]

		// Validate the URL first to provide helpful error messages early
		if _, err := provider.ParseURL(buildURL); err != nil {
			userErr := provider.WrapError(err)
			fmt.Fprintf(os.Stderr, "Error: %v\n", userErr)
			os.Exit(1)
		}

		// Get Redpanda brokers from environment for distributed mode
		redpandaBrokersStr := os.Getenv("REDPANDA_BROKERS")
		if redpandaBrokersStr == "" {
			fmt.Fprintln(os.Stderr, "ERROR: REDPANDA_BROKERS environment variable is required for distributed mode")
			fmt.Fprintln(os.Stderr, "Example: export REDPANDA_BROKERS=\"localhost:9092\"")
			os.Exit(1)
		}

		// Parse comma-separated broker addresses
		redpandaBrokers := strings.Split(redpandaBrokersStr, ",")
		for i := range redpandaBrokers {
			redpandaBrokers[i] = strings.TrimSpace(redpandaBrokers[i])
		}

		// Initialize Redpanda broker for distributed mode
		msgBroker, err := broker.NewRedpandaBroker(redpandaBrokers)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to Redpanda: %v\n", err)
			os.Exit(1)
		}
		defer msgBroker.Close()

		// Create analysis request
		requestID, requestData, err := buildAnalysisRequest(buildURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create request: %v\n", err)
			os.Exit(1)
		}

		// Publish to destill.requests topic
		ctx := context.Background()
		if err := msgBroker.Publish(ctx, contracts.TopicRequests, requestID, requestData); err != nil {
			fmt.Fprintf(os.Stderr, "Error publishing request: %v\n", err)
			os.Exit(1)
		}

		// Print success message
		fmt.Printf("✅ Submitted analysis request: %s\n", requestID)
		fmt.Printf("   Build URL: %s\n\n", buildURL)
		fmt.Println("📊 The ingest and analyze agents will process this build.")
		fmt.Println("   Findings will be stored in Postgres.")
		fmt.Printf("\nView results: destill view %s\n", requestID)
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
