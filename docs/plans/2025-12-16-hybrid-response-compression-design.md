# Hybrid Response with Log Compression

## Problem

The current MCP tool returns a lightweight manifest requiring multiple drill-down calls to see full findings. Each drill-down requires user approval, creating a painful UX. Additionally, Claude can't effectively diagnose failures without seeing the full tier 1 findings.

## Solution

Return all tier 1 findings fully expanded inline (they're the likely root causes), while summarizing tier 2-3 for optional drill-down. Apply semantic log compression to reduce token usage without losing diagnostic value.

## Design

### Response structure

```json
{
  "request_id": "req-20251216T183315-ee030a59",
  "build": {
    "url": "https://buildkite.com/org/repo/builds/123",
    "status": "failed",
    "failed_jobs": ["test-unit", "test-integration"],
    "passed_jobs_count": 24,
    "timestamp": "2025-12-16T18:33:37Z"
  },
  "tier_1_findings": [
    {
      "id": "abc123...",
      "message": "KafkaException: Transaction timeout exceeded",
      "severity": "FATAL",
      "confidence": 0.96,
      "job": "test-unit",
      "job_state": "failed",
      "recurrence": 1,
      "also_in_passing_jobs": false,
      "pre_context": ["...", "..."],
      "post_context": ["...", "..."]
    }
  ],
  "other_findings": [
    {
      "id": "def456...",
      "tier": 2,
      "message": "Connection refused to 192.168.1.1:3306...",
      "severity": "ERROR",
      "confidence": 0.85,
      "job": "test-integration"
    }
  ]
}
```

### Log compression

Apply these transformations to `message`, `pre_context`, and `post_context` fields for tier 1 findings:

#### Timestamp stripping

Remove leading timestamps that add no diagnostic value.

**Pattern:** `^\d{4}-\d{2}-\d{2}[T ][\d:.,Z+-]+\s*`

**Before:** `2024-05-21T10:00:05.123Z [ERROR] Connection failed`
**After:** `[ERROR] Connection failed`

#### Common prefix removal

Find the longest common prefix across a context block and replace with `...`.

**Before:**
```
[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Starting container
[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Pulling image
[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Container failed
```

**After:**
```
... Starting container
... Pulling image
... Container failed
```

#### Path compression

Compress long file paths to show only the relevant suffix.

**Before:** `/var/lib/jenkins/workspace/pipeline-123/src/test/java/com/app/AuthTest.java:45`
**After:** `.../AuthTest.java:45`

#### UUID and hash masking

Replace inline hashes and UUIDs with placeholders when they appear mid-line.

**Pattern:** `\b[a-f0-9]{12,}\b` (12+ hex characters)

**Before:** `Container abc123def456 failed to start`
**After:** `Container <HASH> failed to start`

#### Whitespace normalization

Collapse multiple spaces and normalize indentation.

### Implementation

#### New file: `src/mcp/compress.go`

```go
package mcp

// CompressContextLines applies all compression techniques to context lines.
func CompressContextLines(lines []string) []string

// CompressMessage applies compression to a single message.
func CompressMessage(msg string) string

// stripTimestamps removes leading timestamps from a line.
func stripTimestamps(line string) string

// findCommonPrefix finds the longest common prefix across lines.
func findCommonPrefix(lines []string) string

// compressPath shortens file paths to their suffix.
func compressPath(line string) string

// maskHashes replaces long hex strings with <HASH>.
func maskHashes(line string) string

// normalizeWhitespace collapses multiple spaces.
func normalizeWhitespace(line string) string
```

#### Modified: `src/mcp/tiering.go`

Update `ToManifest()` to:

1. Return full `Finding` objects for tier 1 (not `FindingSummary`)
2. Apply compression to tier 1 findings' context and messages
3. Return `FindingSummary` for tier 2-3 only

#### Modified: `src/mcp/types.go`

Update `ManifestResponse`:

```go
type ManifestResponse struct {
    RequestID      string           `json:"request_id"`
    Build          BuildInfo        `json:"build"`
    Tier1Findings  []Finding        `json:"tier_1_findings"`
    OtherFindings  []FindingSummary `json:"other_findings"`
}
```

### Token budget estimate

| Component | Before compression | After compression |
|-----------|-------------------|-------------------|
| Context line | ~25 tokens | ~8 tokens |
| 15 tier 1 findings (10 context lines each) | ~3750 tokens | ~1200 tokens |
| Build info + metadata | ~200 tokens | ~200 tokens |
| **Total tier 1 response** | ~4000 tokens | ~1500 tokens |

### Drill-down behavior

The `get_finding_details` tool remains available for tier 2-3 findings. It returns uncompressed data since users explicitly requested details.

## Testing

1. Unit tests for each compression function in `compress_test.go`
2. Integration test: analyze a real build, verify tier 1 findings are fully expanded
3. Verify compression ratio on sample CI logs (target: 50%+ reduction)

## Migration

This is a breaking change to the response format. Update:

1. MCP tool description to reflect new response shape
2. Any existing consumers of the old `findings` array format
