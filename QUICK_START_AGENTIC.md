# Destill Agentic Mode - Quick Start

## 🚀 5-Minute Setup

### 1. Build Binaries
```bash
make build
```

### 2. Start Infrastructure
```bash
cd docker && docker-compose up -d
```

### 3. Create Topics
```bash
docker exec -it destill-redpanda rpk topic create destill.logs.raw --partitions 3 --replicas 1 --config retention.ms=3600000
docker exec -it destill-redpanda rpk topic create destill.analysis.findings --partitions 3 --replicas 1 --config retention.ms=604800000
docker exec -it destill-redpanda rpk topic create destill.requests --partitions 1 --replicas 1
```

### 4. Set Environment Variables
```bash
export BUILDKITE_API_TOKEN="your-token"
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"
```

### 5. Start Agents (in separate terminals)
```bash
# Terminal 1
./bin/destill-ingest

# Terminal 2
./bin/destill-analyze
```

### 6. Analyze a Build
```bash
# Terminal 3
./bin/destill build "https://buildkite.com/org/pipeline/builds/123"
# Note the request ID returned (e.g., req-1733769623456789)

# View results in TUI
./bin/destill view req-1733769623456789
```

## 🔍 Monitoring

- **Redpanda Console**: http://localhost:8080
- **Postgres**: `docker exec -it destill-postgres psql -U destill -d destill`
- **Agent Logs**: Watch terminals 1 & 2

## 🧪 Verify It's Working

```bash
# Check topics
docker exec -it destill-redpanda rpk topic list

# Check consumer groups
docker exec -it destill-redpanda rpk group list

# Query findings
docker exec -it destill-postgres psql -U destill -d destill -c \
  "SELECT COUNT(*) FROM findings;"
```

## 🛑 Cleanup

```bash
# Stop agents: Ctrl+C in terminals
cd docker && docker-compose down -v
```

## 📚 Full Documentation

See `TESTING_AGENTIC_MODE.md` for detailed walkthrough.

