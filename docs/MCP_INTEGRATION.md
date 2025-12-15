# MCP Integration - Claude Desktop & Claude Code

This guide explains how to integrate Destill with Claude Desktop and Claude Code using the Model Context Protocol (MCP).

## What is MCP?

The Model Context Protocol (MCP) is a standard protocol that allows Claude to use external tools. Destill provides an MCP server that enables Claude to analyze CI/CD build failures directly.

## Why Use Destill with Claude?

- **AI-Assisted Debugging**: Ask Claude to analyze build failures and get intelligent insights
- **Natural Language Interface**: Just paste a build URL and ask Claude what went wrong
- **Contextual Analysis**: Claude can correlate findings with your codebase and suggest fixes
- **Faster Triage**: Combine Destill's pattern detection with Claude's reasoning

## Prerequisites

- Destill installed and built (see main [README.md](../README.md))
- Claude Desktop or Claude Code (with MCP support)
- API tokens for Buildkite and/or GitHub

## Installation

### 1. Build the MCP Server

The MCP server is a standalone binary that communicates with Claude via stdio:

```bash
# Build the MCP server binary
go build -o ~/.local/bin/destill-mcp src/cmd/mcp-server/main.go

# Verify it was built
ls -la ~/.local/bin/destill-mcp
```

**Note**: You can install it anywhere, but `~/.local/bin/` is recommended.

### 2. Configure Claude Desktop

Claude Desktop reads MCP server configurations from a JSON file.

**Location**: `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS)

Create or edit this file:

```json
{
  "mcpServers": {
    "destill": {
      "command": "/Users/youruser/.local/bin/destill-mcp",
      "env": {
        "BUILDKITE_API_TOKEN": "your-buildkite-token-here",
        "GITHUB_TOKEN": "ghp_your_github_token_here"
      }
    }
  }
}
```

**Important**:
- Replace `/Users/youruser/` with your actual home directory path
- Replace the token values with your actual API tokens
- You can include just one token if you only use one platform

### 3. Configure Claude Code (VS Code Extension)

If you're using Claude Code in VS Code, the configuration is similar but stored in VS Code settings.

**Location**: VS Code Settings → Extensions → Claude → MCP Servers

Or edit `.vscode/settings.json`:

```json
{
  "claude.mcpServers": {
    "destill": {
      "command": "/Users/youruser/.local/bin/destill-mcp",
      "env": {
        "BUILDKITE_API_TOKEN": "your-buildkite-token-here",
        "GITHUB_TOKEN": "ghp_your_github_token_here"
      }
    }
  }
}
```

### 4. Restart Claude

- **Claude Desktop**: Fully quit and restart the application
- **Claude Code**: Reload the VS Code window (`Cmd+Shift+P` → "Reload Window")

### 5. Verify Installation

In Claude, you should see a notification or indicator that the `destill` MCP server is available. You can test it by asking:

```
Do you have access to a tool called analyze_build?
```

Claude should respond confirming it has access to the Destill tool.

## Usage

### Basic Analysis

Simply paste a build URL and ask Claude to analyze it:

**Example 1 - Buildkite:**
```
Analyze this build: https://buildkite.com/myorg/mypipeline/builds/123
```

**Example 2 - GitHub Actions:**
```
What went wrong in this workflow run? https://github.com/owner/repo/actions/runs/456789
```

### Advanced Usage

You can ask Claude to:

**Identify root cause:**
```
This build failed: https://github.com/owner/repo/actions/runs/456789
What's the root cause and how do I fix it?
```

**Compare findings:**
```
Compare these two builds and tell me what changed:
- https://buildkite.com/org/pipeline/builds/100 (passing)
- https://buildkite.com/org/pipeline/builds/101 (failing)
```

**Prioritize issues:**
```
Analyze this build and tell me which issues I should fix first:
https://github.com/owner/repo/actions/runs/789
```

**Generate fix suggestions:**
```
Based on this build failure, suggest code changes to fix it:
https://buildkite.com/org/pipeline/builds/123
```

## How It Works

1. **You ask Claude** to analyze a build URL
2. **Claude calls the MCP tool** - Invokes `analyze_build(url: "...")`
3. **Destill analyzes the build**:
   - Fetches logs and metadata from the CI platform
   - Chunks logs and runs pattern-based analysis
   - Parses JUnit XML artifacts for test failures
   - Returns findings sorted by confidence score
4. **Claude receives findings** as structured JSON
5. **Claude interprets results** and provides insights in natural language

## Available Tools

The Destill MCP server currently provides one tool:

### `analyze_build`

**Description**: Analyze a CI/CD build and return findings sorted by confidence.

**Parameters**:
- `url` (string, required): Build URL from Buildkite or GitHub Actions

**Returns**:
```json
{
  "build_url": "https://github.com/owner/repo/actions/runs/123",
  "findings_count": 15,
  "findings": [
    {
      "severity": "ERROR",
      "message": "npm ERR! code ENOENT",
      "confidence": 0.85,
      "job": "build-and-test"
    }
    // ... more findings
  ]
}
```

## Troubleshooting

### "MCP server not found" or "Tool unavailable"

**Problem**: Claude can't connect to the Destill MCP server.

**Solutions**:
1. Verify the binary exists:
   ```bash
   ls -la ~/.local/bin/destill-mcp
   ```
2. Check the path in your config matches the binary location
3. Ensure the binary has execute permissions:
   ```bash
   chmod +x ~/.local/bin/destill-mcp
   ```
4. Restart Claude completely

### "Authentication failed" from Destill

**Problem**: API tokens are missing or invalid.

**Solutions**:
1. Verify tokens are set in the MCP config (not your shell environment)
2. Check tokens are valid and have correct permissions:
   - Buildkite: Organization access
   - GitHub: `repo` scope
3. Update the config file and restart Claude

### "Build not found" or "Invalid URL"

**Problem**: The build URL is malformed or inaccessible.

**Solutions**:
1. Verify the URL format:
   - Buildkite: `https://buildkite.com/org/pipeline/builds/123`
   - GitHub: `https://github.com/owner/repo/actions/runs/456`
2. Check that your token has access to the repository/organization
3. Ensure the build exists and hasn't been deleted

### Slow response times

**Problem**: Analysis takes a long time for large builds.

**Solutions**:
- This is expected for builds with many jobs or large logs
- The MCP server processes everything locally (no persistence)
- For repeated analysis, use `destill analyze` CLI with `--cache` flag
- Consider using distributed mode for better performance

### No findings returned

**Problem**: Destill returns zero findings.

**Possible reasons**:
1. Build passed successfully (no errors to find)
2. Logs don't match error patterns (false negatives)
3. JUnit artifacts not uploaded or not in expected format

**Debug steps**:
1. Run the same build URL with CLI to see verbose output:
   ```bash
   ./bin/destill analyze "https://..."
   ```
2. Check if there are actual errors in the build logs
3. Verify JUnit XML artifacts are uploaded correctly

## Security Considerations

### Token Safety

- **Never commit** the `claude_desktop_config.json` file with real tokens
- **Use environment variables** if possible (MCP supports this)
- **Rotate tokens regularly** as a security best practice
- **Limit token scope** to only what Destill needs:
  - Buildkite: Read-only organization access
  - GitHub: `repo` scope (required for private repos and Actions)

### Local Processing

- The MCP server runs **locally on your machine**
- Build data is **never sent to external services** (only fetched from CI platforms)
- Findings stay **local** (no cloud persistence unless you use distributed mode)

## Advanced Configuration

### Custom Binary Path

If you built the binary elsewhere, update the path:

```json
{
  "mcpServers": {
    "destill": {
      "command": "/custom/path/to/destill-mcp",
      "env": { ... }
    }
  }
}
```

### Multiple Configurations

You can configure multiple MCP servers (e.g., one per organization):

```json
{
  "mcpServers": {
    "destill-org1": {
      "command": "/Users/youruser/.local/bin/destill-mcp",
      "env": {
        "BUILDKITE_API_TOKEN": "token-for-org1",
        "GITHUB_TOKEN": "token-for-org1"
      }
    },
    "destill-org2": {
      "command": "/Users/youruser/.local/bin/destill-mcp",
      "env": {
        "BUILDKITE_API_TOKEN": "token-for-org2",
        "GITHUB_TOKEN": "token-for-org2"
      }
    }
  }
}
```

### Logging and Debugging

To debug the MCP server, run it manually:

```bash
# Test the MCP server with a sample request
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | ~/.local/bin/destill-mcp
```

Expected output:
```json
{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"analyze_build","description":"..."}]}}
```

## Example Workflow

Here's a complete workflow using Destill + Claude:

1. **Build fails in CI** (you get a notification)
2. **Copy the build URL** from Slack/email/GitHub
3. **Open Claude Desktop** or Claude Code
4. **Ask Claude**: "What went wrong in this build? [paste URL]"
5. **Claude analyzes** the build using Destill
6. **Claude explains** the root cause in plain English
7. **Claude suggests fixes** based on the error patterns
8. **You apply fixes** and re-run the build

## Limitations

- **No streaming**: MCP calls are request/response (no real-time updates)
- **No persistence**: Each analysis is fresh (no caching between Claude sessions)
- **Single build**: Currently analyzes one build at a time (no batch processing)
- **No write operations**: Destill is read-only (can't trigger builds or modify CI configs)

## Future Enhancements

Planned features for the MCP integration:

- [ ] Batch analysis of multiple builds
- [ ] Historical trend analysis
- [ ] Integration with Destill's distributed mode (query cached findings)
- [ ] Build comparison tool
- [ ] Custom pattern rules via MCP

## Next Steps

- Read [GITHUB_ACTIONS.md](./GITHUB_ACTIONS.md) for GitHub Actions setup details
- See main [README.md](../README.md) for CLI usage and distributed mode
- Check [ARCHITECTURE.md](../ARCHITECTURE.md) to understand how Destill works

## Feedback

If you encounter issues or have suggestions for the MCP integration, please open an issue on GitHub.
