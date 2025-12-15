# Destill - CI/CD Build Failure Analyzer

Destill helps engineers quickly find the root cause of build failures by analyzing logs with pattern-based detection and JUnit parsing.

## Features

- 🔍 **Multi-Platform Support**: Buildkite and GitHub Actions
- ⚡ **Fast Local Analysis**: No infrastructure required
- 🎯 **Smart Confidence Scoring**: JUnit parsing (1.0) + pattern-based detection (0.0-1.0)
- 🤖 **Claude Integration**: MCP server for AI-assisted debugging
- 📊 **Interactive TUI**: Real-time findings sorted by confidence
- 🔧 **Self-Hosted Option**: Optional distributed mode with Redpanda + Postgres

## 🚀 Quick Start

### Installation

```bash
# Build from source
make build

# Or install binaries to /usr/local/bin
make install
```

### Analyze a Build

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

### Claude Integration

See [MCP Integration Guide](docs/MCP_INTEGRATION.md) for setting up Destill with Claude Desktop or Claude Code.

For the full distributed system with agents, Redpanda, and Postgres, see **[QUICK_START_AGENTIC.md](./QUICK_START_AGENTIC.md)** for a 5-minute setup guide.

### TL;DR

```bash
# 1. Build binaries
make build

# 2. Start infrastructure (Docker required)
cd docker && docker-compose up -d

# 3. Create topics
docker exec -it destill-redpanda rpk topic create destill.logs.raw --partitions 3 --replicas 1
docker exec -it destill-redpanda rpk topic create destill.analysis.findings --partitions 3 --replicas 1
docker exec -it destill-redpanda rpk topic create destill.requests --partitions 1 --replicas 1

# 4. Set environment variables
export BUILDKITE_API_TOKEN="your-token"
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"

# 5. Start agents (in separate terminals)
./bin/destill-ingest    # Terminal 1
./bin/destill-analyze   # Terminal 2

# 6. Agents automatically process builds and store findings in Postgres
# Query results:
docker exec -it destill-postgres psql -U destill -d destill \
  -c "SELECT severity, LEFT(raw_message, 60) FROM findings ORDER BY confidence_score DESC LIMIT 5;"
# Or view in Redpanda Console at http://localhost:8080
```

## 📋 What is Destill?

Destill is an **agentic log analysis system** that automatically:

1. **Ingests** build logs and JUnit XML artifacts from Buildkite and GitHub Actions
2. **Analyzes** logs to detect errors and failures (stateless processing)
3. **Parses** JUnit XML for definitive test failures (1.0 confidence)
4. **Persists** findings to Postgres (via Redpanda Connect)
5. **Displays** results in an interactive TUI (sorted by confidence)

### Key Features

- ✅ **JUnit XML Support**: Automatic parsing of JUnit test results (1.0 confidence)
- ✅ **Stateless Agents**: Horizontally scalable ingest and analyze agents
- ✅ **Smart Chunking**: 500KB chunks with 50-line overlap for context
- ✅ **Error Detection**: Pattern-based severity detection with confidence scoring
- ✅ **Deduplication**: SHA256 hashing of normalized messages
- ✅ **Distributed**: Redpanda (Kafka) for messaging, Postgres for storage
- ✅ **Real-time**: Stream processing with consumer groups
- ✅ **Interactive TUI**: Bubble Tea-based terminal interface

## 🏗️ Architecture

```
User Request → Ingest Agent ─┬→ Redpanda → Analyze Agent → Redpanda ─┬→ Postgres → TUI
              (fetches logs)  │  (chunks)   (finds errors)  (findings) │  (stores)  (displays)
              (fetches junit) └───────────────────────────────────────┘
                                 (JUnit findings: 1.0 confidence)
```

**Dual-Source Processing**:
- **Log Analysis**: Ingest → chunk → analyze → findings (0.0-1.0 confidence)
- **JUnit Parsing**: Ingest → parse → findings (1.0 confidence, bypasses analyze)

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

## 🧪 JUnit XML Support

Destill automatically detects and parses JUnit XML test results from Buildkite artifacts, providing **definitive test failure findings with 1.0 confidence**.

### How It Works

1. **Automatic Detection**: When processing a build, the ingest agent checks each job for artifacts matching `junit*.xml`
2. **XML Parsing**: JUnit XML is parsed to extract `<failure>` and `<error>` elements
3. **High Confidence**: Test failures receive 1.0 confidence (definitive, not heuristic)
4. **Bypass Analysis**: JUnit findings skip the analyze agent (already structured data)
5. **TUI Integration**: Test failures appear at the top of the TUI (sorted by confidence)

### Setup Requirements

**In your Buildkite pipeline**, upload JUnit XML artifacts:

```yaml
steps:
  - label: "Run Tests"
    command: "make test"
    artifact_paths: "test-results/junit*.xml"  # Upload JUnit XML
```

**That's it!** Destill will automatically:
- ✅ Detect JUnit XML artifacts
- ✅ Parse test failures
- ✅ Create high-confidence findings
- ✅ Display in TUI alongside log-based findings

### Example Finding

A JUnit test failure appears in the TUI like this:

```
Rank #1 | ⚠️  ERROR | Confidence: 1.00 | Recurrence: 1x
Source: junit:test-results/junit.xml
Job: test-suite

[failure] com.example.MyTest.testFoo: expected true but was false

Stack Trace:
  at com.example.MyTest.testFoo(MyTest.java:42)
  at org.junit.runners.ParentRunner.run(ParentRunner.java:238)
  ...

Metadata:
  test_name: testFoo
  class_name: com.example.MyTest
  duration_sec: 0.123
```

### Benefits

- **🎯 Definitive**: Test failures are ground truth (not pattern matching)
- **⚡ Fast**: No LLM or heuristic analysis needed
- **🔍 Traceable**: Full stack traces and test metadata preserved
- **📊 Integrated**: Appears alongside log-based findings in unified view

### Supported Formats

Destill supports standard JUnit XML formats:
- ✅ Single `<testsuite>` (most common)
- ✅ Multiple `<testsuites>` (nested format)
- ✅ Both `<failure>` and `<error>` elements
- ✅ Captures: test name, class, message, stack trace, duration

## 📚 Documentation

### Getting Started
- **[QUICK_START_AGENTIC.md](./QUICK_START_AGENTIC.md)** - 5-minute setup guide
- **[TESTING_AGENTIC_MODE.md](./TESTING_AGENTIC_MODE.md)** - Comprehensive testing walkthrough
- **[docs/GITHUB_ACTIONS.md](./docs/GITHUB_ACTIONS.md)** - GitHub Actions setup
- **[docs/MCP_INTEGRATION.md](./docs/MCP_INTEGRATION.md)** - Claude integration guide

### Technical Details
- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - System design and data flow
- **[docker/README.md](./docker/README.md)** - Infrastructure documentation
- **[docker/MONITORING_CONNECT.md](./docker/MONITORING_CONNECT.md)** - Monitoring guide

### Historical
- **[project_notes/](./project_notes/)** - Development logs and planning docs

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
# See TESTING_AGENTIC_MODE.md for full guide
```

Test coverage by package:
- Broker: 10 tests ✅
- Store: 5 tests ✅
- Pipeline: 2 tests ✅
- Ingest: 11 tests ✅
- Analyze: 15 tests ✅
- JUnit: 8 tests ✅

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

See **[TESTING_AGENTIC_MODE.md](./TESTING_AGENTIC_MODE.md)** for detailed troubleshooting.

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
