# Destill

Destill analyzes CI/CD build logs to surface errors ranked by confidence. It supports Buildkite and GitHub Actions.

## Quick start

### 1. Set up API tokens

```bash
# For Buildkite ('read_builds' and 'read_build_logs' scope)
export BUILDKITE_API_TOKEN="your_token"

# For GitHub Actions (PAT with 'repo' scope)
export GITHUB_TOKEN="your_token"
```

### 2. Install

```bash
make build
make install  # Optional: installs to /usr/local/bin
```

### 3. Analyze a build

```bash
destill analyze "https://buildkite.com/org/pipeline/builds/123"
destill analyze "https://github.com/owner/repo/actions/runs/456"
```

The TUI displays findings sorted by confidence. Use `j/k` to navigate, `0/1/2` to filter by All/Unique/Noise, and `Tab` to cycle jobs.

Use `--json` for machine-readable output.

## MCP server

Destill provides an MCP server for LLM-powered tools like Claude Code.

### Setup

```bash
make build
claude mcp add destill -- /path/to/destill/bin/destill mcp-server
```

Or install globally first:

```bash
make install
claude mcp add destill -- destill mcp-server
```

### Available tools

| Tool | Description |
|------|-------------|
| `analyze_build` | Analyze a build URL and return tiered findings |
| `get_finding_details` | Get full context for a specific finding |

### Example

Ask your assistant:

> Analyze this build: https://buildkite.com/org/pipeline/builds/123

## Configuration

| Variable | Description |
|----------|-------------|
| `BUILDKITE_API_TOKEN` | Buildkite API token with `read_builds` and `read_build_logs` scope|
| `GITHUB_TOKEN` | GitHub PAT with `repo` scope |

## Development

```bash
make build    # Build all binaries
make test     # Run tests
make install  # Install to /usr/local/bin
```

See [ARCHITECTURE.md](./ARCHITECTURE.md) for design details.
