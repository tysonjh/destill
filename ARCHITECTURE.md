# Architecture

Destill uses a distributed streaming architecture with stateless agents.

## Data flow

```
CLI submit → Redpanda → Ingest Agent → Redpanda → Analyze Agent → Redpanda → Postgres → CLI view
```

In local mode, an in-memory broker replaces Redpanda and agents run as goroutines.

## Design principles

### Stateless agents

Both ingest and analyze agents maintain no cross-request state. Each message is processed independently. This enables horizontal scaling by adding agent instances.

### Chunked processing

Logs are split into chunks with overlap between them. Chunking keeps message sizes manageable and enables parallel analysis. Context extraction operates within chunk boundaries.

### Confidence scoring

Findings receive confidence scores based on pattern matching. Errors from failed jobs receive boosted scores. The TUI sorts findings by confidence to surface likely root causes first.

### Message keying

- Log chunks: keyed by build ID for ordering
- Findings: keyed by request ID for grouping

Multiple agents in the same consumer group automatically load-balance work.

## Components

| Component | Purpose |
|-----------|---------|
| `destill` | CLI with `analyze`, `submit`, `view` commands |
| `destill-ingest` | Fetches logs from CI platforms, produces chunks |
| `destill-analyze` | Analyzes chunks, produces findings |

## Infrastructure (distributed mode)

| Service | Purpose |
|---------|---------|
| Redpanda | Message broker |
| Postgres | Persistent storage |
| Redpanda Connect | Kafka-to-Postgres sink |
