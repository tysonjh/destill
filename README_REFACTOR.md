# Destill Refactor: Agentic Data Plane Architecture

## Overview

This document outlines the phased migration from a monolithic Go binary with in-memory state to a distributed "Agentic Data Plane" architecture using Redpanda, Postgres, and Docker.

### Current State
- Single Go binary (`destill`)
- In-memory message broker
- All processing happens in one process
- State lost when process exits

### Target Architecture
```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Agentic Data Plane                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌──────────────┐     ┌─────────────────┐     ┌──────────────────────────┐ │
│  │   destill    │     │    Redpanda     │     │   Redpanda Connect      │ │
│  │   ingest     │────▶│                 │────▶│      (Benthos)          │ │
│  │              │     │ destill.logs.raw│     │                          │ │
│  └──────────────┘     └─────────────────┘     └───────────┬──────────────┘ │
│                                │                          │                 │
│                                │                          │                 │
│                                ▼                          ▼                 │
│  ┌──────────────┐     ┌─────────────────┐     ┌──────────────────────────┐ │
│  │   destill    │     │    Redpanda     │     │       Postgres          │ │
│  │   analyze    │◀────│                 │     │                          │ │
│  │  (stateless) │     │ destill.logs.raw│     │  ┌──────────────────┐   │ │
│  └──────┬───────┘     └─────────────────┘     │  │    findings      │   │ │
│         │                                      │  │    (JSONB)       │   │ │
│         │             ┌─────────────────┐     │  └──────────────────┘   │ │
│         └────────────▶│    Redpanda     │────▶│                          │ │
│                       │                 │     └──────────────────────────┘ │
│                       │ destill.        │                 ▲                 │
│                       │ analysis.       │                 │                 │
│                       │ findings        │     ┌───────────┴──────────────┐ │
│                       └─────────────────┘     │      destill view        │ │
│                                               │      <request-id>        │ │
│                                               └──────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Phase 1: Infrastructure Setup

### Objectives
- Create Docker Compose configuration for local development
- Define new Go module structure for subcommands
- Establish topic naming conventions

### Deliverables

#### 1.1 Docker Compose (`docker/docker-compose.yml`)

```yaml
version: '3.8'

services:
  redpanda:
    image: docker.redpanda.com/redpandadata/redpanda:v24.1.1
    container_name: destill-redpanda
    command:
      - redpanda
      - start
      - --smp 1
      - --memory 1G
      - --overprovisioned
      - --kafka-addr internal://0.0.0.0:9092,external://0.0.0.0:19092
      - --advertise-kafka-addr internal://redpanda:9092,external://localhost:19092
      - --pandaproxy-addr internal://0.0.0.0:8082,external://0.0.0.0:18082
      - --advertise-pandaproxy-addr internal://redpanda:8082,external://localhost:18082
      - --schema-registry-addr internal://0.0.0.0:8081,external://0.0.0.0:18081
    ports:
      - "18081:18081"  # Schema Registry
      - "18082:18082"  # HTTP Proxy
      - "19092:19092"  # Kafka API
      - "19644:9644"   # Admin API
    volumes:
      - redpanda-data:/var/lib/redpanda/data
    healthcheck:
      test: ["CMD", "rpk", "cluster", "health"]
      interval: 10s
      timeout: 5s
      retries: 5

  postgres:
    image: postgres:16-alpine
    container_name: destill-postgres
    environment:
      POSTGRES_USER: destill
      POSTGRES_PASSWORD: destill
      POSTGRES_DB: destill
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init.sql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U destill"]
      interval: 5s
      timeout: 5s
      retries: 5

  redpanda-connect:
    image: docker.redpanda.com/redpandadata/connect:4
    container_name: destill-connect
    volumes:
      - ./connect.yaml:/connect.yaml
    command: ["-c", "/connect.yaml"]
    depends_on:
      redpanda:
        condition: service_healthy
      postgres:
        condition: service_healthy
    environment:
      REDPANDA_BROKERS: redpanda:9092
      POSTGRES_DSN: postgres://destill:destill@postgres:5432/destill?sslmode=disable

  redpanda-console:
    image: docker.redpanda.com/redpandadata/console:v2.4.5
    container_name: destill-console
    ports:
      - "8080:8080"
    environment:
      KAFKA_BROKERS: redpanda:9092
      KAFKA_SCHEMAREGISTRY_ENABLED: "true"
      KAFKA_SCHEMAREGISTRY_URLS: http://redpanda:8081
    depends_on:
      - redpanda

volumes:
  redpanda-data:
  postgres-data:
```

#### 1.2 Go Module Structure

```
destill-agent/
├── cmd/
│   ├── destill/           # Main entry point (orchestrates subcommands)
│   │   └── main.go
│   ├── ingest/            # Standalone ingest agent
│   │   └── main.go
│   ├── analyze/           # Standalone analyze agent
│   │   └── main.go
│   └── view/              # TUI for viewing findings
│       └── main.go
├── internal/
│   ├── broker/            # Broker abstraction (InMemory, Redpanda)
│   │   ├── broker.go      # Interface
│   │   ├── inmemory.go    # Legacy in-memory implementation
│   │   └── redpanda.go    # Redpanda/Kafka implementation
│   ├── pipeline/          # Pipeline orchestration
│   │   ├── pipeline.go    # Interface
│   │   ├── legacy.go      # In-memory pipeline (current behavior)
│   │   └── agentic.go     # Distributed agentic pipeline
│   ├── ingest/            # Ingestion logic
│   │   ├── agent.go
│   │   └── chunker.go     # 500KB chunking logic
│   ├── analyze/           # Analysis logic
│   │   └── agent.go
│   ├── store/             # Data persistence
│   │   ├── store.go       # Interface
│   │   ├── memory.go      # In-memory store
│   │   └── postgres.go    # Postgres implementation
│   └── tui/               # Terminal UI (existing)
├── docker/
│   ├── docker-compose.yml
│   ├── init-db.sql
│   └── connect.yaml
└── contracts/
    └── messages.go        # Shared message types
```

#### 1.3 Topic Naming Convention

| Topic | Key | Value | Purpose |
|-------|-----|-------|---------|
| `destill.logs.raw` | `{build_id}` | `LogChunk` (JSON) | Raw log chunks (~500KB) |
| `destill.analysis.findings` | `{request_id}` | `TriageCard` (JSON) | Analysis findings |
| `destill.requests` | `{request_id}` | `AnalysisRequest` (JSON) | Build analysis requests |

### Acceptance Criteria
- [ ] `docker-compose up` starts all services
- [ ] Redpanda Console accessible at http://localhost:8080
- [ ] Postgres accessible at localhost:5432
- [ ] Topics can be created via `rpk topic create`

---

## Phase 2: Data Modeling

### Objectives
- Define Postgres schema for findings persistence
- Create Go interfaces for pipeline abstraction
- Enable switching between Legacy and Agentic modes

### Deliverables

#### 2.1 Postgres Schema (`docker/init-db.sql`)

```sql
-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Findings table: stores triage cards from analysis
CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) NOT NULL,
    build_url TEXT NOT NULL,
    job_name VARCHAR(255) NOT NULL,
    
    -- The actual finding
    message_hash VARCHAR(64) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    confidence_score DECIMAL(3,2) NOT NULL,
    
    -- Content (stored as JSONB for flexibility)
    raw_message TEXT NOT NULL,
    normalized_message TEXT NOT NULL,
    pre_context JSONB NOT NULL DEFAULT '[]',  -- Array of context lines
    post_context JSONB NOT NULL DEFAULT '[]', -- Array of context lines
    
    -- Metadata
    source VARCHAR(50) NOT NULL DEFAULT 'buildkite',
    line_number INTEGER,
    chunk_index INTEGER,
    metadata JSONB NOT NULL DEFAULT '{}',
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    analyzed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Indexes for common queries
    CONSTRAINT findings_confidence_check CHECK (confidence_score >= 0 AND confidence_score <= 1)
);

-- Indexes
CREATE INDEX idx_findings_request_id ON findings(request_id);
CREATE INDEX idx_findings_message_hash ON findings(message_hash);
CREATE INDEX idx_findings_job_name ON findings(job_name);
CREATE INDEX idx_findings_confidence ON findings(confidence_score DESC);
CREATE INDEX idx_findings_created_at ON findings(created_at DESC);

-- Requests table: tracks analysis requests
CREATE TABLE requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) UNIQUE NOT NULL,
    build_url TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Counts
    chunks_total INTEGER DEFAULT 0,
    chunks_processed INTEGER DEFAULT 0,
    findings_count INTEGER DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT requests_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX idx_requests_status ON requests(status);
CREATE INDEX idx_requests_created_at ON requests(created_at DESC);

-- View for aggregated findings by hash (recurrence tracking)
CREATE VIEW findings_summary AS
SELECT 
    message_hash,
    normalized_message,
    MAX(severity) as severity,
    AVG(confidence_score) as avg_confidence,
    COUNT(*) as recurrence_count,
    array_agg(DISTINCT job_name) as job_names,
    MIN(created_at) as first_seen,
    MAX(created_at) as last_seen
FROM findings
GROUP BY message_hash, normalized_message;
```

#### 2.2 Pipeline Interface (`internal/pipeline/pipeline.go`)

```go
package pipeline

import (
    "context"
    "destill-agent/contracts"
)

// Pipeline defines the interface for processing build analysis requests
type Pipeline interface {
    // Submit submits a build URL for analysis, returns request ID
    Submit(ctx context.Context, buildURL string) (requestID string, err error)
    
    // Status returns the current status of a request
    Status(ctx context.Context, requestID string) (*RequestStatus, error)
    
    // Stream returns a channel of findings for a request (for TUI streaming)
    Stream(ctx context.Context, requestID string) (<-chan contracts.TriageCard, error)
    
    // Close shuts down the pipeline
    Close() error
}

// RequestStatus represents the status of an analysis request
type RequestStatus struct {
    RequestID       string
    BuildURL        string
    Status          string // pending, processing, completed, failed
    ChunksTotal     int
    ChunksProcessed int
    FindingsCount   int
}

// Config holds pipeline configuration
type Config struct {
    // Broker configuration
    RedpandaBrokers string // Empty = use in-memory
    
    // Postgres configuration (for agentic mode)
    PostgresDSN string
    
    // Buildkite configuration
    BuildkiteToken string
}
```

#### 2.3 Broker Interface (`internal/broker/broker.go`)

```go
package broker

import "context"

// Broker abstracts message publishing and consumption
type Broker interface {
    // Publish sends a message to a topic
    Publish(ctx context.Context, topic string, key string, value []byte) error
    
    // Subscribe returns a channel for consuming messages from a topic
    Subscribe(ctx context.Context, topic string, groupID string) (<-chan Message, error)
    
    // Close shuts down the broker connection
    Close() error
}

// Message represents a consumed message
type Message struct {
    Topic     string
    Key       string
    Value     []byte
    Offset    int64
    Partition int32
    Timestamp int64
}
```

### Acceptance Criteria
- [ ] Postgres schema created successfully
- [ ] Pipeline interface defined with both implementations stubbed
- [ ] Broker interface defined with InMemory implementation working
- [ ] Unit tests pass for interfaces

---

## Phase 3: The Ingest Agent

### Objectives
- Implement log fetching from Buildkite
- Implement 500KB chunking strategy
- Publish chunks to Redpanda

### Deliverables

#### 3.1 Chunker (`internal/ingest/chunker.go`)

```go
package ingest

const (
    // TargetChunkSize is the target size for each chunk (500KB)
    TargetChunkSize = 500 * 1024
    
    // ContextOverlap is the number of lines to overlap between chunks
    // This helps preserve context at chunk boundaries
    ContextOverlap = 50
)

// LogChunk represents a chunk of log data
type LogChunk struct {
    RequestID   string            `json:"request_id"`
    BuildID     string            `json:"build_id"`
    JobName     string            `json:"job_name"`
    JobID       string            `json:"job_id"`
    ChunkIndex  int               `json:"chunk_index"`
    TotalChunks int               `json:"total_chunks"`
    Content     string            `json:"content"`
    LineStart   int               `json:"line_start"`  // First line number in this chunk
    LineEnd     int               `json:"line_end"`    // Last line number in this chunk
    Metadata    map[string]string `json:"metadata"`
}

// ChunkLog splits a log into ~500KB chunks with line overlap
func ChunkLog(content string, requestID, buildID, jobName, jobID string, metadata map[string]string) []LogChunk {
    // Implementation: split by lines, accumulate until ~500KB, overlap
}
```

#### 3.2 Ingest Agent Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        Ingest Agent                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  1. Receive build URL                                           │
│         │                                                       │
│         ▼                                                       │
│  2. Fetch build metadata from Buildkite API                     │
│         │                                                       │
│         ▼                                                       │
│  3. For each job in build:                                      │
│     ┌───────────────────────────────────────────────────────┐  │
│     │  a. Fetch job log                                      │  │
│     │         │                                              │  │
│     │         ▼                                              │  │
│     │  b. Chunk log into ~500KB pieces                       │  │
│     │         │                                              │  │
│     │         ▼                                              │  │
│     │  c. Publish each chunk to destill.logs.raw             │  │
│     │     Key: {build_id}                                    │  │
│     │     Value: LogChunk JSON                               │  │
│     └───────────────────────────────────────────────────────┘  │
│                                                                 │
│  4. Publish completion marker                                   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Acceptance Criteria
- [ ] Chunking produces ~500KB chunks with 50-line overlap
- [ ] Chunks are keyed by build ID for ordering
- [ ] Chunk metadata includes line numbers for reconstruction
- [ ] Integration test: fetch real build, verify chunks published

---

## Phase 4: The Analyze Agent (Stateless)

### Objectives
- Consume log chunks from Redpanda
- Apply heuristics statelessly within each chunk
- Extract context from the chunk only (accept boundary limitations)
- Publish findings to Redpanda

### Deliverables

#### 4.1 Stateless Analysis Design

```
┌─────────────────────────────────────────────────────────────────┐
│                    Analyze Agent (Stateless)                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Consumer Group: destill-analyze                                │
│  Input Topic: destill.logs.raw                                  │
│  Output Topic: destill.analysis.findings                        │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │                  Process Single Chunk                      │ │
│  │                                                            │ │
│  │  1. Deserialize LogChunk                                   │ │
│  │         │                                                  │ │
│  │         ▼                                                  │ │
│  │  2. Split content into lines (in memory)                   │ │
│  │         │                                                  │ │
│  │         ▼                                                  │ │
│  │  3. For each line:                                         │ │
│  │     ┌────────────────────────────────────────────────────┐│ │
│  │     │  a. Detect severity (ERROR, FATAL, etc.)           ││ │
│  │     │  b. Skip if not error-like                         ││ │
│  │     │  c. Calculate confidence score                      ││ │
│  │     │  d. If match:                                       ││ │
│  │     │     - Extract 15 lines before (from THIS chunk)     ││ │
│  │     │     - Extract 30 lines after (from THIS chunk)      ││ │
│  │     │     - Normalize message, calculate hash             ││ │
│  │     │     - Create TriageCard                             ││ │
│  │     │     - Publish to destill.analysis.findings          ││ │
│  │     └────────────────────────────────────────────────────┘│ │
│  │                                                            │ │
│  │  Note: Context may be truncated at chunk boundaries.       │ │
│  │  This is acceptable for PoC - we get "best effort" context.│ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

#### 4.2 TriageCard with Chunk Context

```go
// TriageCard with chunk-aware context
type TriageCard struct {
    // Identity
    ID          string `json:"id"`
    RequestID   string `json:"request_id"`
    MessageHash string `json:"message_hash"`
    
    // Source
    Source    string `json:"source"`
    JobName   string `json:"job_name"`
    BuildURL  string `json:"build_url"`
    
    // Content
    Severity        string   `json:"severity"`
    RawMessage      string   `json:"raw_message"`
    NormalizedMsg   string   `json:"normalized_message"`
    ConfidenceScore float64  `json:"confidence_score"`
    
    // Context (from chunk only - may be truncated)
    PreContext     []string `json:"pre_context"`      // Up to 15 lines before
    PostContext    []string `json:"post_context"`     // Up to 30 lines after
    ContextNote    string   `json:"context_note"`     // e.g., "truncated at chunk start"
    
    // Chunk info (for debugging/tracing)
    ChunkIndex  int `json:"chunk_index"`
    LineInChunk int `json:"line_in_chunk"`
    
    // Metadata
    Metadata  map[string]string `json:"metadata"`
    Timestamp string            `json:"timestamp"`
}
```

### Acceptance Criteria
- [ ] Agent processes chunks without maintaining state between chunks
- [ ] Context extraction works within chunk boundaries
- [ ] Truncated context is noted in `ContextNote` field
- [ ] Findings are published with correct keys
- [ ] Agent can run multiple instances (consumer group scaling)

---

## Phase 5: The Connector

### Objectives
- Configure Redpanda Connect to sink findings to Postgres
- Handle JSONB transformation for context arrays

### Deliverables

#### 5.1 Redpanda Connect Configuration (`docker/connect.yaml`)

```yaml
input:
  kafka:
    addresses:
      - ${REDPANDA_BROKERS}
    topics:
      - destill.analysis.findings
    consumer_group: destill-postgres-sink
    start_from_oldest: true

pipeline:
  processors:
    - mapping: |
        root = this
        # Ensure arrays are proper JSON
        root.pre_context = this.pre_context.or([])
        root.post_context = this.post_context.or([])
        root.metadata = this.metadata.or({})

output:
  sql_insert:
    driver: postgres
    dsn: ${POSTGRES_DSN}
    table: findings
    columns:
      - request_id
      - build_url
      - job_name
      - message_hash
      - severity
      - confidence_score
      - raw_message
      - normalized_message
      - pre_context
      - post_context
      - source
      - line_number
      - chunk_index
      - metadata
    args_mapping: |
      root = [
        this.request_id,
        this.metadata.build_url.or(""),
        this.job_name,
        this.message_hash,
        this.severity,
        this.confidence_score,
        this.raw_message,
        this.normalized_message,
        this.pre_context.format_json(),
        this.post_context.format_json(),
        this.source,
        this.line_in_chunk,
        this.chunk_index,
        this.metadata.format_json()
      ]
    batching:
      count: 100
      period: 1s
```

### Acceptance Criteria
- [ ] Findings flow from Redpanda to Postgres automatically
- [ ] JSONB fields are properly formatted
- [ ] Batching improves throughput
- [ ] Dead letter queue configured for failures

---

## Phase 6: The TUI & Orchestration

### Objectives
- Implement mode detection (Legacy vs Agentic)
- Add `destill run` command with dual-mode support
- Add `destill view <request-id>` command for Postgres queries

### Deliverables

#### 6.1 Command Structure

```
destill
├── run <build-url>           # Analyze a build (auto-detects mode)
│   ├── [Legacy Mode]         # If REDPANDA_BROKERS not set
│   │   └── In-memory pipeline, streaming TUI
│   └── [Agentic Mode]        # If REDPANDA_BROKERS is set
│       └── Submit to Redpanda, print request ID
│
├── ingest                    # Run ingest agent (standalone)
│   └── Consumes requests, publishes chunks
│
├── analyze                   # Run analyze agent (standalone)
│   └── Consumes chunks, publishes findings
│
├── view <request-id>         # View findings from Postgres
│   └── Query Postgres, display in TUI
│
└── status <request-id>       # Check request status
    └── Query request progress
```

#### 6.2 Mode Detection Logic

```go
func DetectMode() PipelineMode {
    if os.Getenv("REDPANDA_BROKERS") != "" {
        return AgenticMode
    }
    return LegacyMode
}

func (cmd *RunCmd) Execute(buildURL string) error {
    mode := DetectMode()
    
    switch mode {
    case LegacyMode:
        // Current behavior: in-memory broker, streaming TUI
        pipeline := legacy.NewPipeline(cfg)
        return runLegacyTUI(pipeline, buildURL)
        
    case AgenticMode:
        // New behavior: submit to Redpanda, print request ID
        pipeline := agentic.NewPipeline(cfg)
        requestID, err := pipeline.Submit(ctx, buildURL)
        if err != nil {
            return err
        }
        fmt.Printf("✅ Submitted analysis request: %s\n", requestID)
        fmt.Printf("   Build URL: %s\n", buildURL)
        fmt.Println()
        fmt.Printf("View results: destill view %s\n", requestID)
        return nil
    }
}
```

#### 6.3 View Command

```go
func (cmd *ViewCmd) Execute(requestID string) error {
    store := postgres.NewStore(cfg.PostgresDSN)
    defer store.Close()
    
    // Fetch findings for request
    findings, err := store.GetFindings(ctx, requestID)
    if err != nil {
        return err
    }
    
    // Launch TUI with findings
    return tui.Start(findings)
}
```

### Acceptance Criteria
- [ ] `destill run` works in both modes based on env vars
- [ ] Legacy mode preserves current streaming TUI behavior
- [ ] Agentic mode submits request and prints ID
- [ ] `destill view` queries Postgres and displays in TUI
- [ ] `destill status` shows request progress

---

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `REDPANDA_BROKERS` | Redpanda broker addresses | (empty = legacy mode) |
| `POSTGRES_DSN` | Postgres connection string | (required for agentic) |
| `BUILDKITE_API_TOKEN` | Buildkite API token | (required) |

---

## Migration Path

### Step 1: Parallel Operation
- Keep existing code working (Legacy mode)
- Add new Agentic infrastructure alongside
- Feature flag via `REDPANDA_BROKERS` env var

### Step 2: Feature Parity
- Ensure Agentic mode produces same findings as Legacy
- Compare outputs for same builds
- Fix any discrepancies

### Step 3: Gradual Adoption
- Start using Agentic mode for non-critical analysis
- Monitor Postgres storage, Redpanda throughput
- Tune chunk sizes, context lines as needed

### Step 4: Deprecate Legacy
- Once Agentic mode is stable, deprecate Legacy
- Remove in-memory broker code
- Simplify to single code path

---

## Open Questions

1. **Chunk boundary context**: Accept truncation for PoC, or implement chunk stitching later?
2. **Deduplication**: Should Postgres dedupe by message_hash per request, or keep all?
3. **Retention**: How long to keep findings in Postgres? Add TTL?
4. **Scaling**: How many analyze agent instances for large builds?

---

## Timeline Estimate

| Phase | Effort | Dependencies |
|-------|--------|--------------|
| Phase 1: Infrastructure | 2-3 days | None |
| Phase 2: Data Modeling | 1-2 days | Phase 1 |
| Phase 3: Ingest Agent | 2-3 days | Phase 2 |
| Phase 4: Analyze Agent | 2-3 days | Phase 2 |
| Phase 5: Connector | 1 day | Phase 1, 4 |
| Phase 6: TUI & Orchestration | 2-3 days | Phase 3, 4, 5 |

**Total: ~10-15 days**

