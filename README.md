# Destill

A distributed log triage tool for CI/CD pipelines that helps developers quickly identify and prioritize build failures using an agentic architecture.

## 🚀 Quick Start

The quickest start:

```bash
# 1. Build binaries
make build

# 2. Set your Buildkite API token
export BUILDKITE_API_TOKEN="your-token"

# 3. Analyze a build in local in-memory mode
./bin/destill build "https://buildkite.com/org/pipeline/builds/123"
```

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

1. **Ingests** build logs from Buildkite (chunks into 500KB pieces)
2. **Analyzes** logs to detect errors and failures (stateless processing)
3. **Persists** findings to Postgres (via Redpanda Connect)
4. **Displays** results in an interactive TUI (sorted by confidence)

### Key Features

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

See **[ARCHITECTURE.md](./ARCHITECTURE.md)** for detailed architecture documentation.

## 📦 Components

### Binaries

- **`bin/destill`** - Local mode CLI with `build` command and streaming TUI
- **`bin/destill-ingest`** - Standalone ingest agent (distributed mode)
- **`bin/destill-analyze`** - Standalone analyze agent (distributed mode)

### Infrastructure (Distributed Mode Only)

- **Redpanda** - Message broker (Kafka-compatible)
- **Postgres** - Persistent storage
- **Redpanda Connect** - Stream processor (Kafka → Postgres)
- **Redpanda Console** - Web UI for monitoring

## 📚 Documentation

### Getting Started
- **[QUICK_START_AGENTIC.md](./QUICK_START_AGENTIC.md)** - 5-minute setup guide
- **[TESTING_AGENTIC_MODE.md](./TESTING_AGENTIC_MODE.md)** - Comprehensive testing walkthrough

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
- Buildkite API token

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
- `bin/destill` - Main CLI (local or distributed mode)
- `bin/destill-ingest` - Ingest agent
- `bin/destill-analyze` - Analyze agent

### Install

```bash
# Install binaries to /usr/local/bin
make install
```

## 🎯 Usage

Destill supports two modes with automatic detection:

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

# Agents automatically process builds as they come in
# Findings are stored in Postgres

# Query findings from Postgres
docker exec -it destill-postgres psql -U destill -d destill \
  -c "SELECT severity, confidence_score, LEFT(raw_message, 80) FROM findings ORDER BY confidence_score DESC LIMIT 10;"

# Or view in Redpanda Console at http://localhost:8080
# Note: TUI integration for distributed mode is planned for a future release 
```

**Advantages**:
- ✅ Persistent storage (findings survive restarts)
- ✅ Horizontally scalable (add more agents)
- ✅ View historical analyses
- ✅ Production-ready

### Local Mode (Quick Testing)

**Best for**: Quick testing, development, demos

**Requirements**: Just the binary (no Docker)

```bash
# Set environment (no REDPANDA_BROKERS = local mode)
export BUILDKITE_API_TOKEN="your-token"

# Run with streaming TUI (all-in-one)
./bin/destill build "https://buildkite.com/org/pipeline/builds/123"
```

**Advantages**:
- ✅ No infrastructure needed
- ✅ Instant startup
- ✅ Streaming TUI (real-time)
- ✅ Simple for demos

**Limitations**:
- ❌ No persistence (data lost on exit)
- ❌ Single process (no scaling)
- ❌ Can't view historical builds

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
| `BUILDKITE_API_TOKEN` | Yes | Buildkite API access token |
| `REDPANDA_BROKERS` | Agentic | Comma-separated broker addresses (e.g., `localhost:19092`) |
| `POSTGRES_DSN` | Agentic | Postgres connection string |

### Mode Detection

- **No `REDPANDA_BROKERS`**: Legacy mode (in-memory)
- **With `REDPANDA_BROKERS`**: Agentic mode (distributed)

Mode is automatically detected based on environment variables.

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
