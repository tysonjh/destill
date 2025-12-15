# Destill Architecture - Distributed Data Plane

Destill is a distributed log triage system for CI/CD pipelines using a distributed architecture with Redpanda (Kafka) and Postgres.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                   Distributed Data Plane                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  CLI: destill submit <build-url>                                │
│       │                                                         │
│       ▼                                                         │
│  ┌──────────────┐      ┌─────────────────┐                     │
│  │   Redpanda   │      │  Ingest Agent   │                     │
│  │  .requests   │─────▶│   (stateless)   │                     │
│  └──────────────┘      └─────────┬───────┘                     │
│                                  │                             │
│                        Buildkite API                           │
│                                  │                             │
│                                  ▼                             │
│                        ┌─────────────────┐                     │
│                        │   Redpanda      │                     │
│                        │  .logs.raw      │                     │
│                        │  (~500KB chunks)│                     │
│                        └────────┬────────┘                     │
│                                 │                              │
│                                 ▼                              │
│                        ┌──────────────────┐                    │
│                        │ Analyze Agent    │                    │
│                        │  (stateless)     │                    │
│                        └────────┬─────────┘                    │
│                                 │                              │
│                                 ▼                              │
│                        ┌─────────────────┐                     │
│                        │   Redpanda      │                     │
│                        │  .findings      │                     │
│                        └────────┬────────┘                     │
│                                 │                              │
│                    Redpanda Connect                            │
│                        (Benthos)                               │
│                                 │                              │
│                                 ▼                              │
│                        ┌─────────────────┐                     │
│                        │   Postgres      │                     │
│                        │   findings      │                     │
│                        └────────┬────────┘                     │
│                                 │                              │
│                                 ▼                              │
│                      CLI: destill view <req-id>                │
│                           (TUI Display)                        │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Ingest Agent (`destill-ingest`)

**Purpose**: Fetch build logs and JUnit artifacts from CI systems

**Operation**:
- Consumes: `destill.requests` topic
- Fetches: Build metadata, job logs, and JUnit XML artifacts from Buildkite API
- **Log Processing**:
  - Chunks: Logs into ~500KB chunks with 50-line overlap
  - Publishes: `LogChunkV2` to `destill.logs.raw` topic
  - Keying: Uses `buildID` as message key for ordering
- **JUnit Processing**:
  - Searches: For `junit*.xml` artifacts in each job
  - Parses: JUnit XML to extract test failures
  - Publishes: `TriageCardV2` directly to `destill.analysis.findings` topic
  - Confidence: 1.0 (test failures are definitive)
  - Bypasses: Analyze agent (no heuristic analysis needed)

**Stateless**: No cross-request state. Can scale horizontally.

### 2. Analyze Agent (`destill-analyze`)

**Purpose**: Analyze log chunks and extract error findings

**Operation**:
- Consumes: `destill.logs.raw` topic
- Analyzes: Each chunk independently (stateless)
- Detects: ERROR and FATAL severity lines
- Scores: Confidence based on patterns and context
- Extracts: 15 lines pre-context, 30 lines post-context (within chunk)
- Normalizes: Messages for deduplication (removes timestamps, UUIDs, etc.)
- Hashes: Normalized messages with SHA256
- Publishes: `TriageCardV2` to `destill.analysis.findings` topic
- Keying: Uses `requestID` as message key for grouping

**Stateless**: Processes each chunk in isolation. Accepts boundary limitations. Can scale horizontally.

### 2b. JUnit XML Processing (in Ingest Agent)

**Purpose**: Extract definitive test failures from structured JUnit XML data

**Why in Ingest Agent?**
- JUnit XML is already structured data (not heuristic)
- Test failures have 1.0 confidence (definitive)
- No need for pattern matching or analysis
- Bypasses analyze agent for efficiency

**Operation**:
- Detection: Identifies `junit*.xml` artifacts from Buildkite
- Parsing: Uses `encoding/xml` to parse test suites and cases
- Extraction: Only processes `<failure>` and `<error>` elements
- Hashing: Uses `class::test::message` for deduplication
- Metadata: Includes test name, class, suite, duration, stack trace
- Publishing: Sends to `destill.analysis.findings` (same as log findings)

**Confidence Scoring**: Always 1.0
- Test failures are ground truth (not heuristic)
- No false positives (unlike log pattern matching)
- Sorted to top of TUI automatically

**Example JUnit Finding**:
```json
{
  "id": "req-123-job-456-a1b2c3d4",
  "message_hash": "a1b2c3d4e5f6...",
  "source": "junit:test-results/junit.xml",
  "severity": "error",
  "raw_message": "[failure] com.example.MyTest.testFoo: expected true but was false",
  "normalized_message": "com.example.MyTest::testFoo",
  "confidence_score": 1.0,
  "post_context": ["at com.example.MyTest.testFoo(MyTest.java:42)", "..."],
  "metadata": {
    "source_type": "junit",
    "test_name": "testFoo",
    "class_name": "com.example.MyTest",
    "failure_type": "failure",
    "duration_sec": "0.123"
  }
}
```

### 3. Redpanda Connect (Benthos)

**Purpose**: Stream processor for Kafka → Postgres sink

**Operation**:
- Consumes: `destill.analysis.findings` topic
- Transforms: JSONB fields (arrays to JSON)
- Batches: 100 messages or 1 second
- Writes: To Postgres `findings` table
- Consumer Group: `destill-postgres-sink`

**Configuration**: See `docker/connect.yaml`

### 4. Postgres Storage

**Purpose**: Persistent storage for analysis findings

**Schema**:
- `findings`: Analysis results with JSONB context
- `requests`: Request tracking and status
- `findings_summary`: Aggregated view for recurrence

**Indexes**: request_id, message_hash, confidence_score, created_at

### 5. CLI Tool (`destill`)

**Unified CLI** with three subcommands:

**`destill analyze <url>`** - Local Mode
- **Purpose**: Local, in-memory build analysis with streaming TUI
- **Mode**: Uses InMemoryBroker with agents as goroutines
- **Requirements**: Only `BUILDKITE_API_TOKEN`
- **Features**: Real-time streaming, no persistence, no infrastructure
- **Usage**: `./bin/destill analyze "https://buildkite.com/org/pipeline/builds/123"`
- **Options**:
  - `--json` - Output JSON (not yet implemented)
  - `--cache FILE` - Load cached cards for iteration

**`destill submit <url>`** - Distributed Mode
- **Purpose**: Submit build for asynchronous analysis
- **Mode**: Publishes request to Redpanda, returns immediately
- **Requirements**: `BUILDKITE_API_TOKEN`, `REDPANDA_BROKERS`, `POSTGRES_DSN`
- **Returns**: Request ID for tracking
- **Usage**: `./bin/destill submit "https://buildkite.com/org/pipeline/builds/123"`

**`destill view <request-id>`** - Distributed Mode
- **Purpose**: View findings from Postgres in TUI
- **Requirements**: `POSTGRES_DSN` environment variable
- **Features**: Converts TriageCardV2 → TriageCard, launches TUI
- **Usage**: `./bin/destill view req-1733769623456789`

## Key Design Decisions

### Stateless Processing

Both agents are **completely stateless**:
- No shared memory between requests
- No cross-chunk state in analyzer
- Each message processed independently
- Enables horizontal scaling

**Trade-off**: Context extraction limited to chunk boundaries. Acceptable for PoC - provides "best effort" context.

### 500KB Chunking with Overlap

**Why**:
- Redpanda best practices (avoid huge messages)
- Parallel processing by multiple analyze agents
- Memory-efficient

**How**:
- Target: ~500KB per chunk
- Overlap: 50 lines between chunks
- Line tracking: Each chunk knows its line range

### Message Keying Strategy

**Logs Topic** (`destill.logs.raw`):
- Key: `buildID`
- Ensures chunks from same build stay ordered
- Enables sequential processing if needed

**Findings Topic** (`destill.analysis.findings`):
- Key: `requestID`
- Groups findings by analysis request
- Simplifies per-request queries

### Consumer Groups

- `destill-ingest`: Ingest agents (parallel processing)
- `destill-analyze`: Analyze agents (parallel processing)
- `destill-postgres-sink`: Connect sink (parallel writes)

Multiple instances in same group = automatic load balancing

## Data Flow

### Submit Request (Distributed Mode)
```
User → destill submit <url>
     → AnalysisRequest → destill.requests (Redpanda)
     ← Request ID returned immediately
```

### Analyze Request (Local Mode)
```
User → destill analyze <url>
     → InMemoryBroker
     → Agents as goroutines
     → Streaming TUI
     ← Interactive results
```

### Ingest
```
Ingest Agent → Subscribe: destill.requests
             → Buildkite API (fetch logs)
             → Chunk (500KB)
             → Publish: destill.logs.raw
```

### Analyze
```
Analyze Agent → Subscribe: destill.logs.raw
              → Detect errors (stateless)
              → Extract context (chunk-only)
              → Normalize + hash
              → Publish: destill.analysis.findings
```

### Persist
```
Redpanda Connect → Subscribe: destill.analysis.findings
                 → Transform (JSONB)
                 → Batch (100 or 1s)
                 → Insert: Postgres findings
```

### View
```
User → destill view <req-id>
     → Load config (POSTGRES_DSN)
     → Connect: Postgres
     → Query: GetFindings(request_id)
     → Convert: TriageCardV2 → TriageCard
     → Display: TUI with findings (sorted by confidence)
```

## Scalability

### Horizontal Scaling

**Ingest Agents**: Add more instances
- Each processes different builds
- Share work via consumer group

**Analyze Agents**: Add more instances
- Each processes different chunks
- Share work via consumer group
- Completely independent

### Vertical Limits

- **Redpanda**: Single-broker for dev (multi-broker for prod)
- **Postgres**: Connection pooling recommended
- **Message Size**: 500KB chunks (well within Kafka limits)

## Monitoring

### Redpanda Console
- Topics: Message flow visualization
- Consumer Groups: Lag monitoring
- URL: http://localhost:8080

### Redpanda Connect
- HTTP API: http://localhost:4195/stats
- Metrics: Prometheus format
- Health: `/ready` endpoint
- See: `docker/MONITORING_CONNECT.md`

### Postgres
- Query: `SELECT COUNT(*) FROM findings WHERE request_id = ?`
- Status: `SELECT * FROM requests WHERE request_id = ?`
- Summary: `SELECT * FROM findings_summary`

## Development

### Build
```bash
make build
```

Produces:
- `bin/destill` - Unified CLI (`analyze`, `submit`, and `view` commands)
- `bin/destill-ingest` - Ingest agent
- `bin/destill-analyze` - Analyze agent

### Test
```bash
make test
```

Runs all unit tests (43 tests across packages).

### Infrastructure
```bash
cd docker && docker-compose up -d
```

Services: Redpanda, Postgres, Connect, Console

## Production Considerations

### Topics
- Retention: 1h for logs (ephemeral), 7d for findings
- Partitions: 3 for parallelism
- Replication: 3 for durability (multi-broker)

### Agents
- Deployment: Kubernetes/Docker
- Replicas: 2-5 per agent type
- Resources: Minimal (stateless, efficient)
- Health checks: `/ready` endpoints

### Storage
- Postgres: Regular backups
- Indexes: Monitor query performance
- Cleanup: TTL on old findings (optional)

### Monitoring
- Metrics: Prometheus + Grafana
- Alerts: Consumer lag, error rates
- Logs: Centralized logging (ELK, etc.)

## References

- **Quick Start**: `QUICK_START_DISTRIBUTED.md`
- **Testing Guide**: `TESTING_DISTRIBUTED_MODE.md`
- **Connect Monitoring**: `docker/MONITORING_CONNECT.md`
- **Infrastructure**: `docker/README.md`

