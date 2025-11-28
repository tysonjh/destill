# Destill

A log triage tool for CI/CD pipelines that helps developers quickly identify and prioritize build failures.

## Quick Start

### Prerequisites

- Go 1.24.10 or later
- Buildkite API Token

### Installation

```bash
# Clone the repository
git clone https://github.com/tysonjh/destill-agent.git
cd destill-agent

# Build the CLI
go build -o destill ./src/cmd/cli
```

### Configuration

1. Get a Buildkite API token from [Buildkite API Access Tokens](https://buildkite.com/user/api-access-tokens)
   - Required scopes: `read_builds`, `read_job_env`

2. Set the environment variable:
```bash
export BUILDKITE_API_TOKEN="your-token-here"
```

## Usage

### Analyze a Build

Submit a Buildkite build URL for analysis:

```bash
./destill build https://buildkite.com/org/pipeline/builds/4091
```

The system will:
1. Fetch build metadata and job logs from Buildkite
2. Analyze logs to detect failures and calculate confidence scores
3. Output ranked failure cards sorted by priority

### Interactive Mode

Launch the interactive TUI for real-time log analysis:

```bash
./destill analyze
```

## Building from Source

```bash
# Build
go build -o destill ./src/cmd/cli

# Run all tests
go test ./...

# Run specific tests
go test ./src/cmd/analysis    # Analysis agent tests
go test ./src/cmd/ingestion   # Ingestion agent tests
go test ./src/broker          # Message broker tests
```

## Documentation

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** - System design and engineering details
- **[project_notes/](./project_notes/)** - Developer log and implementation notes

## License

MIT