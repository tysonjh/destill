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

Findings receive confidence scores (0.0–1.0) based on pattern matching. Boost patterns include stack traces, exit codes, and build tool errors. Penalty patterns include test expectations, handled errors, and success messages.

Job outcome adjusts scores after pattern matching:

- **Failed jobs**: Scores increase asymptotically toward 1.0, preserving relative ordering.
- **Passed jobs**: Scores decrease proportionally. Errors from passing jobs are often teardown noise or expected test output.

Findings from failed jobs always rank above findings from passed jobs. The TUI sorts by confidence to surface likely root causes first.

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
