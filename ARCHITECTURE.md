# 🏛️ DESTILL Architecture Overview

Destill is a distributed intelligence system designed to minimize Mean Time To Resolution (MTTR) for CI/CD failures by identifying recurring error patterns. The architecture follows a simple, three-phase stream processing pipeline, with each phase communicating via a central Message Broker (NATS).

---

## 1. Core Data Flow 

All data within Destill flows through two main subjects (queues) managed by the Message Broker:

* **`ci_requests_raw` (Producer: CLI/User):** Receives requests (e.g., a Buildkite URL) for a build to analyze.
* **`ci_logs_raw` (Producer: Ingestion Agent):** Receives raw log chunks and build metadata ready for analysis.
* **`ci_failures_ranked` (Producer: Analysis Agent):** Receives the final, intelligent `TriageCard` objects, sorted and hash-tagged for recurrence.

---

## 2. Phase 1: Ingestion Agent (The Producer) 🎣

The Ingestion Agent is the system's entry point, responsible for fetching raw data from external services (like Buildkite) and translating it into a stream format.

* **Input Subject:** `ci_requests_raw`
* **Core Responsibilities:**
    * **External Integration:** Uses the Buildkite API Client to fetch build and job metadata.
    * **Data Serialization:** Takes raw log output (large text files) and breaks it down into small, digestible **`LogChunk`** messages.
    * **Stream Publishing:** Publishes raw log data to the `ci_logs_raw` subject for processing by Phase 2.
* **Goal:** Ensure **high throughput** and reliable delivery of all raw, necessary data.

---

## 3. Phase 2: Analysis Agent (The Intelligence) 🧠

The Analysis Agent is the intelligence core of Destill, responsible for transforming raw, noisy log data into actionable, high-signal failure patterns.

* **Input Subject:** `ci_logs_raw`
* **Core Responsibilities:**
    * **Log Normalization (Recurrence):** Performs a 10-step cleaning process to strip non-deterministic noise (timestamps, UUIDs, PIDs). This is crucial for recurrence tracking.
    * **Hashing:** Calculates the **`MessageHash` (SHA256)** on the normalized content. This hash is the immutable ID for recognizing recurring failures across different builds.
    * **Severity Detection (Permissive):** Tags the log line with a severity (e.g., `ERROR`) using simple, permissive keyword matching (high recall).
    * **Confidence Scoring (High Precision):** Calculates the **`ConfidenceScore`** based on structural quality (e.g., stack trace presence, line-start anchors) and applies **aggressive penalties** for known Type A/noise patterns (e.g., connection retries, deprecation warnings).
* **Output Subject:** `ci_failures_ranked` (Publishes the final `TriageCard` objects).

---

## 4. Phase 3: CLI Orchestrator and TUI Display (The Consumer) 🖥️

This phase provides the direct user interface for the engineer to triage failures and find the root cause quickly.

* **Input Subject:** `ci_failures_ranked`
* **Core Responsibilities:**
    * **Data Consumption:** Subscribes to the ranked failure stream.
    * **Triage Prioritization:** Presents the list of failures in the TUI, **defaulting to sort by `ConfidenceScore` (descending)**. This ensures Type B (high-leverage) failures are always viewed first.
    * **Recurrence Grouping:** Aggregates and displays failures based on the **`MessageHash`** and **Recurrence Count**, enabling the engineer to fix the pattern, not the instance.
* **Goal:** Provide a clean, prioritized UX to minimize the engineer's time spent browsing logs.

---

## 5. Key Design Decisions

### 5.1 Permissive Detection + Confident Scoring

The system uses a two-layer approach:
- **Severity Detection (High Recall):** Casts a wide net to catch all potential errors
- **Confidence Scoring (High Precision):** Applies bonuses and penalties to surface actionable failures

### 5.2 Message Hashing for Recurrence

By normalizing logs (stripping timestamps, UUIDs, IPs, etc.) and hashing the result, the system can identify when the "same" error occurs across different builds, enabling:
- Deduplication
- Pattern recognition
- Trend analysis
- Alert fatigue reduction

### 5.3 Penalty Patterns

High-confidence penalty patterns reduce false positives:
- **Transient failures** (connection reset + retry): -0.30
- **Address already in use**: -0.25
- **Tests passed summaries**: -0.35
- **Deprecation warnings**: -0.20

---

## 6. Package Structure

```
src/
├── broker/          # Message broker implementations
├── buildkite/       # Buildkite API client
├── cmd/
│   ├── analysis/    # Analysis Agent
│   ├── cli/         # CLI orchestrator
│   └── ingestion/   # Ingestion Agent
├── config/          # Configuration management
├── contracts/       # Shared interfaces and data structures
└── pipeline/        # Integration tests
```

---

## 7. Future Roadmap

- **Phase 3**: TUI interface with Bubble Tea
- **Phase 4**: Persistence layer and historical analysis
- **Phase 5**: Multi-platform support (GitHub Actions, GitLab CI)
- **Phase 6**: ML-based pattern learning and classification
