# Testing Destill in Agentic Mode - Manual Test Guide

This guide walks through testing the complete agentic architecture end-to-end.

## Prerequisites

- Docker Desktop running
- Go 1.24.10 or later
- Buildkite API token
- A failed Buildkite build URL to analyze

## Step 1: Build the Binaries

```bash
cd /Users/tysonjh/dev/destill-agent
make build-agentic
```

This creates:
- `bin/destill` - Main CLI
- `bin/destill-ingest` - Ingest agent
- `bin/destill-analyze` - Analyze agent

Verify:
```bash
ls -lh bin/
```

## Step 2: Start the Infrastructure

```bash
cd docker
docker-compose up -d
```

Verify all services are running:
```bash
docker-compose ps
```

You should see:
- `destill-redpanda` (healthy)
- `destill-postgres` (healthy)
- `destill-connect` (running)
- `destill-console` (running)

### Check Redpanda Console

Open http://localhost:8080 in your browser. You should see the Redpanda Console UI.

### Check Postgres

```bash
docker exec -it destill-postgres psql -U destill -d destill
```

Verify tables exist:
```sql
\dt
```

Should show: `findings`, `requests`

Exit:
```
\q
```

## Step 3: Create Topics

Redpanda topics need to be created manually:

```bash
# Create raw logs topic (ephemeral, 1 hour retention)
docker exec -it destill-redpanda rpk topic create destill.logs.raw \
  --partitions 3 \
  --replicas 1 \
  --config retention.ms=3600000

# Create findings topic (7 days retention)
docker exec -it destill-redpanda rpk topic create destill.analysis.findings \
  --partitions 3 \
  --replicas 1 \
  --config retention.ms=604800000

# Create requests topic
docker exec -it destill-redpanda rpk topic create destill.requests \
  --partitions 1 \
  --replicas 1
```

Verify topics:
```bash
docker exec -it destill-redpanda rpk topic list
```

## Step 4: Set Environment Variables

```bash
# Required
export BUILDKITE_API_TOKEN="your-token-here"

# Agentic mode configuration
export REDPANDA_BROKERS="localhost:19092"
export POSTGRES_DSN="postgres://destill:destill@localhost:5432/destill?sslmode=disable"
```

**Important**: Keep these environment variables set for all terminal windows in the following steps.

## Step 5: Start the Ingest Agent

In **Terminal 1**:

```bash
cd /Users/tysonjh/dev/destill-agent
./bin/destill-ingest
```

You should see:
```
[INFO] Starting Destill Ingest Agent
[INFO] Redpanda brokers: [localhost:19092]
[INFO] Ingest agent started, waiting for requests...
[INFO] [IngestAgent] Listening for requests on 'destill.requests' topic...
```

Leave this running.

## Step 6: Start the Analyze Agent

In **Terminal 2**:

```bash
cd /Users/tysonjh/dev/destill-agent
./bin/destill-analyze
```

You should see:
```
[INFO] Starting Destill Analyze Agent
[INFO] Redpanda brokers: [localhost:19092]
[INFO] Analyze agent started, processing log chunks...
[INFO] [AnalyzeAgent] Listening for log chunks on 'destill.logs.raw' topic...
```

Leave this running.

## Step 7: Submit a Build for Analysis

In **Terminal 3**:

```bash
cd /Users/tysonjh/dev/destill-agent
./bin/destill run "https://buildkite.com/your-org/your-pipeline/builds/1234"
```

Replace with an actual failed build URL.

You should see:
```
🚀 Running in Agentic Mode (distributed)

✅ Submitted analysis request: req-1733769623456789
   Build URL: https://buildkite.com/your-org/your-pipeline/builds/1234

📊 The ingest and analyze agents will process this build.
   Findings will be stored in Postgres.

View results: destill view req-1733769623456789
Check status:  destill status req-1733769623456789
```

**Copy the request ID** - you'll need it for the next steps.

## Step 8: Monitor the Processing

### In Terminal 1 (Ingest Agent)

You should see activity:
```
[INFO] [IngestAgent] Processing request req-1733769623456789
[INFO] [IngestAgent] Build URL: https://buildkite.com/...
[INFO] [IngestAgent] Fetching build metadata for org-pipeline-1234
[INFO] [IngestAgent] Found 3 jobs in build (state: failed)
[INFO] [IngestAgent] Fetching logs for job: test (id: abc-123, state: failed)
[INFO] [IngestAgent] Split job 'test' into 5 chunks
[INFO] [IngestAgent] Published Chunk 1/5: lines 1-523 (512000 bytes)
...
```

### In Terminal 2 (Analyze Agent)

You should see activity:
```
[INFO] [AnalyzeAgent] Processing chunk 1/5 for job 'test'
[INFO] [AnalyzeAgent] Found 3 issues in chunk 1/5 of job 'test'
[DEBUG] [AnalyzeAgent] Published finding: ERROR (confidence: 0.85)
...
```

### In Redpanda Console

Visit http://localhost:8080 and navigate to:
- **Topics > destill.logs.raw**: Should see log chunks
- **Topics > destill.analysis.findings**: Should see findings
- **Topics > destill.requests**: Should see your request

## Step 9: Check Request Status

```bash
./bin/destill status req-1733769623456789
```

Output:
```
Request ID:       req-1733769623456789
Build URL:        https://buildkite.com/your-org/your-pipeline/builds/1234
Status:           processing
Chunks Total:     0
Chunks Processed: 0
Findings:         12
```

**Note**: The status tracking is basic in this PoC. The important part is the findings count.

## Step 10: View Results in TUI

```bash
./bin/destill view req-1733769623456789
```

You should see:
```
📊 Found 12 findings for request: req-1733769623456789
Launching TUI...
```

The TUI will launch showing all findings:
- Sorted by confidence score (highest first)
- Navigate with arrow keys or j/k
- Press Enter to see full details
- Press q to exit

## Step 11: Verify Data in Postgres

```bash
docker exec -it destill-postgres psql -U destill -d destill
```

Query findings:
```sql
-- Count findings by severity
SELECT severity, COUNT(*) 
FROM findings 
WHERE request_id = 'req-1733769623456789'
GROUP BY severity
ORDER BY severity;

-- Top 5 findings by confidence
SELECT severity, confidence_score, LEFT(raw_message, 80) as message
FROM findings
WHERE request_id = 'req-1733769623456789'
ORDER BY confidence_score DESC
LIMIT 5;

-- Check request record
SELECT * FROM requests WHERE request_id = 'req-1733769623456789';
```

Exit:
```
\q
```

## Step 12: Test Multiple Builds

Submit another build:

```bash
./bin/destill run "https://buildkite.com/your-org/your-pipeline/builds/5678"
```

The agents will process it independently. You can:
- View the new request with its request ID
- View the previous request - data is persisted
- Both analyses happen concurrently

## Troubleshooting

### Agents not receiving messages

Check topic subscriptions:
```bash
docker exec -it destill-redpanda rpk group list
```

Should show:
- `destill-ingest`
- `destill-analyze`
- `destill-postgres-sink`

### No findings in Postgres

1. Check Redpanda Connect logs:
```bash
docker logs destill-connect
```

2. Verify findings topic has messages:
```bash
docker exec -it destill-redpanda rpk topic consume destill.analysis.findings --num 5
```

### Postgres connection issues

Verify DSN:
```bash
docker exec -it destill-postgres psql -U destill -d destill -c "SELECT version();"
```

### Redpanda connection issues

Check Redpanda health:
```bash
docker exec -it destill-redpanda rpk cluster health
```

## Performance Testing

Test with a large build:

```bash
# Submit a build with 50+ jobs
./bin/destill run "https://buildkite.com/your-org/large-pipeline/builds/999"

# Monitor throughput in Redpanda Console
# - Check messages/sec in topics
# - Check consumer lag in groups

# Scale analyze agents (in new terminals)
./bin/destill-analyze  # Terminal 4
./bin/destill-analyze  # Terminal 5
```

Multiple analyze agents will form a consumer group and split the work.

## Cleanup

### Stop agents
Press Ctrl+C in each terminal running an agent.

### Stop infrastructure
```bash
cd docker
docker-compose down
```

### Clean slate (removes all data)
```bash
cd docker
docker-compose down -v
```

## Verification Checklist

- [ ] Infrastructure starts successfully
- [ ] Topics are created
- [ ] Ingest agent starts and listens
- [ ] Analyze agent starts and listens
- [ ] Build submission returns request ID
- [ ] Ingest agent fetches and chunks logs
- [ ] Analyze agent processes chunks and publishes findings
- [ ] Findings appear in Postgres
- [ ] Status command shows progress
- [ ] View command displays findings in TUI
- [ ] Multiple requests can be processed
- [ ] Data persists after agents restart
- [ ] Redpanda Console shows message flow
- [ ] Multiple analyze agents can run concurrently

## Expected Output Summary

**Successful End-to-End Test:**
1. Infrastructure: 4 healthy containers
2. Topics: 3 topics created
3. Agents: 2 agents running and logging activity
4. Request: Submission returns request ID
5. Processing: Logs visible in both agent terminals
6. Storage: Findings in Postgres (12+ entries)
7. TUI: Interactive display of findings
8. Console: Message flow visible in Redpanda Console

If all checkpoints pass, the agentic architecture is working correctly! 🎉

## Next Steps

Once manual testing is successful:
- Consider removing legacy code path
- Add integration tests
- Document production deployment
- Add monitoring/metrics
- Optimize chunk sizes for your workloads

