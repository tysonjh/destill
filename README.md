# Destill Agent

A decoupled, agent-based log triage tool for CI/CD pipelines that helps analyze and categorize build failures using stream processing architecture.

## Overview

Destill is designed to process CI/CD build logs (currently targeting Buildkite) and produce ranked failure cards to help developers quickly identify and triage build failures. The system uses a stream processing architecture with independent agents communicating via message topics.

### Architecture

The system consists of two main agents:

- **Ingestion Agent**: Consumes build requests, fetches raw logs from CI systems, and publishes them to the `ci_logs_raw` topic
- **Analysis Agent**: Processes raw logs, performs analysis (normalization, severity detection, confidence scoring), and publishes ranked failure cards to the `ci_failures_ranked` topic

### Key Topics

- `destill_requests`: Build analysis requests (input)
- `ci_logs_raw`: Raw log chunks from CI jobs
- `ci_failures_ranked`: Analyzed and ranked failure cards (output)

## Prerequisites

- Go 1.24.10 or later
- Buildkite API Token (for fetching build logs)

## Configuration

The application requires a Buildkite API token to fetch build logs. Create an API access token in your Buildkite account:

1. Go to [Buildkite Personal Settings > API Access Tokens](https://buildkite.com/user/api-access-tokens)
2. Create a new token with `read_builds` and `read_job_env` scopes
3. Set the token as an environment variable:

```bash
export BUILDKITE_API_TOKEN="your-token-here"
```

## Project Structure

The codebase is organized into the following high-level packages:

- **`src/broker/`** - Message broker implementations (in-memory channel-based broker for local development)
- **`src/buildkite/`** - Buildkite API client for fetching build metadata and job logs
- **`src/cmd/analysis/`** - Analysis Agent that processes raw logs and produces ranked failure cards
- **`src/cmd/ingestion/`** - Ingestion Agent that fetches logs from CI systems and publishes raw log data
- **`src/cmd/cli/`** - Main CLI application and orchestrator using Cobra framework
- **`src/config/`** - Configuration management (environment variables, API tokens)
- **`src/contracts/`** - Shared interfaces (MessageBroker) and data structures (TriageCard, LogChunk)
- **`src/pipeline/`** - End-to-end integration tests for the stream processing pipeline

## Building

```bash
# Build the CLI
go build -o destill ./src/cmd/cli

# Run tests
go test ./...
```

## Usage

### Submitting a Build for Analysis

Before running the application, ensure the Buildkite API token is set:

```bash
export BUILDKITE_API_TOKEN="your-token-here"
```

Submit a Buildkite build URL for analysis:

```bash
./destill build https://buildkite.com/org/pipeline/builds/4091
```

This command:
1. Creates a request with a unique ID
2. Publishes it to the `destill_requests` topic
3. The Ingestion Agent parses the URL and fetches build metadata from Buildkite
4. For each job in the build, fetches the raw log content via the Buildkite API
5. Publishes individual LogChunks (one per job) to the `ci_logs_raw` topic
6. The Analysis Agent processes logs and produces ranked failure cards

### Starting the Analysis Mode

Launch the interactive TUI (Terminal UI) for log analysis:

```bash
./destill analyze
```

This starts the stream processing pipeline with both agents running as persistent goroutines, ready to process build requests.

## Development

### Message Broker

The project currently uses an in-memory channel-based broker (`InMemoryBroker`) for local development. This simulates a Kafka-like streaming API and can be replaced with a production message broker (e.g., Kafka, NATS) by implementing the `MessageBroker` interface.

### Analysis Pipeline

The Analysis Agent currently implements placeholder logic for:
- **Log normalization**: Removes dynamic values for pattern matching
- **Severity detection**: Determines log level from content
- **Confidence scoring**: Ranks failures by importance
- **Message hashing**: Tracks recurring failures

### Testing

Run unit tests:

```bash
go test ./src/broker
go test ./src/cmd/analysis
go test ./src/cmd/ingestion
```

Run integration tests:

```bash
go test ./src/pipeline
```

## Dependencies

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- Bazel Rules for Go - Build system integration

## Future Roadmap

- [ ] Implement actual Buildkite API integration
- [ ] Build interactive TUI with Bubble Tea
- [ ] Add support for GitHub Actions
- [ ] Implement ML-based failure classification
- [ ] Add persistence layer for historical analysis
- [ ] Add persistance layer for human-triaged signals from CLI that can be used in the Analysis stage to build up organizational context
- [ ] Support for production message brokers (Redpanda/Kafka)

## License

MIT