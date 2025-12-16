# Destill - CI/CD Build Failure Analyzer

Destill helps engineers quickly find the root cause of build failures by analyzing logs with pattern-based detection and smart confidence scoring.

## Features

- 🔍 **Multi-Platform Support**: Buildkite and GitHub Actions
- ⚡ **Fast Local Analysis**: No infrastructure required
- 🎯 **Smart Confidence Scoring**: Failed jobs get boosted confidence + pattern-based detection
- 🤖 **Claude Integration**: MCP server for AI-assisted debugging
- 📊 **Interactive TUI**: Real-time findings sorted by confidence
- 🔧 **Self-Hosted Option**: Optional distributed mode with Redpanda + Postgres

## 🚀 Quick Start (No Infrastructure Required for Local Mode)

Destill runs entirely on your machine in "Local Mode". No Docker, no database setup required for individual analysis.

### 1. Setup API Tokens

First, ensure you have your CI provider API tokens configured as environment variables. Add these to your shell profile (e.g., `~/.zshrc` or `~/.bashrc`):

```bash
# Required for Buildkite analysis
export BUILDKITE_API_TOKEN="your_buildkite_token_here"

# Required for GitHub Actions analysis (PAT with 'repo' scope)
export GITHUB_TOKEN="ghp_your_github_token_here" 
```

For detailed instructions on generating these tokens, refer to:
*   [docs/GITHUB_ACTIONS.md](./docs/GITHUB_ACTIONS.md) for GitHub Tokens.
*   The Buildkite documentation for Buildkite API tokens.

### 2. Installation

```bash
# Build from source
make build

# Or install binaries to /usr/local/bin (recommended for daily use)
make install
```

### 3. Analyze a Build

Next time a build fails, run `destill analyze` with the build URL:

**Buildkite:**
```bash
./bin/destill analyze "https://buildkite.com/org/pipeline/builds/123"
```

**GitHub Actions:**
```bash
./bin/destill analyze "https://github.com/owner/repo/actions/runs/456"
```

### 4. What You Get

*   **Ranked Findings**: The most likely errors are shown at the top (based on confidence score).
*   **Failed Job Boosting**: Errors from failed jobs get higher confidence scores.
*   **Smart Context**: See the error lines plus relevant context, stripped of noise.
*   **Interactive TUI**: Navigate findings in a real-time terminal user interface.

## 🤖 Claude Integration (Optional)

If you use Claude Desktop, you can let Claude analyze builds for you:

1.  Build the MCP server: `make build` (produces `bin/destill-mcp`)
2.  Add to your Claude config (see `docs/MCP_INTEGRATION.md` for details):
    ```json
    {
      "mcpServers": {
        "destill": {
          "command": "/absolute/path/to/bin/destill-mcp",
          "env": {
            "BUILDKITE_API_TOKEN": "...",
            "GITHUB_TOKEN": "..."
          }
        }
      }
    }
    ```
3.  Restart Claude Desktop and ask: "Analyze this build: <url>"

## 🗣️ Feedback

We welcome your feedback to improve Destill. Please open a GitHub issue to share your thoughts, bug reports, and suggestions.

*   Did Destill help you find the root cause faster?
*   What false positives or missed findings did you observe?
*   What features would make Destill more useful for your workflow?

## 📋 What is Destill?

Destill is a **distributed log analysis system** that automatically:

1. **Ingests** build logs from Buildkite and GitHub Actions
2. **Analyzes** logs to detect errors and failures (stateless processing)
3. **Boosts** confidence for errors from failed jobs
4. **Persists** findings to Postgres (via Redpanda Connect)
5. **Displays** results in an interactive TUI (sorted by confidence)

### Key Features

- ✅ **Failed Job Detection**: Errors from failed jobs get boosted confidence
- ✅ **Stateless Agents**: Horizontally scalable ingest and analyze agents
- ✅ **Smart Chunking**: 500KB chunks with 50-line overlap for context
- ✅ **Error Detection**: Pattern-based severity detection with confidence scoring
- ✅ **Deduplication**: SHA256 hashing of normalized messages
- ✅ **Distributed**: Redpanda (Kafka) for messaging, Postgres for storage
- ✅ **Real-time**: Stream processing with consumer groups
- ✅ **Interactive TUI**: Bubble Tea-based terminal interface

## 🏗️ Architecture

```
User Request → Ingest Agent → Redpanda → Analyze Agent → Redpanda → Postgres → TUI
              (fetches logs)   (chunks)   (finds errors)  (findings)  (stores)  (displays)
```

**Processing Flow**:
- Ingest → chunk → analyze → findings (confidence boosted for failed jobs)

See **[ARCHITECTURE.md](./ARCHITECTURE.md)** for detailed architecture documentation.

## 📦 Components

### Binaries

- **`bin/destill`** - Unified CLI with three commands:
  - `analyze` - Local mode (in-memory processing with streaming TUI)
  - `submit` - Distributed mode (publish request to Redpanda)
  - `view` - Distributed mode (query findings from Postgres)
- **`bin/destill-ingest`** - Standalone ingest agent (distributed mode)
- **`bin/destill-analyze`** - Standalone analyze agent (distributed mode)

### Infrastructure (Distributed Mode Only)

- **Redpanda** - Message broker (Kafka-compatible)
- **Postgres** - Persistent storage
- **Redpanda Connect** - Stream processor (Kafka → Postgres)
- **Redpanda Console** - Web UI for monitoring

## 📚 Documentation

### Getting Started
- **[QUICK_START_DISTRIBUTED.md](./QUICK_START_DISTRIBUTED.md)** - 5-minute setup guide
- **[TESTING_DISTRIBUTED_MODE.md](./TESTING_DISTRIBUTED_MODE.md)** - Comprehensive testing walkthrough
- **[docs/GITHUB_ACTIONS.md](./docs/GITHUB_ACTIONS.md)** - GitHub Actions setup
- **[docs/MCP_INTEGRATION.md](./docs/MCP_INTEGRATION.md)** - Claude integration guide

### Technical Details
- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - System design and data flow
- **[docker/README.md](./docker/README.md)** - Infrastructure documentation
- **[docker/MONITORING_CONNECT.md](./docker/MONITORING_CONNECT.md)** - Monitoring guide

## 🛠️ Building from Source

### Prerequisites

- Go 1.24.10 or later
- Docker Desktop (for infrastructure)
- Buildkite API token or GitHub token

### Build

```bash
# Build all binaries
make build

# Run tests
make test

# Run tests with coverage
make test-coverage
```

**Binaries produced**:
- `bin/destill` - Unified CLI (`analyze`, `submit`, and `view` commands)
- `bin/destill-ingest` - Ingest agent (distributed mode)
- `bin/destill-analyze` - Analyze agent (distributed mode)

### Install

```bash
# Install binaries to /usr/local/bin
make install
```

## 🎯 Usage

Destill supports two modes:

### Local Mode (Quick Testing)

**Best for**: Quick testing, development, demos

**Requirements**: Just the binary (no Docker)

**Buildkite:**
```bash
export BUILDKITE_API_TOKEN="your-token"
./bin/destill analyze "https://buildkite.com/org/pipeline/builds/123"
```

**GitHub Actions:**
```bash
export GITHUB_TOKEN="your-token"
./bin/destill analyze "https://github.com/owner/repo/actions/runs/456"
```

**Options:**
- `--json` - Output findings as JSON instead of TUI (not yet implemented)
- `--cache FILE` - Load cached triage cards for fast iteration

**How it works**:
- Launches in-memory broker
- Starts ingestion and analysis agents as goroutines
- Displays findings in real-time streaming TUI
- Press 'r' to refresh/re-rank cards as they arrive

**Advantages**:
- ✅ No infrastructure needed
- ✅ Instant startup
- ✅ Streaming TUI (real-time)
- ✅ Simple for demos

**Limitations**:
- ❌ No persistence (data lost on exit)
- ❌ Single process (no scaling)
- ❌ Can't view historical builds

### Distributed Mode (Recommended for Production)

**Best for**: Production, persistence, scalability

**Requirements**: Redpanda and Postgres running (via Docker)

```bash
# Set environment variables
export BUILDKITE_API_TOKEN="your-token"
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"

# Start agents (in separate terminals)
./bin/destill-ingest
./bin/destill-analyze

# Submit a build for analysis
./bin/destill submit "https://buildkite.com/org/pipeline/builds/123"
# Returns: ✅ Submitted analysis request: req-1733769623456789

# View findings in TUI (replace with your actual request ID)
./bin/destill view req-1733769623456789

# Or query findings from Postgres directly
docker exec -it destill-postgres psql -U destill -d destill \
  -c "SELECT severity, confidence_score, LEFT(raw_message, 80) FROM findings ORDER BY confidence_score DESC LIMIT 10;"

# Or view in Redpanda Console at http://localhost:8080
```

**How it works**:
- `submit` publishes request to Redpanda and returns immediately
- Agents process asynchronously (fetch logs, analyze, store findings)
- `view` queries Postgres and displays results in TUI

**Advantages**:
- ✅ Persistent storage (findings survive restarts)
- ✅ Horizontally scalable (add more agents)
- ✅ View historical analyses
- ✅ Production-ready

## 🔍 Monitoring

### Redpanda Console
- **URL**: http://localhost:8080
- **Features**: Topics, consumer groups, messages

### Redpanda Connect
- **Health**: `curl http://localhost:4195/ready`
- **Metrics**: `curl http://localhost:4195/stats`

### Postgres
```bash
docker exec -it destill-postgres psql -U destill -d destill
```

```sql
-- Count findings
SELECT COUNT(*) FROM findings;

-- Recent findings
SELECT severity, confidence_score, LEFT(raw_message, 80)
FROM findings
ORDER BY created_at DESC
LIMIT 10;
```

## 🧪 Testing

Run the comprehensive test suite:

```bash
# Unit tests (43 tests)
make test

# Manual end-to-end test
# See TESTING_DISTRIBUTED_MODE.md for full guide
```

Test coverage by package:
- Broker: 10 tests ✅
- Store: 5 tests ✅
- Pipeline: 2 tests ✅
- Ingest: 11 tests ✅
- Analyze: 15 tests ✅

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Submit a pull request

## 📝 Configuration

### Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `BUILDKITE_API_TOKEN` | For Buildkite | Buildkite API access token |
| `GITHUB_TOKEN` | For GitHub Actions | GitHub Personal Access Token with `repo` scope |
| `REDPANDA_BROKERS` | Distributed only | Comma-separated broker addresses (e.g., `localhost:19092`) |
| `POSTGRES_DSN` | Distributed only | Postgres connection string |

### Command Summary

- **`destill analyze <url>`** - Local mode (in-memory, no infrastructure)
- **`destill submit <url>`** - Distributed mode (requires agents + infrastructure)
- **`destill view <request-id>`** - Distributed mode (query Postgres)

## 🐛 Troubleshooting

### Agents not receiving messages
```bash
# Check consumer groups
docker exec -it destill-redpanda rpk group list

# Check topics
docker exec -it destill-redpanda rpk topic list
```

### No findings in Postgres
```bash
# Check Connect logs
docker logs destill-connect

# Check findings topic
docker exec -it destill-redpanda rpk topic consume destill.analysis.findings --num 5
```

### Infrastructure issues
```bash
# Check service health
docker-compose ps

# View logs
docker-compose logs -f
```

See **[TESTING_DISTRIBUTED_MODE.md](./TESTING_DISTRIBUTED_MODE.md)** for detailed troubleshooting.

## 📊 Performance

### Throughput
- **Ingest**: ~1000 lines/sec per agent
- **Analyze**: ~5000 lines/sec per agent
- **Postgres**: ~100 findings/sec (batched)

### Scaling
- **Horizontal**: Add more agent instances
- **Vertical**: Increase Redpanda/Postgres resources

### Resource Usage
- **Ingest Agent**: ~50MB RAM
- **Analyze Agent**: ~30MB RAM
- **Infrastructure**: ~2GB RAM (Docker)

## 📄 License

MIT License - see LICENSE file for details.

## 🙏 Acknowledgments

Built with:
- [Redpanda](https://redpanda.com/) - Streaming platform
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Franz-go](https://github.com/twmb/franz-go) - Kafka client
- [Cobra](https://github.com/spf13/cobra) - CLI framework

---

For questions or issues, please open a GitHub issue.
