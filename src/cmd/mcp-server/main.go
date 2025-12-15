// Package main provides the MCP server entry point for Destill.
// This server implements the Model Context Protocol, enabling Claude
// to analyze CI/CD builds through the analyze_build tool.
package main

import (
	"context"
	"destill-agent/src/mcp"
	"log"
	"os"
)

func main() {
	// Create MCP server instance
	server := mcp.NewServer()

	// Run server over stdin/stdout (stdio transport)
	if err := server.Run(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
