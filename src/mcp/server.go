// Package mcp implements the Model Context Protocol server for Destill.
// This server exposes tools that Claude can use to analyze CI/CD builds.
package mcp

import (
	"context"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP SDK server with Destill-specific tools
type Server struct {
	server *mcp.Server
}

// NewServer creates a new MCP server with all Destill tools registered
func NewServer() *Server {
	// Create implementation metadata
	impl := &mcp.Implementation{
		Name:    "destill",
		Version: "1.0.0",
	}

	// Create server options (nil for default options)
	var options *mcp.ServerOptions = nil

	// Create the MCP server
	mcpServer := mcp.NewServer(impl, options)

	// Register tools
	RegisterAnalyzeBuildTool(mcpServer)

	return &Server{
		server: mcpServer,
	}
}

// Run starts the MCP server over stdin/stdout
func (s *Server) Run(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	// Create stdio transport
	transport := &mcp.StdioTransport{}

	// Run the server
	return s.server.Run(ctx, transport)
}
