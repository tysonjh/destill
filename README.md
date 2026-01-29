# Destill

Destill is an MCP server that gives LLMs the ability to analyze CI/CD build failures. Point your AI assistant at a failing build URL and get structured, confidence-ranked findings that identify likely root causes.

## Why Destill?

Build logs are noisy. A single failed CI run can produce megabytes of output across dozens of jobs. Destill processes this chaos and returns what matters:

- **Tiered findings** - Unique failures ranked by confidence, separated from common noise
- **Flaky test detection** - Tests that fail intermittently are flagged based on historical patterns
- **Novel failure identification** - New errors that haven't appeared in recent builds are highlighted
- **Token-efficient responses** - Compressed output designed for LLM context windows

## Quick start

### 1. Set up API tokens

```bash
# For Buildkite (requires 'read_builds' and 'read_build_logs' scopes)
export BUILDKITE_API_TOKEN="your_token"

# For GitHub Actions (PAT with 'repo' scope)
export GITHUB_TOKEN="your_token"
```

### 2. Install

```bash
make build
make install  # Optional: installs to /usr/local/bin
```

### 3. Add to Claude Code

```bash
claude mcp add destill -- destill mcp-server
```

Or with explicit path:

```bash
claude mcp add destill -- /path/to/destill/bin/destill mcp-server
```

### 4. Analyze builds

Ask your assistant:

> Analyze this build: https://github.com/owner/repo/actions/runs/123456

The assistant receives structured JSON with build metadata, test results, and ranked findings.

## MCP tools

| Tool | Description |
|------|-------------|
| `analyze_build` | Analyze a build URL. Returns build info, test summary, and tiered findings. Tier 1 findings include full context. |
| `get_finding_details` | Drill into a specific finding by ID. Returns complete context lines for deeper investigation. |

### Response structure

The `analyze_build` response includes:

- **Build metadata** - Status, branch, commit, duration, failed/passed job counts
- **Test summary** - Total/passed/failed counts, novel vs flaky failure classification
- **Tier 1 findings** - Unique failures with context, ranked by confidence (0.0-1.0)
- **Tier 2-3 summaries** - Lower-priority findings available for drill-down

## CLI usage

Destill also provides a terminal interface for direct analysis:

```bash
destill analyze "https://buildkite.com/org/pipeline/builds/123"
destill analyze "https://github.com/owner/repo/actions/runs/456"
```

Use `--json` for machine-readable output.

## Configuration

| Variable | Description |
|----------|-------------|
| `BUILDKITE_API_TOKEN` | Buildkite API token with `read_builds` and `read_build_logs` scopes |
| `GITHUB_TOKEN` | GitHub PAT with `repo` scope |
| `ARTIFACT_SERVER_USER` | Username for custom artifact servers requiring Basic auth |
| `ARTIFACT_SERVER_PASSWORD` | Password for custom artifact servers requiring Basic auth |
| `DESTILL_DOWNLOAD_LOGS` | Enable downloading artifact log files (`true`/`false`, default: `false`) |
| `DESTILL_MAX_LOG_FILE_SIZE` | Max size per log file in bytes (default: 10485760 = 10MB) |
| `DESTILL_MAX_TOTAL_LOG_SIZE` | Max total size for all log files in bytes (default: 52428800 = 50MB) |
| `DESTILL_DEBUG_ARTIFACTS` | Enable debug logging to `/tmp/destill-debug.log` |

### Artifact Log Downloads

When `DESTILL_DOWNLOAD_LOGS=true`, destill will:
- Download log files (`.log` and `.log.gz`) from artifacts that match failed test names
- Decompress gzipped logs automatically
- Save both original and decompressed files to `~/.destill/cache/<request-id>/artifacts/`
- Analyze log content using the same pattern-based analysis as job logs
- Apply size limits to prevent excessive downloads

## Development

```bash
make build    # Build all binaries
make test     # Run tests
make install  # Install to /usr/local/bin
```

See [ARCHITECTURE.md](./ARCHITECTURE.md) for design details.
