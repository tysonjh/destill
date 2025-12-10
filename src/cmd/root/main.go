// Package main provides the main entry point for the Destill CLI.
// This orchestrates all subcommands and provides mode detection (Legacy vs Agentic).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "destill",
	Short: "Destill - A log triage tool for CI/CD pipelines",
	Long: `Destill is a decoupled, agent-based log triage tool that helps
analyze and categorize CI/CD build failures.

It supports two modes:
- Legacy Mode: In-memory broker, streaming TUI (default)
- Agentic Mode: Redpanda + Postgres, distributed processing

Mode is auto-detected based on REDPANDA_BROKERS environment variable.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

