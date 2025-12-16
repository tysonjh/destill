# Hybrid Response with Log Compression Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Return all tier 1 findings fully expanded with compressed context, tier 2-3 as summaries only.

**Architecture:** Add compression layer in `src/mcp/compress.go`, modify `ToManifest()` to return full tier 1 findings with compression applied, update response types.

**Tech Stack:** Go, regex for pattern matching

---

### Task 1: Add Timestamp Stripping

**Files:**
- Create: `src/mcp/compress.go`
- Create: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

In `src/mcp/compress_test.go`:

```go
package mcp

import "testing"

func TestStripTimestamps(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "ISO timestamp with T separator",
			input:    "2024-05-21T10:00:05.123Z [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "ISO timestamp with space separator",
			input:    "2024-05-21 10:00:05,123 [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "timestamp with timezone offset",
			input:    "2024-05-21T10:00:05+00:00 [ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "no timestamp",
			input:    "[ERROR] Connection failed",
			expected: "[ERROR] Connection failed",
		},
		{
			name:     "timestamp mid-line preserved",
			input:    "Error at 2024-05-21T10:00:05Z in module",
			expected: "Error at 2024-05-21T10:00:05Z in module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripTimestamps(tt.input)
			if result != tt.expected {
				t.Errorf("stripTimestamps(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run TestStripTimestamps`
Expected: FAIL - undefined: stripTimestamps

**Step 3: Write minimal implementation**

In `src/mcp/compress.go`:

```go
package mcp

import "regexp"

// timestampPattern matches leading timestamps in various formats:
// - 2024-05-21T10:00:05.123Z
// - 2024-05-21 10:00:05,123
// - 2024-05-21T10:00:05+00:00
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}[.,]?\d*[Z]?([+-]\d{2}:?\d{2})?\s*`)

// stripTimestamps removes leading timestamps from a line.
func stripTimestamps(line string) string {
	return timestampPattern.ReplaceAllString(line, "")
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run TestStripTimestamps`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add timestamp stripping for log compression"
```

---

### Task 2: Add Hash Masking

**Files:**
- Modify: `src/mcp/compress.go`
- Modify: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

Append to `src/mcp/compress_test.go`:

```go
func TestMaskHashes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "container ID",
			input:    "Container abc123def456789 failed to start",
			expected: "Container <HASH> failed to start",
		},
		{
			name:     "git SHA",
			input:    "Commit 1a2b3c4d5e6f7890abcdef1234567890abcdef12 broke tests",
			expected: "Commit <HASH> broke tests",
		},
		{
			name:     "multiple hashes",
			input:    "Image abc123def456789:latest on host def456abc789012",
			expected: "Image <HASH>:latest on host <HASH>",
		},
		{
			name:     "short hex preserved",
			input:    "Error code 0x1234 returned",
			expected: "Error code 0x1234 returned",
		},
		{
			name:     "no hashes",
			input:    "Connection failed to server",
			expected: "Connection failed to server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskHashes(tt.input)
			if result != tt.expected {
				t.Errorf("maskHashes(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run TestMaskHashes`
Expected: FAIL - undefined: maskHashes

**Step 3: Write minimal implementation**

Add to `src/mcp/compress.go`:

```go
// hashPattern matches hex strings of 12+ characters (container IDs, git SHAs, etc.)
var hashPattern = regexp.MustCompile(`\b[a-f0-9]{12,}\b`)

// maskHashes replaces long hex strings with <HASH>.
func maskHashes(line string) string {
	return hashPattern.ReplaceAllString(line, "<HASH>")
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run TestMaskHashes`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add hash masking for log compression"
```

---

### Task 3: Add Path Compression

**Files:**
- Modify: `src/mcp/compress.go`
- Modify: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

Append to `src/mcp/compress_test.go`:

```go
func TestCompressPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long absolute path",
			input:    "/var/lib/jenkins/workspace/pipeline-123/src/test/java/com/app/AuthTest.java:45",
			expected: ".../AuthTest.java:45",
		},
		{
			name:     "path with line reference",
			input:    "File /home/user/project/src/main/Service.go:123 - error",
			expected: "File .../Service.go:123 - error",
		},
		{
			name:     "short path preserved",
			input:    "src/main.go:10 - warning",
			expected: "src/main.go:10 - warning",
		},
		{
			name:     "no path",
			input:    "Connection refused",
			expected: "Connection refused",
		},
		{
			name:     "multiple paths",
			input:    "/a/b/c/file1.go:1 imports /d/e/f/file2.go:2",
			expected: ".../file1.go:1 imports .../file2.go:2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compressPath(tt.input)
			if result != tt.expected {
				t.Errorf("compressPath(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run TestCompressPath`
Expected: FAIL - undefined: compressPath

**Step 3: Write minimal implementation**

Add to `src/mcp/compress.go`:

```go
// longPathPattern matches absolute paths with 3+ directories.
// Captures the filename (and optional line number) at the end.
var longPathPattern = regexp.MustCompile(`/(?:[^/\s]+/){3,}([^/\s:]+(?::\d+)?)`)

// compressPath shortens long file paths to .../filename.
func compressPath(line string) string {
	return longPathPattern.ReplaceAllString(line, ".../$1")
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run TestCompressPath`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add path compression for log compression"
```

---

### Task 4: Add Common Prefix Removal

**Files:**
- Modify: `src/mcp/compress.go`
- Modify: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

Append to `src/mcp/compress_test.go`:

```go
func TestFindCommonPrefix(t *testing.T) {
	tests := []struct {
		name     string
		lines    []string
		expected string
	}{
		{
			name: "Java logger prefix",
			lines: []string{
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Starting",
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Pulling",
				"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Failed",
			},
			expected: "[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] ",
		},
		{
			name: "no common prefix",
			lines: []string{
				"Starting container",
				"Pulling image",
				"Container failed",
			},
			expected: "",
		},
		{
			name: "short common prefix ignored",
			lines: []string{
				"[INFO] Starting",
				"[INFO] Stopping",
			},
			expected: "", // Too short (< 20 chars) to be worth removing
		},
		{
			name:     "empty lines",
			lines:    []string{},
			expected: "",
		},
		{
			name:     "single line",
			lines:    []string{"[INFO] Single line"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCommonPrefix(tt.lines)
			if result != tt.expected {
				t.Errorf("findCommonPrefix() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestRemoveCommonPrefix(t *testing.T) {
	lines := []string{
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Starting container",
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Pulling image",
		"[INFO] [com.mycompany.infrastructure.runner.DockerExecutor] Container failed",
	}

	result := removeCommonPrefix(lines)

	expected := []string{
		"... Starting container",
		"... Pulling image",
		"... Container failed",
	}

	if len(result) != len(expected) {
		t.Fatalf("len = %d, expected %d", len(result), len(expected))
	}

	for i, line := range result {
		if line != expected[i] {
			t.Errorf("line[%d] = %q, expected %q", i, line, expected[i])
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run "TestFindCommonPrefix|TestRemoveCommonPrefix"`
Expected: FAIL - undefined: findCommonPrefix, removeCommonPrefix

**Step 3: Write minimal implementation**

Add to `src/mcp/compress.go`:

```go
// minPrefixLength is the minimum prefix length worth removing.
// Shorter prefixes don't save enough tokens to justify the "..." replacement.
const minPrefixLength = 20

// findCommonPrefix finds the longest common prefix across lines.
// Returns empty string if prefix is too short or lines are empty.
func findCommonPrefix(lines []string) string {
	if len(lines) < 2 {
		return ""
	}

	prefix := lines[0]
	for _, line := range lines[1:] {
		for len(prefix) > 0 && (len(line) < len(prefix) || line[:len(prefix)] != prefix) {
			prefix = prefix[:len(prefix)-1]
		}
		if len(prefix) == 0 {
			break
		}
	}

	if len(prefix) < minPrefixLength {
		return ""
	}

	return prefix
}

// removeCommonPrefix replaces common prefix with "... " across lines.
func removeCommonPrefix(lines []string) []string {
	prefix := findCommonPrefix(lines)
	if prefix == "" {
		return lines
	}

	result := make([]string, len(lines))
	for i, line := range lines {
		result[i] = "... " + line[len(prefix):]
	}
	return result
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run "TestFindCommonPrefix|TestRemoveCommonPrefix"`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add common prefix removal for log compression"
```

---

### Task 5: Add Whitespace Normalization

**Files:**
- Modify: `src/mcp/compress.go`
- Modify: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

Append to `src/mcp/compress_test.go`:

```go
func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiple spaces",
			input:    "Error    in     module",
			expected: "Error in module",
		},
		{
			name:     "tabs to spaces",
			input:    "Error\tin\tmodule",
			expected: "Error in module",
		},
		{
			name:     "leading/trailing spaces",
			input:    "   Error in module   ",
			expected: "Error in module",
		},
		{
			name:     "mixed whitespace",
			input:    "  Error  \t  in \t module  ",
			expected: "Error in module",
		},
		{
			name:     "already normalized",
			input:    "Error in module",
			expected: "Error in module",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeWhitespace(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeWhitespace(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run TestNormalizeWhitespace`
Expected: FAIL - undefined: normalizeWhitespace

**Step 3: Write minimal implementation**

Add to `src/mcp/compress.go`:

```go
import "strings"

// whitespacePattern matches multiple consecutive whitespace characters.
var whitespacePattern = regexp.MustCompile(`\s+`)

// normalizeWhitespace collapses multiple spaces/tabs and trims.
func normalizeWhitespace(line string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(line, " "))
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run TestNormalizeWhitespace`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add whitespace normalization for log compression"
```

---

### Task 6: Add CompressLine and CompressContextLines

**Files:**
- Modify: `src/mcp/compress.go`
- Modify: `src/mcp/compress_test.go`

**Step 1: Write the failing test**

Append to `src/mcp/compress_test.go`:

```go
func TestCompressLine(t *testing.T) {
	input := "2024-05-21T10:00:05.123Z /var/lib/jenkins/workspace/pipeline/src/test/AuthTest.java:45 - Container abc123def456789 failed"
	expected := ".../AuthTest.java:45 - Container <HASH> failed"

	result := CompressLine(input)
	if result != expected {
		t.Errorf("CompressLine() = %q, expected %q", result, expected)
	}
}

func TestCompressContextLines(t *testing.T) {
	lines := []string{
		"2024-05-21T10:00:01.000Z [INFO] [com.mycompany.runner.Executor] Starting test",
		"2024-05-21T10:00:02.000Z [INFO] [com.mycompany.runner.Executor] Running test",
		"2024-05-21T10:00:03.000Z [INFO] [com.mycompany.runner.Executor] Test failed",
	}

	result := CompressContextLines(lines)

	// Should remove timestamps, find common prefix, normalize whitespace
	expected := []string{
		"... Starting test",
		"... Running test",
		"... Test failed",
	}

	if len(result) != len(expected) {
		t.Fatalf("len = %d, expected %d", len(result), len(expected))
	}

	for i, line := range result {
		if line != expected[i] {
			t.Errorf("line[%d] = %q, expected %q", i, line, expected[i])
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run "TestCompressLine|TestCompressContextLines"`
Expected: FAIL - undefined: CompressLine, CompressContextLines

**Step 3: Write minimal implementation**

Add to `src/mcp/compress.go`:

```go
// CompressLine applies all single-line compression techniques.
// Order matters: timestamps first, then paths, then hashes, then whitespace.
func CompressLine(line string) string {
	line = stripTimestamps(line)
	line = compressPath(line)
	line = maskHashes(line)
	line = normalizeWhitespace(line)
	return line
}

// CompressContextLines applies compression to a slice of context lines.
// Applies per-line compression first, then removes common prefix.
func CompressContextLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	// Apply per-line compression
	compressed := make([]string, len(lines))
	for i, line := range lines {
		compressed[i] = CompressLine(line)
	}

	// Remove common prefix across the block
	return removeCommonPrefix(compressed)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run "TestCompressLine|TestCompressContextLines"`
Expected: PASS

**Step 5: Commit**

```bash
git add src/mcp/compress.go src/mcp/compress_test.go
git commit -m "feat(mcp): add CompressLine and CompressContextLines exports"
```

---

### Task 7: Update ManifestResponse Type

**Files:**
- Modify: `src/mcp/types.go`

**Step 1: Read current types.go**

Already read - see `ManifestResponse` at lines 53-59.

**Step 2: Update the type**

In `src/mcp/types.go`, replace lines 53-59:

```go
// ManifestResponse is the response from analyze_build.
// Tier 1 findings are fully expanded (they're the likely root causes).
// Tier 2-3 findings are summarized for optional drill-down.
type ManifestResponse struct {
	RequestID     string           `json:"request_id"`
	Build         BuildInfo        `json:"build"`
	Tier1Findings []Finding        `json:"tier_1_findings"`
	OtherFindings []FindingSummary `json:"other_findings"`
}
```

**Step 3: Run existing tests to check for breakage**

Run: `go test -v ./src/mcp/...`
Expected: Some tests may fail due to type change - that's expected.

**Step 4: Commit**

```bash
git add src/mcp/types.go
git commit -m "refactor(mcp): update ManifestResponse for hybrid approach"
```

---

### Task 8: Update ToManifest Function

**Files:**
- Modify: `src/mcp/tiering.go`
- Modify: `src/mcp/tiering_test.go`

**Step 1: Write the failing test**

Add to `src/mcp/tiering_test.go`:

```go
func TestToManifest_HybridResponse(t *testing.T) {
	response := TieredResponse{
		Build: BuildInfo{URL: "https://example.com/build/1", Status: "failed"},
		Tier1UniqueFailures: []Finding{
			{
				ID:          "tier1-id",
				Message:     "2024-05-21T10:00:00Z /var/lib/long/path/to/file.go:123 - Error with abc123def456789",
				Severity:    "ERROR",
				Confidence:  0.95,
				Job:         "test-job",
				PreContext:  []string{"2024-05-21T09:59:59Z [INFO] [com.company.module.Class] Pre line"},
				PostContext: []string{"2024-05-21T10:00:01Z [INFO] [com.company.module.Class] Post line"},
			},
		},
		Tier3CommonNoise: []Finding{
			{
				ID:         "tier3-id",
				Message:    "Common warning message that appears everywhere in the logs",
				Severity:   "WARNING",
				Confidence: 0.5,
				Job:        "test-job",
			},
		},
	}

	manifest := ToManifest("req-123", response)

	// Tier 1 should be fully expanded with compression
	if len(manifest.Tier1Findings) != 1 {
		t.Fatalf("Tier1Findings len = %d, expected 1", len(manifest.Tier1Findings))
	}

	tier1 := manifest.Tier1Findings[0]

	// Message should be compressed (timestamp stripped, path shortened, hash masked)
	if tier1.Message == response.Tier1UniqueFailures[0].Message {
		t.Error("Tier1 message should be compressed")
	}
	if tier1.ID != "tier1-id" {
		t.Errorf("Tier1 ID = %q, expected %q", tier1.ID, "tier1-id")
	}

	// Tier 2-3 should be summaries
	if len(manifest.OtherFindings) != 1 {
		t.Fatalf("OtherFindings len = %d, expected 1", len(manifest.OtherFindings))
	}

	other := manifest.OtherFindings[0]
	if other.Tier != 3 {
		t.Errorf("OtherFindings[0].Tier = %d, expected 3", other.Tier)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v ./src/mcp/... -run TestToManifest_HybridResponse`
Expected: FAIL - Tier1Findings field doesn't exist or wrong type

**Step 3: Update ToManifest implementation**

In `src/mcp/tiering.go`, replace `ToManifest` function (lines 211-231):

```go
// ToManifest converts a TieredResponse to a ManifestResponse.
// Tier 1 findings are fully expanded with compression applied.
// Tier 2-3 findings are converted to lightweight summaries.
func ToManifest(requestID string, response TieredResponse) ManifestResponse {
	// Compress and include full tier 1 findings
	tier1 := make([]Finding, len(response.Tier1UniqueFailures))
	for i, f := range response.Tier1UniqueFailures {
		tier1[i] = compressFinding(f)
	}

	// Convert tier 2-3 to summaries
	var other []FindingSummary
	for _, f := range response.Tier2FrequencySpikes {
		other = append(other, toSummary(f, 2))
	}
	for _, f := range response.Tier3CommonNoise {
		other = append(other, toSummary(f, 3))
	}

	return ManifestResponse{
		RequestID:     requestID,
		Build:         response.Build,
		Tier1Findings: tier1,
		OtherFindings: other,
	}
}

// compressFinding applies log compression to a Finding.
func compressFinding(f Finding) Finding {
	return Finding{
		ID:                f.ID,
		Message:           CompressLine(f.Message),
		Severity:          f.Severity,
		Confidence:        f.Confidence,
		Job:               f.Job,
		JobState:          f.JobState,
		Recurrence:        f.Recurrence,
		AlsoInPassingJobs: f.AlsoInPassingJobs,
		PreContext:        CompressContextLines(f.PreContext),
		PostContext:       CompressContextLines(f.PostContext),
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v ./src/mcp/... -run TestToManifest_HybridResponse`
Expected: PASS

**Step 5: Run all tests**

Run: `go test -v ./src/mcp/...`
Expected: PASS

**Step 6: Commit**

```bash
git add src/mcp/tiering.go src/mcp/tiering_test.go
git commit -m "feat(mcp): update ToManifest for hybrid response with compression"
```

---

### Task 9: Update MCP Tool Description

**Files:**
- Modify: `src/mcp/server.go`

**Step 1: Update tool description**

In `src/mcp/server.go`, update the `analyzeTool` description (around line 46):

```go
analyzeTool := mcp.NewTool("analyze_build",
	mcp.WithDescription("Analyze a CI/CD build and return tiered findings. Returns all tier 1 findings (unique failures) fully expanded with context - these are the likely root causes. Tier 2-3 findings are summarized; use get_finding_details to drill into them if needed."),
	mcp.WithString("url",
		mcp.Required(),
		mcp.Description("Build URL (Buildkite or GitHub Actions)"),
	),
	mcp.WithNumber("limit",
		mcp.Description("Max findings per tier (default: 15)"),
	),
)
```

**Step 2: Commit**

```bash
git add src/mcp/server.go
git commit -m "docs(mcp): update analyze_build tool description"
```

---

### Task 10: Integration Test and Rebuild

**Step 1: Run all tests**

Run: `go test -v ./src/mcp/...`
Expected: PASS

**Step 2: Build and install**

Run: `make install`
Expected: Success

**Step 3: Manual verification**

Test with MCP tool:
```
analyze_build("https://buildkite.com/redpanda/redpanda/builds/77890")
```

Expected response shape:
```json
{
  "request_id": "req-...",
  "build": { ... },
  "tier_1_findings": [
    {
      "id": "...",
      "message": "compressed message without timestamps",
      "pre_context": ["... compressed lines"],
      "post_context": ["... compressed lines"],
      ...
    }
  ],
  "other_findings": [
    { "id": "...", "tier": 3, "message": "truncated...", ... }
  ]
}
```

**Step 4: Commit any fixes**

If tests reveal issues, fix and commit.

---

## Summary

| Task | Description |
|------|-------------|
| 1 | Timestamp stripping |
| 2 | Hash masking |
| 3 | Path compression |
| 4 | Common prefix removal |
| 5 | Whitespace normalization |
| 6 | CompressLine and CompressContextLines exports |
| 7 | Update ManifestResponse type |
| 8 | Update ToManifest function |
| 9 | Update MCP tool description |
| 10 | Integration test and rebuild |
