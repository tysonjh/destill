// Package main provides the CLI application for the Destill log triage tool.
// This CLI serves as the application orchestrator using the Cobra framework.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"destill-agent/src/broker"
	"destill-agent/src/buildkite"
	"destill-agent/src/contracts"
	"destill-agent/src/mcp"
	analysispipeline "destill-agent/src/pipeline"
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
		// Debug check at very start
		if os.Getenv("DESTILL_DEBUG_ARTIFACTS") != "" {
			fmt.Println("[DEBUG] DESTILL_DEBUG_ARTIFACTS is set")
		}

		buildURL := args[0]
		jsonOutput, _ := cmd.Flags().GetBool("json")
		cacheFile, _ := cmd.Flags().GetString("cache")

		// Validate build URL
		if err := validateBuildURL(buildURL); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		// 1. Setup: Create local mode infrastructure
		mode, err := NewLocalMode()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
			os.Exit(1)
		}
		defer mode.Close()

		// 3. Display: Show results in requested format
		if jsonOutput {
			// 2. Submit: Publish analysis request
			if _, err := mode.SubmitAnalysis(buildURL); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to submit analysis: %v\n", err)
				os.Exit(1)
			}
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

			// For TUI: Subscribe BEFORE submitting to avoid race conditions
			// where warnings are published before the TUI is listening
			var channels *tui.BrokerChannels
			if len(initialCards) == 0 {
				channels, err = tui.SubscribeToBroker(mode.Broker())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to subscribe to broker: %v\n", err)
					os.Exit(1)
				}
			}

			// 2. Submit: Publish analysis request (after subscribing)
			if _, err := mode.SubmitAnalysis(buildURL); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to submit analysis: %v\n", err)
				os.Exit(1)
			}

			if err := displayTUI(channels, initialCards); err != nil {
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

	// Deduplicate by MessageHash, tracking recurrence count
	cards = contracts.DeduplicateCards(cards)
	fmt.Fprintf(os.Stderr, "Deduplicated to %d unique findings\n", len(cards))

	// Sort by confidence score (descending), then recurrence count (descending)
	sort.Slice(cards, func(i, j int) bool {
		if cards[i].ConfidenceScore != cards[j].ConfidenceScore {
			return cards[i].ConfidenceScore > cards[j].ConfidenceScore
		}
		return cards[i].GetRecurrenceCount() > cards[j].GetRecurrenceCount()
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

// mcpServerCmd starts the MCP server for LLM integration.
var mcpServerCmd = &cobra.Command{
	Use:   "mcp-server",
	Short: "Start MCP server for LLM integration",
	Long: `Starts the destill MCP (Model Context Protocol) server.

The MCP server exposes destill's analysis capabilities as tools that can be
invoked by LLMs and coding assistants. It communicates over stdio.

Available tools:
  analyze_build - Analyze a CI/CD build and return tiered findings

Example MCP config for Claude Desktop:
  {
    "mcpServers": {
      "destill": {
        "command": "destill",
        "args": ["mcp-server"]
      }
    }
  }

Environment variables:
  BUILDKITE_API_TOKEN - Required for Buildkite builds
  GITHUB_TOKEN        - Required for GitHub Actions builds`,
	Run: func(cmd *cobra.Command, args []string) {
		st := store.NewInMemoryStore()
		server := mcp.NewServer(st)
		if err := server.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			os.Exit(1)
		}
	},
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

// syncCmd represents the sync command for backfilling test history
var syncCmd = &cobra.Command{
	Use:   "sync [pipeline-url]",
	Short: "Sync test history from recent builds",
	Long: `Syncs test results from recent builds to populate the local SQLite database.

This enables flaky test detection by building up historical test data.

Accepts either a pipeline URL or a specific build URL:
  - Pipeline: https://buildkite.com/org/pipeline/builds?branch=dev
  - Build:    https://buildkite.com/org/pipeline/builds/123

The branch query parameter filters to only sync builds from that branch.

Options:
  -n, --builds    Number of recent builds to sync (default: 20)

Builds are skipped if:
  - They are still running (not finished/failed/passed)
  - They have already been processed (test results exist in SQLite)

Examples:
  destill sync https://buildkite.com/org/pipeline/builds?branch=dev
  destill sync https://buildkite.com/org/pipeline/builds/123 -n 50

Environment variables:
  BUILDKITE_API_TOKEN - Required for Buildkite builds`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pipelineURL := args[0]
		numBuilds, _ := cmd.Flags().GetInt("builds")

		// Try to parse as pipeline URL first (supports branch filter)
		org, pipeline, branch, err := buildkite.ParsePipelineURL(pipelineURL)
		if err != nil {
			// Fall back to build URL format
			org, pipeline, _, err = buildkite.ParseBuildURL(pipelineURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Get API token
		token := os.Getenv("BUILDKITE_API_TOKEN")
		if token == "" {
			fmt.Fprintln(os.Stderr, "ERROR: BUILDKITE_API_TOKEN environment variable is required")
			os.Exit(1)
		}

		// Create Buildkite client
		client := buildkite.NewClient(token)
		ctx := context.Background()

		// Initialize test history database
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get home directory: %v\n", err)
			os.Exit(1)
		}
		dbPath := filepath.Join(homeDir, ".destill", "history.db")
		history, err := store.NewTestHistory(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open test history database: %v\n", err)
			os.Exit(1)
		}
		defer history.Close()

		// Get pipeline ID for checking processed builds
		pipelineID := fmt.Sprintf("%s/%s", org, pipeline)

		// Get already processed builds
		processedBuilds, err := history.GetProcessedBuilds(ctx, pipelineID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get processed builds: %v\n", err)
			os.Exit(1)
		}

		if branch != "" {
			fmt.Printf("Syncing test history for %s/%s (branch: %s)\n", org, pipeline, branch)
		} else {
			fmt.Printf("Syncing test history for %s/%s (all branches)\n", org, pipeline)
		}
		fmt.Printf("Fetching last %d builds...\n", numBuilds)

		// List recent builds with optional branch filter
		var listOpts *buildkite.ListBuildsOptions
		if branch != "" {
			listOpts = &buildkite.ListBuildsOptions{Branch: branch}
		}
		builds, err := client.ListBuilds(ctx, org, pipeline, numBuilds, listOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list builds: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d builds\n", len(builds))

		// Filter and process builds with rate limiting
		var skippedRunning, skippedProcessed, processed, noArtifacts int
		for i, build := range builds {
			// Skip running builds
			if build.State == "running" || build.State == "scheduled" || build.State == "blocked" || build.State == "canceling" {
				skippedRunning++
				continue
			}

			// Skip already processed builds
			if processedBuilds[build.Number] {
				skippedProcessed++
				continue
			}

			// Rate limiting: wait between builds to avoid 429 errors
			if i > 0 {
				time.Sleep(500 * time.Millisecond)
			}

			// Process this build with retry on rate limit
			fmt.Printf("  Processing build #%d (%s)...", build.Number, build.State)

			// Run full analysis pipeline (logs + artifacts) with retry on rate limit
			var analysisResult *analysispipeline.AnalysisResult
			maxRetries := 3

			for attempt := 0; attempt < maxRetries; attempt++ {
				analysisResult, err = analysispipeline.AnalyzeBuild(ctx, build.WebURL)
				if err != nil {
					if strings.Contains(err.Error(), "429") {
						waitTime := time.Duration(attempt+1) * 2 * time.Second
						fmt.Printf(" rate limited, waiting %v...", waitTime)
						time.Sleep(waitTime)
						continue
					}
					break
				}
				break // Success
			}

			if err != nil {
				fmt.Printf(" error: %v\n", err)
				continue
			}

			// Store findings to history (test results are stored by artifact agent)
			pipelineID := org + "/" + pipeline
			if len(analysisResult.Cards) > 0 {
				_ = history.RecordFindingsFromCards(ctx, pipelineID, build.Number, analysisResult.Cards)
			}

			// Report results
			testCount := len(analysisResult.TestResults)
			findingCount := len(analysisResult.Cards)
			if testCount == 0 && findingCount == 0 {
				fmt.Printf(" no data\n")
				noArtifacts++
			} else {
				fmt.Printf(" %d tests, %d findings\n", testCount, findingCount)
				processed++
			}
		}

		fmt.Printf("\nSync complete:\n")
		fmt.Printf("  Processed: %d builds (%d with data)\n", processed+noArtifacts, processed)
		fmt.Printf("  Skipped (running): %d\n", skippedRunning)
		fmt.Printf("  Skipped (already processed): %d\n", skippedProcessed)
		fmt.Printf("\nHistory stored in: %s\n", dbPath)
	},
}

func init() {
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(mcpServerCmd)
	rootCmd.AddCommand(syncCmd)

	// Add flags to analyze command
	analyzeCmd.Flags().BoolP("json", "j", false, "Output findings as JSON instead of launching TUI")
	analyzeCmd.Flags().StringP("cache", "c", "", "Cache file path to load triage cards (speeds up iteration)")

	// Add flags to sync command
	syncCmd.Flags().IntP("builds", "n", 20, "Number of recent builds to sync")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
