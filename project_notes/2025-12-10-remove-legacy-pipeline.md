# Removing LegacyPipeline While Preserving Local Single-Process Mode

## Executive Summary

The `LegacyPipeline` in `src/pipeline/legacy.go` is an **incomplete stub** from Phase 2 of the refactoring plan. The **actual working legacy implementation** is in `src/cmd/cli/main.go`, which already provides a fully functional local single-process mode with embedded agents.

Since Phase 6 is complete and the agentic architecture is fully implemented, we can now clean up the redundant pipeline abstraction while preserving the valuable local development mode.

**Additionally**, this refactoring significantly improves the distributed mode UX based on design discussions:

### What Changes

**Removed:**
- ❌ `src/pipeline/legacy.go` - Incomplete stub, never fully implemented
- ❌ `src/cmd/cli/main.go` or `src/cmd/destill-cli/main.go` - Consolidate to one unified CLI
- ❌ `src/broker/adapter.go` (LegacyAdapter) - Update agents to use `broker.Broker` directly
- ❌ `src/contracts/broker.go` (old MessageBroker interface) - Single broker interface
- ❌ `--detach` flag - Confusing, replaced with clearer `--no-tui`
- ❌ `analyze` command - Obsolete edge case

**Improved:**
- ✅ Unified `run` command that launches TUI in **both modes** (local streaming, distributed polling)
- ✅ **UUID request IDs** - Unique per analysis (supports retries/reruns)
- ✅ **CLI accepts both formats** - Build URL (convenience) OR request ID (explicit)
- ✅ Progressive TUI in distributed mode (polls Postgres every 2 seconds)
- ✅ `--no-tui` flag for headless automation (clear intent)
- ✅ `--cache` flag for development iteration (load pre-saved results)
- ✅ `list` command for viewing multiple analyses per build

### User Experience Improvement

**Before (distributed mode is clunky):**
```bash
$ destill run <url>         # Exits immediately, no feedback
$ destill status <uuid>     # Manual polling, need to remember UUID
$ destill status <uuid>     # Check again...
$ destill view <uuid>       # Finally view results
```

**After (distributed mode is interactive):**
```bash
$ destill run <url>              # Prints request ID, launches progressive TUI
                                 # User sees results appear in real-time
                                 # Can press 'q' to exit, analysis continues

$ destill view <url>             # Later: View latest by build URL
$ destill view <request-id>      # Or: View specific by request ID
$ destill list <url>             # List all analyses for build
```

## Current State Analysis

### Three Entry Points (Redundant!)

1. **`src/cmd/cli/main.go`** → `bin/destill-legacy`
   - ✅ **Fully functional** local single-process mode
   - Launches agents as goroutines in same process
   - Uses in-memory broker with LegacyAdapter
   - Provides streaming TUI with real-time updates
   - Commands: `build`, `analyze`

2. **`src/cmd/destill-cli/main.go`** → `bin/destill`
   - ⚠️ **Incomplete** - uses the stub `LegacyPipeline`
   - Mode detection based on `REDPANDA_BROKERS` env var
   - Legacy mode just prints warning (line 99)
   - Agentic mode works (submits to Redpanda)
   - Commands: `run`, `view`, `status`

3. **Standalone Agent Binaries**
   - `src/cmd/ingest-agent/main.go` → `bin/destill-ingest`
   - `src/cmd/analyze-agent/main.go` → `bin/destill-analyze`
   - ✅ Used for distributed agentic mode

### The Pipeline Abstraction

Located in `src/pipeline/`:
- `pipeline.go` - Interface and mode detection
- `legacy.go` - **Stub implementation** (never fully wired up)
- `agentic.go` - Full implementation for distributed mode

The `LegacyPipeline` was intended to wrap the legacy mode, but **the actual legacy implementation bypassed it entirely** and lives in `src/cmd/cli/main.go`.

## Recommended Approach: Consolidate to Single Unified CLI

The cleanest solution is to **merge the best parts of both CLIs** into a unified entry point.

### Goals

1. ✅ Preserve local single-process mode for development/demos
2. ✅ Keep distributed agentic mode for production
3. ✅ Remove redundant pipeline abstraction
4. ✅ Maintain all existing functionality
5. ✅ Simplify codebase

### Architecture After Refactoring

```
destill run <build-url>  →  ALWAYS launches TUI (unless --no-tui)

┌─────────────────────────────────────────────────────────────────────┐
│ Local Mode (no REDPANDA_BROKERS)                                   │
├─────────────────────────────────────────────────────────────────────┤
│ • InMemoryBroker (implements broker.Broker)                         │
│ • Launches agents as goroutines in same process                     │
│ • Agents use broker.Broker interface directly                       │
│ • Context-aware execution with graceful shutdown                    │
│ • Streaming TUI with real-time results                              │
│ • Best for: Local development, demos, quick analysis                │
│                                                                      │
│ Commands:                                                            │
│   destill run <build-url>         → Streaming TUI                   │
│   destill run <build-url> --cache → Load pre-saved cards (dev)      │
│   destill run <build-url> --no-tui → Submit only (rare)             │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│ Distributed Mode (REDPANDA_BROKERS set)                            │
├─────────────────────────────────────────────────────────────────────┤
│ • Redpanda broker + Postgres                                        │
│ • Agents run as separate processes (started independently)          │
│ • Submits request, generates UUID request-id                        │
│ • Prints request-id to terminal                                     │
│ • Polls Postgres every 2 seconds for new findings                   │
│ • Progressive TUI with updates as findings arrive                   │
│ • Best for: Production, scalability, persistence                    │
│                                                                      │
│ Commands:                                                            │
│   destill run <build-url>          → Submit + Progressive TUI       │
│   destill run <build-url> --no-tui → Submit only (headless)         │
│   destill view <build-url>         → View historical results        │
│   destill status <build-url>       → Check progress                 │
│                                                                      │
│ Future: destill list <build-url>  → List all analyses for build     │
│         (handle multiple retries/executions per build)              │
└─────────────────────────────────────────────────────────────────────┘
```

**Key Design Decisions:**

1. **`--detach` removed entirely** - Use `--no-tui` instead (clearer intent)
2. **Request-id is UUID** - Unique per analysis run (supports retries/reruns)
3. **CLI accepts both formats** - Build URL (convenience, gets latest) OR request-id (explicit)
4. **Postgres polling in distributed mode** - Updates every 2 seconds (pragmatic, simple)
5. **Print request-id in output** - Users can reference specific analysis runs
6. **`list` command** - Show all analyses for a build when multiple exist

### Request ID Design: UUID with Smart CLI

**The Problem:**
- Build URLs are not unique (retries, reruns create multiple analyses)
- Users don't want to track UUIDs manually
- Need to support both "latest analysis" and "specific analysis" use cases

**The Solution:** Keep UUID request IDs, but make CLI accept both formats.

#### Database Schema

```sql
CREATE TABLE requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) UNIQUE NOT NULL,  -- UUID for internal tracking
    build_url TEXT NOT NULL,                   -- User-facing identifier
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    chunks_total INTEGER DEFAULT 0,
    chunks_processed INTEGER DEFAULT 0,
    findings_count INTEGER DEFAULT 0,

    -- Allow multiple analyses per build (retries, re-runs)
    CONSTRAINT requests_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

-- Index for fast lookup by build URL
CREATE INDEX idx_requests_build_url ON requests(build_url);
CREATE INDEX idx_requests_created_at ON requests(created_at DESC);
```

**Note:** No UNIQUE constraint on `build_url` - allows multiple analyses per build!

#### CLI Resolution Logic

The CLI accepts both formats and resolves to a request ID:

```go
// ResolveIdentifier converts user input (build URL or request ID) to a request ID
func ResolveIdentifier(ctx context.Context, arg string, store Store) (string, error) {
    // If it looks like a UUID, use it directly
    uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
    if uuidPattern.MatchString(arg) {
        // Verify it exists
        _, err := store.GetRequestStatus(ctx, arg)
        if err != nil {
            return "", fmt.Errorf("request not found: %s", arg)
        }
        return arg, nil
    }

    // If it's a build URL, look up latest request
    buildURLPattern := regexp.MustCompile(`buildkite\.com/.+/builds/\d+`)
    if buildURLPattern.MatchString(arg) {
        req, err := store.GetLatestRequestForBuild(ctx, arg)
        if err != nil {
            return "", fmt.Errorf("no analysis found for build: %s", arg)
        }

        // Inform user which analysis is being used
        fmt.Printf("📂 Using latest analysis from %s (request: %s)\n",
            formatRelativeTime(req.CreatedAt), req.RequestID[:8]+"...")

        return req.RequestID, nil
    }

    return "", fmt.Errorf("invalid identifier: %s (expected build URL or request ID)", arg)
}
```

#### Store Methods

```go
// GetLatestRequestForBuild returns the most recent analysis for a build
func (s *PostgresStore) GetLatestRequestForBuild(ctx context.Context, buildURL string) (*RequestInfo, error) {
    var req RequestInfo
    err := s.db.QueryRowContext(ctx,
        `SELECT request_id, build_url, status, created_at, findings_count
         FROM requests
         WHERE build_url = $1
         ORDER BY created_at DESC
         LIMIT 1`,
        buildURL,
    ).Scan(&req.RequestID, &req.BuildURL, &req.Status, &req.CreatedAt, &req.FindingsCount)

    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("no analysis found for build: %s", buildURL)
    }
    return &req, err
}

// GetAllRequestsForBuild returns all analyses for a build (for list command)
func (s *PostgresStore) GetAllRequestsForBuild(ctx context.Context, buildURL string) ([]RequestInfo, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT request_id, build_url, status, created_at, findings_count
         FROM requests
         WHERE build_url = $1
         ORDER BY created_at DESC`,
        buildURL,
    )
    defer rows.Close()

    var requests []RequestInfo
    for rows.Next() {
        var req RequestInfo
        if err := rows.Scan(&req.RequestID, &req.BuildURL, &req.Status, &req.CreatedAt, &req.FindingsCount); err != nil {
            return nil, err
        }
        requests = append(requests, req)
    }

    return requests, rows.Err()
}
```

#### User Experience Examples

**Creating a new analysis:**
```bash
$ destill run https://buildkite.com/acme/api/4091

🚀 Submitting analysis request...
📋 Request ID: 550e8400-e29b-41d4-a716-446655440000
🔍 Analyzing build: acme/api/4091

[Progressive TUI with findings appearing as they're analyzed]

Progress: ━━━━━━━━━━━━━━━━━━━━━━━━ 8/10 jobs
Findings: 24 errors, 12 warnings

# User presses 'q' to exit, analysis continues
# Request ID is saved in output for later reference
```

**Viewing by build URL (gets latest):**
```bash
$ destill view https://buildkite.com/acme/api/4091

📂 Using latest analysis from 2 hours ago (request: 550e8400...)
✅ Found 24 findings

[TUI launches with results]
```

**Viewing by request ID (explicit):**
```bash
$ destill view 550e8400-e29b-41d4-a716-446655440000

📂 Loading analysis...
✅ Found 24 findings

[TUI launches with results]
```

**Listing all analyses for a build:**
```bash
$ destill list https://buildkite.com/acme/api/4091

📂 Found 3 analyses for build: acme/api/4091

  1. 550e8400... (2 hours ago)   - completed (24 findings)  [Latest]
  2. 7a3b9c12... (1 day ago)     - completed (18 findings)
  3. 9f4e2d8a... (2 days ago)    - failed (connection timeout)

# View specific analysis
$ destill view 7a3b9c12-4567-89ab-cdef-0123456789ab
```

**Why this works:**
- ✅ UUID ensures uniqueness (retries/reruns get new IDs)
- ✅ Build URL for convenience (most common: "show me latest")
- ✅ Request ID for explicit reference ("show me THAT specific run")
- ✅ `list` command when user needs to choose between multiple runs

### Unified Command Structure

**The philosophy:** One command (`run`) does the right thing in both modes.

#### Command Comparison

| Command | Local Mode | Distributed Mode |
|---------|------------|------------------|
| `run <url>` | Submit → Streaming TUI | Submit → Progressive TUI (Postgres polling) |
| `run <url> --cache <file>` | Load from cache → TUI | N/A (ignored) |
| `run <url> --no-tui` | Submit only (rare) | Submit only (headless automation) |
| `view <url\|uuid>` | N/A (data doesn't persist) | Query Postgres → TUI (accepts both formats) |
| `status <url\|uuid>` | N/A (data doesn't persist) | Query Postgres → Show progress (accepts both) |
| `list <url>` | N/A (data doesn't persist) | Show all analyses for build |

#### What Gets Removed

**Legacy commands that are gone:**
1. **`analyze`** - Obsolete. Was designed for edge case of "persistent broker + in-memory CLI"
2. **`build`** - Renamed to `run` (clearer, shorter)
3. **`--detach` flag** - Replaced with `--no-tui` (clearer intent)

#### Example Workflows

**Local development (in-memory):**
```bash
# Quick analysis with streaming results
destill run https://buildkite.com/acme/api/4091

# Development iteration (skip API calls)
destill run https://buildkite.com/acme/api/4091 --cache previous-run.json
```

**Production (distributed):**
```bash
# Interactive: Submit and watch
destill run https://buildkite.com/acme/api/4091
# Prints request ID: 550e8400-e29b-41d4-a716-446655440000
# [Progressive TUI polls Postgres, user sees results appear]
# User can press 'q' to exit, analysis continues in background

# Headless: Submit from CI/automation
destill run https://buildkite.com/acme/api/4091 --no-tui
# Prints request ID and exits (no TUI)

# View latest by build URL (convenience)
destill view https://buildkite.com/acme/api/4091
# → Looks up latest request ID internally

# View specific by request ID (explicit)
destill view 550e8400-e29b-41d4-a716-446655440000
# → Loads this exact analysis

# List all analyses for build
destill list https://buildkite.com/acme/api/4091
# → Shows all request IDs with metadata

# Check progress (accepts either format)
destill status https://buildkite.com/acme/api/4091
destill status 550e8400-e29b-41d4-a716-446655440000
```

**Why this is better:**
- ✅ Consistent interface across modes
- ✅ Request ID printed for reference, but not required for common operations
- ✅ Build URL for convenience (90% of use cases: "show me latest")
- ✅ Request ID for explicit control (retries, comparing different runs)
- ✅ `list` command handles multiple analyses per build gracefully
- ✅ Supports both interactive and automation use cases

## Implementation Plan

### Phase 0: Update Agents to Use broker.Broker Interface

**Remove the LegacyAdapter by updating agents to use the `broker.Broker` interface directly.**

#### Changes to Agent Interfaces

**1. Update `src/cmd/ingestion/agent.go`:**
```go
type Agent struct {
    msgBroker       broker.Broker  // Changed from contracts.MessageBroker
    buildkiteClient *buildkite.Client
    logger          logger.Logger
}

func NewAgent(msgBroker broker.Broker, buildkiteAPIToken string, log logger.Logger) *Agent {
    return &Agent{
        msgBroker:       msgBroker,
        buildkiteClient: buildkite.NewClient(buildkiteAPIToken),
        logger:          log,
    }
}

func (a *Agent) Run(ctx context.Context) error {  // Added context parameter
    requestChannel, err := a.msgBroker.Subscribe(ctx, "destill_requests", "ingestion-agent")
    if err != nil {
        return fmt.Errorf("failed to subscribe to destill_requests: %w", err)
    }

    a.logger.Info("[IngestionAgent] Listening for requests on 'destill_requests' topic...")

    for {
        select {
        case msg, ok := <-requestChannel:
            if !ok {
                return nil  // Channel closed
            }
            if err := a.processRequest(msg.Value); err != nil {  // Extract Value from Message
                a.logger.Error("[IngestionAgent] Error processing request: %v", err)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

**2. Update `src/cmd/analysis/agent.go`:**
```go
type Agent struct {
    msgBroker broker.Broker  // Changed from contracts.MessageBroker
    logger    logger.Logger
}

func NewAgent(msgBroker broker.Broker, log logger.Logger) *Agent {
    return &Agent{
        msgBroker: msgBroker,
        logger:    log,
    }
}

func (a *Agent) Run(ctx context.Context) error {  // Added context parameter
    logChannel, err := a.msgBroker.Subscribe(ctx, "ci_logs_raw", "analysis-agent")
    if err != nil {
        return fmt.Errorf("failed to subscribe to ci_logs_raw: %w", err)
    }

    a.logger.Info("[AnalysisAgent] Listening for logs on 'ci_logs_raw' topic...")

    for {
        select {
        case msg, ok := <-logChannel:
            if !ok {
                return nil  // Channel closed
            }
            if err := a.processLogChunk(ctx, msg.Value); err != nil {
                a.logger.Error("[AnalysisAgent] Error processing chunk: %v", err)
            }
        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

#### Changes to Tests

**Update `src/cmd/analysis/agent_test.go` and `src/cmd/ingestion/agent_test.go`:**
```go
func TestAnalysisAgentUnmarshalsLogChunk(t *testing.T) {
    inmemBroker := broker.NewInMemoryBroker()
    defer inmemBroker.Close()

    ctx := context.Background()

    // Subscribe to output topic (new broker interface)
    outputChan, err := inmemBroker.Subscribe(ctx, "ci_failures_ranked", "test-group")
    if err != nil {
        t.Fatalf("Failed to subscribe to ci_failures_ranked: %v", err)
    }

    // Create and start agent (no more LegacyAdapter!)
    agent := analysis.NewAgent(inmemBroker, logger.NewSilentLogger())
    go func() {
        _ = agent.Run(ctx)
    }()

    // Publish test data
    if err := inmemBroker.Publish(ctx, "ci_logs_raw", "", data); err != nil {
        t.Fatalf("Failed to publish LogChunk: %v", err)
    }

    // Wait for output
    select {
    case msg := <-outputChan:
        var triageCard contracts.TriageCard
        if err := json.Unmarshal(msg.Value, &triageCard); err != nil {
            t.Fatalf("Failed to unmarshal output: %v", err)
        }
        // ... assertions
    case <-time.After(2 * time.Second):
        t.Fatal("Timeout waiting for TriageCard output")
    }
}
```

#### Files to Delete

- `src/broker/adapter.go` - LegacyAdapter no longer needed
- `src/contracts/broker.go` - Old MessageBroker interface removed

### Phase 1: Enhance Local Mode in destill-cli

**Update `src/cmd/destill-cli/main.go`** to include the working agent orchestration.

#### Changes Needed:

1. **Add agent orchestration function** (no more LegacyAdapter!):
```go
func startLocalPipeline(ctx context.Context, cfg *config.Config, log logger.Logger) (broker.Broker, func()) {
    // Create in-memory broker (implements broker.Broker directly)
    inMemBroker := broker.NewInMemoryBroker()

    // Start ingestion agent
    ingestionAgent := ingestion.NewAgent(inMemBroker, cfg.BuildkiteAPIToken, log)
    go func() {
        if err := ingestionAgent.Run(ctx); err != nil && err != context.Canceled {
            log.Error("[Pipeline] Ingestion agent error: %v", err)
        }
    }()

    // Start analysis agent
    analysisAgent := analysis.NewAgent(inMemBroker, log)
    go func() {
        if err := analysisAgent.Run(ctx); err != nil && err != context.Canceled {
            log.Error("[Pipeline] Analysis agent error: %v", err)
        }
    }()

    // Return broker and cleanup function
    cleanup := func() {
        inMemBroker.Close()
    }

    return inMemBroker, cleanup
}
```

2. **Update `runLegacyMode` function** to:
   - Start agents using `startLocalPipeline()`
   - Subscribe to `ci_failures_ranked` topic
   - Publish request to `destill_requests` topic
   - Launch streaming TUI (from `cli/main.go:211-320`)

3. **Add logger** to the CLI:
```go
var log logger.Logger

func init() {
    log = logger.NewConsoleLogger()
}
```

4. **Import necessary packages**:
```go
import (
    "destill-agent/src/cmd/analysis"
    "destill-agent/src/cmd/ingestion"
    "destill-agent/src/logger"
)
```

### Phase 2: Remove LegacyPipeline

**Files to delete**:
- `src/pipeline/legacy.go` - The incomplete stub
- `src/pipeline/pipeline.go` - Remove `Pipeline` interface and `Mode` enum
- Keep only `src/pipeline/agentic.go` (rename to just handle distributed logic)

**Alternative**: Keep `pipeline.go` but simplify it to only contain shared types like `RequestStatus` and `Config`.

### Phase 3: Remove Redundant CLI

**Decision point**: Keep one unified CLI

**Option A: Keep `destill-cli` (recommended)**
- More modern structure
- Already has `view` and `status` commands
- Just needs local mode agents wired up
- Remove `src/cmd/cli/` entirely

**Option B: Keep `cli`**
- Already has working local mode
- Would need to add `view` and `status` commands
- Would need mode detection logic
- Remove `src/cmd/destill-cli/` entirely

**Recommendation: Option A** - `destill-cli` has better command structure

### Phase 4: Update Build System

**Update `Makefile`**:
```makefile
.PHONY: all build clean test help

all: build

# Build all binaries
build:
	@echo "Building destill CLI..."
	@go build -o bin/destill ./src/cmd/destill-cli
	@echo "Building standalone agents..."
	@go build -o bin/destill-ingest ./src/cmd/ingest-agent
	@go build -o bin/destill-analyze ./src/cmd/analyze-agent

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@echo "Clean complete"

test:
	@echo "Running tests..."
	@go test ./src/... -v

help:
	@echo "Destill Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  all     - Build all binaries (default)"
	@echo "  build   - Build all binaries"
	@echo "  clean   - Remove build artifacts"
	@echo "  test    - Run all tests"
	@echo "  help    - Show this help message"
	@echo ""
	@echo "Binaries:"
	@echo "  bin/destill         - Unified CLI (local + distributed modes)"
	@echo "  bin/destill-ingest  - Standalone ingest agent (for distributed)"
	@echo "  bin/destill-analyze - Standalone analyze agent (for distributed)"
```

Remove `build-legacy` and `build-agentic` targets.

### Phase 5: Update Documentation

**Update README or docs** to reflect:
- Single unified CLI with automatic mode detection
- Local mode is the default (no configuration needed)
- Distributed mode requires `REDPANDA_BROKERS` and `POSTGRES_DSN`
- Example usage for both modes

## Detailed Changes for destill-cli

### File: `src/cmd/destill-cli/main.go`

```diff
 package main

 import (
+	"context"
 	"fmt"
 	"os"
 	"sort"
+	"time"

 	"github.com/spf13/cobra"

+	"destill-agent/src/broker"
+	"destill-agent/src/cmd/analysis"
+	"destill-agent/src/cmd/ingestion"
 	"destill-agent/src/config"
 	"destill-agent/src/contracts"
-	"destill-agent/src/pipeline"
+	"destill-agent/src/logger"
+	"destill-agent/src/pipeline"  // Only for AgenticPipeline now
 	"destill-agent/src/store"
 	"destill-agent/src/tui"
 )

 var (
 	appConfig *config.Config
-	mode      pipeline.Mode
+	log       logger.Logger
 )

+func init() {
+	log = logger.NewConsoleLogger()
+}

 var rootCmd = &cobra.Command{
 	Use:   "destill",
 	Short: "Destill - A log triage tool for CI/CD pipelines",
 	Long: `Destill is an agent-based log triage tool that helps analyze and
 categorize CI/CD build failures.

 It supports two modes:
-- Legacy Mode: In-memory broker, streaming TUI (default)
+- Local Mode: In-memory broker, streaming TUI (default)
 - Agentic Mode: Redpanda + Postgres, distributed processing

 Mode is auto-detected based on REDPANDA_BROKERS environment variable.`,
 	PersistentPreRun: func(cmd *cobra.Command, args []string) {
 		var err error
 		appConfig, err = config.LoadFromEnv()
 		if err != nil {
 			fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
 			os.Exit(1)
 		}
-
-		// Detect mode
-		pipelineCfg := &pipeline.Config{
-			RedpandaBrokers: appConfig.RedpandaBrokers,
-			PostgresDSN:     appConfig.PostgresDSN,
-			BuildkiteToken:  appConfig.BuildkiteAPIToken,
-		}
-		mode = pipeline.DetectMode(pipelineCfg)
 	},
 }

 var runCmd = &cobra.Command{
 	Use:   "run [build-url]",
 	Short: "Analyze a Buildkite build",
 	Long: `Submit a Buildkite build for analysis.

-Legacy Mode (default): Runs complete pipeline in-process with streaming TUI
+Local Mode (default): Runs complete pipeline in-process with streaming TUI
 Agentic Mode: Submits request to Redpanda and returns request ID

 Set REDPANDA_BROKERS to enable agentic mode.`,
 	Args: cobra.ExactArgs(1),
 	Run: func(cmd *cobra.Command, args []string) {
 		buildURL := args[0]
 		ctx := context.Background()

-		pipelineCfg := &pipeline.Config{
-			RedpandaBrokers: appConfig.RedpandaBrokers,
-			PostgresDSN:     appConfig.PostgresDSN,
-			BuildkiteToken:  appConfig.BuildkiteAPIToken,
-		}
-
-		switch mode {
-		case pipeline.LegacyMode:
-			runLegacyMode(ctx, pipelineCfg, buildURL)
-		case pipeline.AgenticMode:
-			runAgenticMode(ctx, pipelineCfg, buildURL)
+		// Detect mode based on REDPANDA_BROKERS
+		if len(appConfig.RedpandaBrokers) > 0 {
+			runAgenticMode(ctx, buildURL)
+		} else {
+			runLocalMode(ctx, buildURL)
 		}
 	},
 }

-func runLegacyMode(ctx context.Context, cfg *pipeline.Config, buildURL string) {
-	fmt.Println("🔧 Running in Legacy Mode (in-memory)")
+func runLocalMode(ctx context.Context, buildURL string) {
+	fmt.Println("🔧 Running in Local Mode (in-memory)")
 	fmt.Println("💡 Tip: Set REDPANDA_BROKERS for distributed agentic mode")
 	fmt.Println()

-	// Create legacy pipeline
-	p, err := pipeline.NewLegacyPipeline(cfg)
-	if err != nil {
-		fmt.Fprintf(os.Stderr, "Failed to create pipeline: %v\n", err)
-		os.Exit(1)
-	}
-	defer p.Close()
+	// Start local pipeline with embedded agents
+	msgBroker, cleanup := startLocalPipeline(ctx, appConfig, log)
+	defer cleanup()
+
+	// Give agents time to start
+	time.Sleep(100 * time.Millisecond)
+
+	// Generate request ID
+	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
+
+	// Subscribe to output before publishing request
+	outputChan, err := msgBroker.Subscribe("ci_failures_ranked")
+	if err != nil {
+		fmt.Fprintf(os.Stderr, "Failed to subscribe to results: %v\n", err)
+		os.Exit(1)
+	}
+
+	// Publish analysis request
+	request := struct {
+		RequestID string `json:"request_id"`
+		BuildURL  string `json:"build_url"`
+	}{
+		RequestID: requestID,
+		BuildURL:  buildURL,
+	}
+
+	requestData, _ := json.Marshal(request)
+	if err := msgBroker.Publish("destill_requests", requestData); err != nil {
+		fmt.Fprintf(os.Stderr, "Failed to publish request: %v\n", err)
+		os.Exit(1)
+	}

-	// Note: Full legacy integration will be in the existing CLI
-	// For now, just show the mode
-	fmt.Println("⚠️  Legacy mode TUI integration not yet wired up")
-	fmt.Println("   Use the existing 'destill build' command from src/cmd/cli")
+	// Launch TUI to stream results
+	if err := tui.StartStreaming(outputChan); err != nil {
+		fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
+		os.Exit(1)
+	}
 }

-func runAgenticMode(ctx context.Context, cfg *pipeline.Config, buildURL string) {
+func runAgenticMode(ctx context.Context, buildURL string) {
 	fmt.Println("🚀 Running in Agentic Mode (distributed)")
 	fmt.Println()

+	cfg := &pipeline.Config{
+		RedpandaBrokers: appConfig.RedpandaBrokers,
+		PostgresDSN:     appConfig.PostgresDSN,
+		BuildkiteToken:  appConfig.BuildkiteAPIToken,
+	}
+
 	// Create agentic pipeline
 	p, err := pipeline.NewAgenticPipeline(cfg)
 	if err != nil {
 		fmt.Fprintf(os.Stderr, "Failed to create pipeline: %v\n", err)
 		os.Exit(1)
 	}
 	defer p.Close()

 	// Submit request
 	requestID, err := p.Submit(ctx, buildURL)
 	if err != nil {
 		fmt.Fprintf(os.Stderr, "Failed to submit request: %v\n", err)
 		os.Exit(1)
 	}

 	fmt.Printf("✅ Submitted analysis request: %s\n", requestID)
 	fmt.Printf("   Build URL: %s\n", buildURL)
 	fmt.Println()
 	fmt.Println("📊 The ingest and analyze agents will process this build.")
 	fmt.Println("   Findings will be stored in Postgres.")
 	fmt.Println()
 	fmt.Printf("View results: destill view %s\n", requestID)
 	fmt.Printf("Check status:  destill status %s\n", requestID)
 }

+// startLocalPipeline starts agents as goroutines in the same process
+func startLocalPipeline(ctx context.Context, cfg *config.Config, log logger.Logger) (contracts.MessageBroker, func()) {
+	// Create in-memory broker
+	inMemoryBroker := broker.NewInMemoryBroker()
+	msgBroker := broker.NewLegacyAdapter(inMemoryBroker)
+
+	// Start ingestion agent
+	ingestionAgent := ingestion.NewAgent(msgBroker, cfg.BuildkiteAPIToken, log)
+	go func() {
+		if err := ingestionAgent.Run(); err != nil {
+			log.Error("[Pipeline] Ingestion agent error: %v", err)
+		}
+	}()
+
+	// Start analysis agent
+	analysisAgent := analysis.NewAgent(msgBroker, log)
+	go func() {
+		if err := analysisAgent.Run(); err != nil {
+			log.Error("[Pipeline] Analysis agent error: %v", err)
+		}
+	}()
+
+	// Return broker and cleanup function
+	cleanup := func() {
+		msgBroker.Close()
+	}
+
+	return msgBroker, cleanup
+}
```

## Benefits of This Refactoring

1. **Simplified Architecture**
   - Single unified CLI instead of two separate ones
   - Remove incomplete pipeline abstraction
   - Clear separation: local vs distributed

2. **Preserved Functionality**
   - Local single-process mode works out-of-the-box
   - Distributed mode for production use cases
   - All existing features maintained

3. **Better Developer Experience**
   - No configuration needed for local development
   - Just run `destill run <url>` and get instant feedback
   - Easy to switch to distributed mode (set env var)

4. **Cleaner Codebase**
   - Remove ~200 lines of dead code (legacy.go, redundant pipeline.go)
   - Remove duplicate CLI implementation
   - Easier to maintain going forward

## Migration Path for Users

### Before (Current)
```bash
# Local mode
bin/destill-legacy build <url>

# Distributed mode
export REDPANDA_BROKERS=localhost:19092
export POSTGRES_DSN=postgres://...
bin/destill run <url>
```

### After (Proposed)
```bash
# Local mode (default)
bin/destill run <url>

# Distributed mode (set env vars)
export REDPANDA_BROKERS=localhost:19092
export POSTGRES_DSN=postgres://...
bin/destill run <url>
```

**No breaking changes** - just consolidation and cleanup.

## Testing Strategy

1. **Unit Tests**: All existing tests should continue to pass
2. **Integration Test - Local Mode**:
   ```bash
   make build
   ./bin/destill run https://buildkite.com/example/build/123
   # Should show streaming TUI with results
   ```

3. **Integration Test - Distributed Mode**:
   ```bash
   docker-compose up -d  # Start Redpanda + Postgres
   export REDPANDA_BROKERS=localhost:19092
   export POSTGRES_DSN=postgres://destill:destill@localhost:5432/destill

   # Start agents
   ./bin/destill-ingest &
   ./bin/destill-analyze &

   # Submit request
   ./bin/destill run https://buildkite.com/example/build/123
   # Should return request ID

   # View results
   ./bin/destill view <request-id>
   ```

## Timeline Estimate

- **Phase 1** (Wire up local mode): 2-3 hours
- **Phase 2** (Remove LegacyPipeline): 30 minutes
- **Phase 3** (Remove redundant CLI): 30 minutes
- **Phase 4** (Update Makefile): 15 minutes
- **Phase 5** (Update docs): 1 hour
- **Testing**: 1-2 hours

**Total: ~5-7 hours** of focused work

## Conclusion

The refactoring is straightforward because:
1. The working local mode already exists in `src/cmd/cli/main.go`
2. The `LegacyPipeline` is just an unused stub
3. We're consolidating, not rewriting

The result is a cleaner, more maintainable codebase that preserves all functionality while removing technical debt from the phased migration.
