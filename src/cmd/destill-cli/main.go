// Package main provides the unified Destill CLI with mode detection.
package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"destill-agent/src/config"
	"destill-agent/src/contracts"
	"destill-agent/src/pipeline"
	"destill-agent/src/store"
	"destill-agent/src/tui"
)

var (
	appConfig *config.Config
	mode      pipeline.Mode
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "destill",
	Short: "Destill - A log triage tool for CI/CD pipelines",
	Long: `Destill is an agent-based log triage tool that helps analyze and
categorize CI/CD build failures.

It supports two modes:
- Legacy Mode: In-memory broker, streaming TUI (default)
- Agentic Mode: Redpanda + Postgres, distributed processing

Mode is auto-detected based on REDPANDA_BROKERS environment variable.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		var err error
		appConfig, err = config.LoadFromEnv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
			os.Exit(1)
		}

		// Detect mode
		pipelineCfg := &pipeline.Config{
			RedpandaBrokers: appConfig.RedpandaBrokers,
			PostgresDSN:     appConfig.PostgresDSN,
			BuildkiteToken:  appConfig.BuildkiteAPIToken,
		}
		mode = pipeline.DetectMode(pipelineCfg)
	},
}

// runCmd submits a build for analysis
var runCmd = &cobra.Command{
	Use:   "run [build-url]",
	Short: "Analyze a Buildkite build",
	Long: `Submit a Buildkite build for analysis.

Legacy Mode (default): Runs complete pipeline in-process with streaming TUI
Agentic Mode: Submits request to Redpanda and returns request ID

Set REDPANDA_BROKERS to enable agentic mode.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		buildURL := args[0]
		ctx := context.Background()

		pipelineCfg := &pipeline.Config{
			RedpandaBrokers: appConfig.RedpandaBrokers,
			PostgresDSN:     appConfig.PostgresDSN,
			BuildkiteToken:  appConfig.BuildkiteAPIToken,
		}

		switch mode {
		case pipeline.LegacyMode:
			runLegacyMode(ctx, pipelineCfg, buildURL)
		case pipeline.AgenticMode:
			runAgenticMode(ctx, pipelineCfg, buildURL)
		}
	},
}

func runLegacyMode(ctx context.Context, cfg *pipeline.Config, buildURL string) {
	fmt.Println("🔧 Running in Legacy Mode (in-memory)")
	fmt.Println("💡 Tip: Set REDPANDA_BROKERS for distributed agentic mode")
	fmt.Println()

	// Create legacy pipeline
	p, err := pipeline.NewLegacyPipeline(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create pipeline: %v\n", err)
		os.Exit(1)
	}
	defer p.Close()

	// Note: Full legacy integration will be in the existing CLI
	// For now, just show the mode
	fmt.Println("⚠️  Legacy mode TUI integration not yet wired up")
	fmt.Println("   Use the existing 'destill build' command from src/cmd/cli")
}

func runAgenticMode(ctx context.Context, cfg *pipeline.Config, buildURL string) {
	fmt.Println("🚀 Running in Agentic Mode (distributed)")
	fmt.Println()

	// Create agentic pipeline
	p, err := pipeline.NewAgenticPipeline(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create pipeline: %v\n", err)
		os.Exit(1)
	}
	defer p.Close()

	// Submit request
	requestID, err := p.Submit(ctx, buildURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to submit request: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Submitted analysis request: %s\n", requestID)
	fmt.Printf("   Build URL: %s\n", buildURL)
	fmt.Println()
	fmt.Println("📊 The ingest and analyze agents will process this build.")
	fmt.Println("   Findings will be stored in Postgres.")
	fmt.Println()
	fmt.Printf("View results: destill view %s\n", requestID)
	fmt.Printf("Check status:  destill status %s\n", requestID)
}

// viewCmd displays findings from Postgres
var viewCmd = &cobra.Command{
	Use:   "view [request-id]",
	Short: "View analysis findings from Postgres",
	Long: `Query Postgres for analysis findings and display them in the TUI.

This command requires agentic mode (REDPANDA_BROKERS and POSTGRES_DSN must be set).`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		requestID := args[0]
		ctx := context.Background()

		if appConfig.PostgresDSN == "" {
			fmt.Fprintln(os.Stderr, "ERROR: POSTGRES_DSN environment variable is required for view command")
			os.Exit(1)
		}

		// Create store
		st, err := store.NewPostgresStore(appConfig.PostgresDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to Postgres: %v\n", err)
			os.Exit(1)
		}
		defer st.Close()

		// Get findings
		findings, err := st.GetFindings(ctx, requestID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get findings: %v\n", err)
			os.Exit(1)
		}

		if len(findings) == 0 {
			fmt.Printf("No findings found for request: %s\n", requestID)
			fmt.Println()
			fmt.Println("💡 Tips:")
			fmt.Println("   • Check if the request ID is correct")
			fmt.Println("   • Ensure ingest and analyze agents are running")
			fmt.Println("   • Wait a moment for analysis to complete")
			return
		}

		// Convert to legacy format for TUI
		cards := convertToLegacyFormat(findings)

		// Sort by confidence
		sort.Slice(cards, func(i, j int) bool {
			return cards[i].ConfidenceScore > cards[j].ConfidenceScore
		})

		fmt.Printf("📊 Found %d findings for request: %s\n", len(findings), requestID)
		fmt.Println("Launching TUI...")
		fmt.Println()

		// Launch TUI
		if err := tui.Start(cards); err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			os.Exit(1)
		}
	},
}

// statusCmd shows request status
var statusCmd = &cobra.Command{
	Use:   "status [request-id]",
	Short: "Check the status of an analysis request",
	Long:  `Query Postgres for the status of an analysis request.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		requestID := args[0]
		ctx := context.Background()

		if appConfig.PostgresDSN == "" {
			fmt.Fprintln(os.Stderr, "ERROR: POSTGRES_DSN environment variable is required for status command")
			os.Exit(1)
		}

		// Create store
		st, err := store.NewPostgresStore(appConfig.PostgresDSN)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to connect to Postgres: %v\n", err)
			os.Exit(1)
		}
		defer st.Close()

		// Get status
		status, err := st.GetRequestStatus(ctx, requestID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
			os.Exit(1)
		}

		// Display status
		fmt.Printf("Request ID:     %s\n", status.RequestID)
		fmt.Printf("Build URL:      %s\n", status.BuildURL)
		fmt.Printf("Status:         %s\n", status.Status)
		fmt.Printf("Chunks Total:   %d\n", status.ChunksTotal)
		fmt.Printf("Chunks Processed: %d\n", status.ChunksProcessed)
		fmt.Printf("Findings:       %d\n", status.FindingsCount)
	},
}

// convertToLegacyFormat converts TriageCardV2 to legacy TriageCard for TUI
func convertToLegacyFormat(findings []contracts.TriageCardV2) []contracts.TriageCard {
	cards := make([]contracts.TriageCard, len(findings))
	for i, f := range findings {
		// Convert array context to string
		preContext := ""
		if len(f.PreContext) > 0 {
			for _, line := range f.PreContext {
				preContext += line + "\n"
			}
		}

		postContext := ""
		if len(f.PostContext) > 0 {
			for _, line := range f.PostContext {
				postContext += line + "\n"
			}
		}

		cards[i] = contracts.TriageCard{
			ID:              f.ID,
			RequestID:       f.RequestID,
			MessageHash:     f.MessageHash,
			Source:          f.Source,
			JobName:         f.JobName,
			Severity:        f.Severity,
			Message:         f.NormalizedMsg,
			RawMessage:      f.RawMessage,
			Metadata:        f.Metadata,
			ConfidenceScore: f.ConfidenceScore,
			PreContext:      preContext,
			PostContext:     postContext,
			Timestamp:       f.Timestamp,
		}
	}
	return cards
}

func init() {
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(viewCmd)
	rootCmd.AddCommand(statusCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

