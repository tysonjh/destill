# Destill

Destill analyzes CI/CD build logs to surface errors ranked by confidence. It supports Buildkite and GitHub Actions.

## Quick start

### 1. Set up API tokens

```bash
# For Buildkite
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

The TUI displays findings sorted by confidence. Errors from failed jobs receive boosted confidence scores.

Use `--json` for machine-readable output (e.g. Claude Code or Gemini).

## Modes

### Local mode (default)

Runs entirely in-memory. No infrastructure required.

```bash
destill analyze <url>
```

### Distributed mode

Persists findings to Postgres via Redpanda. Supports horizontal scaling.

```bash
# Start infrastructure
cd docker && docker-compose up -d

# Set environment
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"

# Run agents (separate terminals)
./bin/destill-ingest
./bin/destill-analyze

# Submit and view
./bin/destill submit <url>
./bin/destill view <request-id>
```

See [QUICK_START_DISTRIBUTED.md](./QUICK_START_DISTRIBUTED.md) for setup.

## Configuration

| Variable | Description |
|----------|-------------|
| `BUILDKITE_API_TOKEN` | Buildkite API token |
| `GITHUB_TOKEN` | GitHub PAT with `repo` scope |
| `REDPANDA_BROKERS` | Broker addresses (distributed mode) |
| `POSTGRES_DSN` | Postgres connection string (distributed mode) |

## Architecture

See [ARCHITECTURE.md](./ARCHITECTURE.md) for design principles.

## Development

```bash
make build    # Build all binaries
make test     # Run tests
make install  # Install to /usr/local/bin
```

## Feedback

Open a GitHub issue to report bugs or suggest improvements.
