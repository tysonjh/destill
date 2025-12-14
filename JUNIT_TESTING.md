# Testing JUnit XML Integration

This guide explains how to test the JUnit XML parsing feature in Destill.

## Overview

Destill now automatically detects and parses JUnit XML artifacts from Buildkite builds, creating high-confidence (1.0) findings for test failures.

## Quick Test

### 1. Create a Sample JUnit XML File

```bash
cat > /tmp/junit-sample.xml << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="MyTestSuite" tests="3" failures="2" errors="0" skipped="0" time="1.234">
  <testcase name="testSuccess" classname="com.example.MyTest" time="0.123"/>
  <testcase name="testFailure" classname="com.example.MyTest" time="0.500">
    <failure message="expected true but was false" type="AssertionError">
at com.example.MyTest.testFailure(MyTest.java:42)
at org.junit.runners.ParentRunner.run(ParentRunner.java:238)
    </failure>
  </testcase>
  <testcase name="testError" classname="com.example.MyTest" time="0.611">
    <error message="NullPointerException: foo was null" type="NullPointerException">
at com.example.MyTest.testError(MyTest.java:55)
at org.junit.runners.ParentRunner.run(ParentRunner.java:238)
    </error>
  </testcase>
</testsuite>
EOF
```

### 2. Test the Parser Directly

```bash
cd /Users/tysonjh/dev/destill-agent

# Run JUnit parser tests
go test ./src/junit/... -v

# Expected output: All 8 tests pass
```

### 3. Test with Real Buildkite Build

**Prerequisites**:
- A Buildkite build that uploads JUnit XML artifacts
- The artifacts must match the pattern `junit*.xml`

**Example Buildkite Pipeline**:

```yaml
steps:
  - label: "Run Tests"
    command: |
      # Run your tests (example with Go)
      go test ./... -v | go-junit-report > junit.xml
    artifact_paths: "junit.xml"
```

**Run Destill**:

```bash
# Start infrastructure
cd docker && docker compose up -d

# Create topics
docker exec -it destill-redpanda rpk topic create destill.logs.raw --partitions 3
docker exec -it destill-redpanda rpk topic create destill.analysis.findings --partitions 3
docker exec -it destill-redpanda rpk topic create destill.requests --partitions 1

# Set environment
export BUILDKITE_API_TOKEN="your-token"
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"

# Start agents
./bin/destill-ingest    # Terminal 1
./bin/destill-analyze   # Terminal 2

# Submit a build with JUnit artifacts
./bin/destill submit "https://buildkite.com/org/pipeline/builds/123"

# Check ingest agent logs for:
# [IngestAgent] Found X test failures in junit.xml
# [IngestAgent] Published JUnit finding: com.example.MyTest::testFoo
```

### 4. Verify Findings in Postgres

```bash
docker exec -it destill-postgres psql -U destill -d destill -c "
SELECT 
  source,
  severity,
  confidence_score,
  LEFT(raw_message, 80) as message,
  metadata->>'source_type' as type,
  metadata->>'test_name' as test
FROM findings
WHERE metadata->>'source_type' = 'junit'
ORDER BY confidence_score DESC
LIMIT 10;
"
```

**Expected Output**:
```
source                  | severity | confidence_score | message                                          | type  | test
------------------------+----------+------------------+--------------------------------------------------+-------+-----------
junit:junit.xml         | error    | 1.0              | [failure] com.example.MyTest.testFailure: exp... | junit | testFailure
junit:junit.xml         | error    | 1.0              | [error] com.example.MyTest.testError: NullPoi... | junit | testError
```

### 5. View in TUI (Future)

Once the TUI is updated to query Postgres:

```bash
./bin/destill view <request-id>
```

JUnit findings will appear at the top (confidence: 1.0).

## What Gets Detected

### Artifact Path Patterns

The ingest agent looks for artifacts matching:
- `junit.xml`
- `junit-*.xml`
- `test-results/junit.xml`
- Any path containing `junit` and ending in `.xml` (case-insensitive)

### JUnit XML Elements

Parsed elements:
- `<testsuite>` - Test suite metadata
- `<testcase>` - Individual test cases
- `<failure>` - Test assertion failures
- `<error>` - Test errors (exceptions)

Ignored elements:
- `<skipped>` - Skipped tests (not failures)
- Passing tests (no `<failure>` or `<error>`)

## Finding Structure

JUnit findings have these characteristics:

**Identity**:
- `id`: `{request_id}-{job_id}-{hash}`
- `message_hash`: SHA256 of `class::test::message`

**Source**:
- `source`: `junit:{artifact_path}`
- `job_name`: Buildkite job name

**Content**:
- `severity`: Always `"error"`
- `confidence_score`: Always `1.0`
- `raw_message`: `[failure] com.example.Test.testFoo: message`
- `normalized_message`: `com.example.Test::testFoo`

**Context**:
- `pre_context`: Empty (no pre-context for JUnit)
- `post_context`: Stack trace (up to 50 lines)
- `context_note`: `"JUnit test failure (structured data)"`

**Metadata**:
- `source_type`: `"junit"`
- `artifact_path`: Path to JUnit XML file
- `test_name`: Test method name
- `class_name`: Test class name
- `suite_name`: Test suite name
- `failure_type`: `"failure"` or `"error"`
- `duration_sec`: Test duration in seconds

## Debugging

### Enable Debug Logging

The ingest agent logs JUnit processing:

```
[IngestAgent] Found 3 artifacts for job test-suite
[IngestAgent] Processing JUnit artifact: junit.xml
[IngestAgent] Found 2 test failures in junit.xml
[IngestAgent] Published JUnit finding: com.example.MyTest::testFailure
[IngestAgent] Published JUnit finding: com.example.MyTest::testError
[IngestAgent] Processed JUnit artifacts for job 'test-suite': 2 test failures
```

### Check Redpanda Topics

```bash
# Check if JUnit findings are published
docker exec -it destill-redpanda rpk topic consume destill.analysis.findings --num 5

# Look for messages with:
# - "source": "junit:..."
# - "confidence_score": 1.0
# - "metadata": {"source_type": "junit", ...}
```

### Common Issues

**No JUnit findings detected**:
- Check artifact path matches `junit*.xml` pattern
- Verify artifact was uploaded to Buildkite
- Check ingest agent logs for artifact detection

**Parse errors**:
- Ensure JUnit XML is valid (use `xmllint` to validate)
- Check ingest agent logs for parse errors
- Verify XML follows JUnit schema

**Findings not in Postgres**:
- Check Redpanda Connect logs: `docker logs destill-connect`
- Verify findings topic has messages
- Check Postgres connection

## Performance

**JUnit Processing Impact**:
- Artifact download: ~100ms per artifact
- XML parsing: ~10ms per file
- Finding creation: ~1ms per test failure

**Typical Build**:
- 5 jobs with JUnit artifacts
- 10 test failures total
- Additional processing time: ~1 second

## Next Steps

1. **Test with your builds**: Add `artifact_paths: "junit*.xml"` to your pipeline
2. **Verify findings**: Check Postgres for JUnit findings with 1.0 confidence
3. **Compare**: JUnit findings vs log-based findings (accuracy comparison)
4. **Monitor**: Check ingest agent logs for JUnit processing stats

## Example Buildkite Pipelines

### Go with go-junit-report

```yaml
steps:
  - label: "Go Tests"
    command: |
      go test ./... -v 2>&1 | go-junit-report > junit.xml
    artifact_paths: "junit.xml"
```

### Python with pytest

```yaml
steps:
  - label: "Python Tests"
    command: |
      pytest --junitxml=junit.xml
    artifact_paths: "junit.xml"
```

### Java with Maven

```yaml
steps:
  - label: "Maven Tests"
    command: |
      mvn test
    artifact_paths: "target/surefire-reports/TEST-*.xml"
```

### JavaScript with Jest

```yaml
steps:
  - label: "Jest Tests"
    command: |
      npm test -- --ci --reporters=default --reporters=jest-junit
    artifact_paths: "junit.xml"
```

---

For questions or issues with JUnit integration, check the ingest agent logs first, then verify artifact paths and XML validity.

