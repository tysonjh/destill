# Destill Infrastructure - Phase 1

This directory contains the Docker Compose configuration for the Destill Agentic Data Plane architecture.

## Services

### Redpanda
- **Purpose**: Message broker for streaming log chunks and findings
- **Ports**:
  - `19092`: Kafka API (external)
  - `18081`: Schema Registry
  - `18082`: HTTP Proxy
  - `19644`: Admin API
- **Topics**:
  - `destill.logs.raw`: Raw log chunks (~500KB each)
  - `destill.analysis.findings`: Analysis findings (triage cards)
  - `destill.requests`: Build analysis requests

### Postgres
- **Purpose**: Persistent storage for analysis findings
- **Port**: `5432`
- **Database**: `destill`
- **Credentials**: `destill/destill` (development only)
- **Schema**: See `init-db.sql`

### Redpanda Connect
- **Purpose**: Sink findings from Redpanda to Postgres
- **Port**: `4195` (HTTP API for monitoring)
- **Config**: `connect.yaml`
- **Consumer Group**: `destill-postgres-sink`
- **Monitoring**: Via HTTP endpoints - see `MONITORING_CONNECT.md`

### Redpanda Console
- **Purpose**: Web UI for monitoring Redpanda topics
- **Port**: `8080`
- **URL**: http://localhost:8080
- **Features**:
  - Topic browser and message viewer
  - Consumer group monitoring
  - Schema registry
  - **Note**: Console's "Connect" tab is for Kafka Connect only, not Redpanda Connect

## Quick Start

```bash
# Start all services
cd docker
docker-compose up -d

# Check service health
docker-compose ps

# View logs
docker-compose logs -f

# Stop all services
docker-compose down

# Stop and remove volumes (clean slate)
docker-compose down -v
```

## Creating Topics

Topics can be created manually using `rpk`:

```bash
# Create raw logs topic (ephemeral, short retention)
docker exec -it destill-redpanda rpk topic create destill.logs.raw \
  --partitions 3 \
  --replicas 1 \
  --config retention.ms=3600000

# Create findings topic (longer retention)
docker exec -it destill-redpanda rpk topic create destill.analysis.findings \
  --partitions 3 \
  --replicas 1 \
  --config retention.ms=604800000

# Create requests topic
docker exec -it destill-redpanda rpk topic create destill.requests \
  --partitions 1 \
  --replicas 1

# List topics
docker exec -it destill-redpanda rpk topic list
```

## Verifying Setup

### Check Redpanda
```bash
# Check cluster health
docker exec -it destill-redpanda rpk cluster health

# Produce a test message
echo '{"test": "message"}' | docker exec -i destill-redpanda \
  rpk topic produce destill.logs.raw

# Consume test messages
docker exec -it destill-redpanda \
  rpk topic consume destill.logs.raw --num 1
```

### Check Postgres
```bash
# Connect to Postgres
docker exec -it destill-postgres psql -U destill -d destill

# List tables
\dt

# Query findings
SELECT COUNT(*) FROM findings;

# Exit
\q
```

### Check Redpanda Console
Open http://localhost:8080 in your browser to view:
- Topics and their messages
- Consumer groups
- Schema registry

### Monitor Redpanda Connect
Redpanda Connect uses its own API (not compatible with Console's Connect tab):

```bash
# Health check
curl http://localhost:4195/ready

# Metrics (Prometheus format)
curl http://localhost:4195/stats

# Quick stats
curl -s http://localhost:4195/stats | grep -E "^(input_received|output_sent)" | grep -v "#"
```

See `MONITORING_CONNECT.md` for detailed monitoring guide.

## Environment Variables

For connecting to this infrastructure from the Destill CLI:

```bash
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"
export BUILDKITE_API_TOKEN="your-token-here"
```

## Troubleshooting

### Services won't start
```bash
# Check for port conflicts
lsof -i :5432  # Postgres
lsof -i :19092 # Redpanda
lsof -i :8080  # Console

# Check logs
docker-compose logs redpanda
docker-compose logs postgres
```

### Redpanda Connect not working
```bash
# Check connect logs
docker-compose logs redpanda-connect

# Verify it can reach Postgres
docker exec -it destill-connect ping postgres

# Verify it can reach Redpanda
docker exec -it destill-connect ping redpanda
```

### Clean slate
```bash
# Remove all data and start fresh
docker-compose down -v
docker-compose up -d
```


