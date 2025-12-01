// Demo program to showcase the Destill TUI with a rich, realistic dataset.
package main

import (
	"fmt"
	"os"
	"time"

	"destill-agent/src/contracts"
	"destill-agent/src/tui"
)

func main() {
	fmt.Println("Generating sample failure data...")
	cards := generateSampleData()

	fmt.Printf("Loaded %d failures across %d jobs.\n", len(cards), countUniqueJobs(cards))
	fmt.Println("Launching TUI...")
	time.Sleep(500 * time.Millisecond) // Brief pause for effect

	if err := tui.Start(cards); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

func countUniqueJobs(cards []contracts.TriageCard) int {
	jobs := make(map[string]bool)
	for _, c := range cards {
		jobs[c.JobName] = true
	}
	return len(jobs)
}

func generateSampleData() []contracts.TriageCard {
	return []contracts.TriageCard{
		// 1. Backend Integration Tests - OOM
		{
			ID:              "card-001",
			Source:          "buildkite",
			JobName:         "backend-integration-tests",
			Severity:        "FATAL",
			Message:         "java.lang.OutOfMemoryError: Java heap space",
			MessageHash:     "a7b3c9d2e1f4567890abcdef12345678",
			ConfidenceScore: 0.99,
			PreContext: `[INFO] Running test: com.acme.processor.LargeBatchTest
[INFO] Loading dataset: datasets/huge_import.csv (500MB)
[DEBUG] Memory usage: 1024MB / 2048MB
[INFO] Processing batch 1 of 50...
[WARN] GC overhead limit exceeded imminent`,
			PostContext: `at com.acme.processor.DataHandler.process(DataHandler.java:142)
at com.acme.processor.BatchRunner.run(BatchRunner.java:55)
at java.base/java.lang.Thread.run(Thread.java:833)
[ERROR] Test runner crashed unexpectedly`,
			Metadata: map[string]string{
				"recurrence_count": "15",
			},
		},

		// 2. E2E Tests - Database Connection Timeout
		{
			ID:              "card-002",
			Source:          "buildkite",
			JobName:         "e2e-tests-chrome",
			Severity:        "ERROR",
			Message:         "ConnectionTimeout: Failed to connect to postgres://db-prod:5432 after 30000ms",
			MessageHash:     "f8e7d6c5b4a3928170fedcba98765432",
			ConfidenceScore: 0.95,
			PreContext: `[TestWorker-1] Starting test: "User Checkout Flow"
[TestWorker-1] Navigating to /checkout
[TestWorker-1] Filling shipping address
[TestWorker-1] Clicking "Place Order"
[DB-Pool] Acquiring connection...`,
			PostContext: `[TestWorker-1] Screenshot saved to artifacts/failure_checkout_timeout.png
[TestWorker-1] Test failed: "User Checkout Flow"
[TestWorker-1] Retrying test (1/3)...`,
			Metadata: map[string]string{
				"recurrence_count": "8",
			},
		},

		// 3. Unit Tests - Assertion Failure
		{
			ID:              "card-003",
			Source:          "buildkite",
			JobName:         "unit-tests",
			Severity:        "ERROR",
			Message:         "AssertionError: expected status 200 OK, got 503 Service Unavailable",
			MessageHash:     "1234567890abcdef1234567890abcdef",
			ConfidenceScore: 0.92,
			PreContext: `PASS: TestUserLogin
PASS: TestUserLogout
RUN: TestUserProfile_Fetch
[MockServer] GET /api/v1/user/123
[MockServer] Simulating downstream failure`,
			PostContext: `    user_test.go:45:
        Error:      Not equal.
        Expected:   200
        Actual:     503
FAIL: TestUserProfile_Fetch (0.02s)`,
			Metadata: map[string]string{
				"recurrence_count": "3",
			},
		},

		// 4. Linting - Deprecation Warning
		{
			ID:              "card-004",
			Source:          "buildkite",
			JobName:         "lint-check",
			Severity:        "WARN",
			Message:         "DeprecationWarning: 'ioutil.ReadAll' is deprecated, use 'io.ReadAll' instead",
			MessageHash:     "abcdefabcdefabcdefabcdefabcdefab",
			ConfidenceScore: 0.75,
			PreContext: `checking src/utils/file.go... OK
checking src/utils/net.go... OK
checking src/legacy/importer.go...`,
			PostContext: `src/legacy/importer.go:15:12: data, err := ioutil.ReadAll(r)
See: https://pkg.go.dev/io#ReadAll
checking src/main.go... OK`,
			Metadata: map[string]string{
				"recurrence_count": "1",
			},
		},

		// 5. Build - Compilation Error
		{
			ID:              "card-005",
			Source:          "buildkite",
			JobName:         "build-linux-amd64",
			Severity:        "ERROR",
			Message:         "undefined: handleAuthCallback",
			MessageHash:     "9876543210fedcba9876543210fedcba",
			ConfidenceScore: 0.88,
			PreContext: `compiling package: github.com/acme/auth
src/auth/oauth.go:45:2: missing return
src/auth/oauth.go:50:15: undefined: UserConfig`,
			PostContext: `src/auth/routes.go:112:15: undefined: handleAuthCallback
note: module requires Go 1.21
make: *** [Makefile:45: build] Error 1`,
			Metadata: map[string]string{
				"recurrence_count": "2",
			},
		},

		// 6. Frontend - TypeScript Error
		{
			ID:              "card-006",
			Source:          "buildkite",
			JobName:         "frontend-build",
			Severity:        "ERROR",
			Message:         "TS2322: Type 'string | null' is not assignable to type 'string'.",
			MessageHash:     "11223344556677889900aabbccddeeff",
			ConfidenceScore: 0.85,
			PreContext: `src/components/Header.tsx(12,5): error TS2322: Type 'string | null' is not assignable to type 'string'.
  Type 'null' is not assignable to type 'string'.
src/components/UserProfile.tsx(45,10): error TS2531: Object is possibly 'null'.`,
			PostContext: `    43 |
    44 |       return <div>{user.name}</div>;
  > 45 |     return <div>{user.email}</div>;
       |                  ^^^^^
Found 2 errors.`,
			Metadata: map[string]string{
				"recurrence_count": "5",
			},
		},

		// 7. Infrastructure - Terraform Error
		{
			ID:              "card-007",
			Source:          "buildkite",
			JobName:         "infra-deploy-staging",
			Severity:        "ERROR",
			Message:         "Error: Error creating Security Group: InvalidGroup.Duplicate",
			MessageHash:     "ffeeddccbbaa99887766554433221100",
			ConfidenceScore: 0.90,
			PreContext: `aws_s3_bucket.logs: Creating...
aws_s3_bucket.logs: Creation complete after 2s [id=acme-staging-logs]
aws_security_group.web: Creating...`,
			PostContext: `
Error: Error creating Security Group: InvalidGroup.Duplicate: The security group 'web-sg' already exists for VPC 'vpc-123456'
  on modules/networking/sg.tf line 12, in resource "aws_security_group" "web":
  12: resource "aws_security_group" "web" {
`,
			Metadata: map[string]string{
				"recurrence_count": "1",
			},
		},

		// 8. Python Script - KeyError
		{
			ID:              "card-008",
			Source:          "buildkite",
			JobName:         "data-migration",
			Severity:        "ERROR",
			Message:         "KeyError: 'customer_id'",
			MessageHash:     "00112233445566778899aabbccddeeff",
			ConfidenceScore: 0.80,
			PreContext: `INFO:root:Starting migration script v2.1
INFO:root:Connected to database
INFO:root:Migrating table: orders
INFO:root:Processing batch 100-200`,
			PostContext: `Traceback (most recent call last):
  File "migrate.py", line 45, in <module>
    process_row(row)
  File "migrate.py", line 23, in process_row
    user = row['customer_id']
KeyError: 'customer_id'`,
			Metadata: map[string]string{
				"recurrence_count": "4",
			},
		},

		// 9. Long Context Log - Requires Scrolling
		{
			ID:              "card-009",
			Source:          "buildkite",
			JobName:         "job-long-context",
			Severity:        "ERROR",
			Message:         "RuntimeError: Failed to process large batch job after extensive processing pipeline",
			MessageHash:     "aabbccdd11223344556677889900eeff",
			ConfidenceScore: 0.94,
			PreContext: `[2025-11-30 10:15:23] INFO: Pipeline Stage 1/10: Data Ingestion Started
[2025-11-30 10:15:24] INFO: Loading configuration from config/pipeline.yaml
[2025-11-30 10:15:24] INFO: Validating schema for input dataset
[2025-11-30 10:15:25] INFO: Schema validation passed: 45 columns detected
[2025-11-30 10:15:25] INFO: Initiating data load from S3: s3://data-lake/raw/2025-11-30/
[2025-11-30 10:15:26] INFO: Downloaded 1.2GB in 850ms
[2025-11-30 10:15:26] INFO: Decompressing archive...
[2025-11-30 10:15:28] INFO: Extracted 5.8GB of CSV data
[2025-11-30 10:15:28] INFO: Pipeline Stage 2/10: Data Cleansing Started
[2025-11-30 10:15:29] INFO: Removing duplicate records...
[2025-11-30 10:15:32] INFO: Found and removed 12,543 duplicate entries
[2025-11-30 10:15:32] INFO: Validating data types...
[2025-11-30 10:15:35] INFO: Converting timestamp fields to UTC
[2025-11-30 10:15:37] INFO: Normalizing currency values to USD
[2025-11-30 10:15:39] INFO: Pipeline Stage 3/10: Data Enrichment Started
[2025-11-30 10:15:40] INFO: Fetching enrichment data from API: https://api.enrichment.example.com
[2025-11-30 10:15:41] INFO: API Rate limit: 1000 req/min, current: 245 req/min
[2025-11-30 10:15:43] INFO: Enriched 50,000 records with geolocation data
[2025-11-30 10:15:45] INFO: Enriched 50,000 records with demographic data
[2025-11-30 10:15:47] INFO: Pipeline Stage 4/10: Data Transformation Started
[2025-11-30 10:15:48] INFO: Applying business rules engine
[2025-11-30 10:15:50] INFO: Executing rule: customer_segment_classification
[2025-11-30 10:15:52] INFO: Executing rule: revenue_tier_assignment
[2025-11-30 10:15:54] INFO: Executing rule: churn_risk_calculation
[2025-11-30 10:15:56] INFO: Pipeline Stage 5/10: Data Aggregation Started
[2025-11-30 10:15:57] INFO: Computing daily aggregates...
[2025-11-30 10:15:59] INFO: Computing weekly aggregates...
[2025-11-30 10:16:01] INFO: Computing monthly aggregates...
[2025-11-30 10:16:03] INFO: Pipeline Stage 6/10: Quality Checks Started
[2025-11-30 10:16:04] INFO: Running data quality suite (25 checks)
[2025-11-30 10:16:05] INFO: Check 1/25: NULL value threshold - PASSED
[2025-11-30 10:16:06] INFO: Check 2/25: Data freshness - PASSED
[2025-11-30 10:16:07] INFO: Check 3/25: Referential integrity - PASSED
[2025-11-30 10:16:08] INFO: Check 4/25: Value range validation - PASSED
[2025-11-30 10:16:09] WARN: Check 5/25: Outlier detection - 127 outliers detected (within threshold)
[2025-11-30 10:16:10] INFO: Check 6/25: Distribution consistency - PASSED
[2025-11-30 10:16:11] INFO: Pipeline Stage 7/10: Feature Engineering Started
[2025-11-30 10:16:12] INFO: Generating time-based features...
[2025-11-30 10:16:14] INFO: Generating categorical embeddings...
[2025-11-30 10:16:16] INFO: Generating interaction features...
[2025-11-30 10:16:18] INFO: Feature matrix now contains 342 features
[2025-11-30 10:16:19] INFO: Pipeline Stage 8/10: Model Scoring Started
[2025-11-30 10:16:20] INFO: Loading ML model from: models/prod/v2.4.1/model.pkl
[2025-11-30 10:16:22] INFO: Model loaded successfully (size: 245MB)
[2025-11-30 10:16:23] INFO: Running inference on batch 1/10...
[2025-11-30 10:16:25] INFO: Running inference on batch 2/10...
[2025-11-30 10:16:27] INFO: Running inference on batch 3/10...
[2025-11-30 10:16:29] INFO: Running inference on batch 4/10...
[2025-11-30 10:16:31] INFO: Running inference on batch 5/10...
[2025-11-30 10:16:33] WARN: Memory usage at 85% - triggering garbage collection
[2025-11-30 10:16:34] INFO: GC completed, freed 1.2GB
[2025-11-30 10:16:35] INFO: Running inference on batch 6/10...
[2025-11-30 10:16:37] INFO: Running inference on batch 7/10...
[2025-11-30 10:16:39] INFO: Running inference on batch 8/10...
[2025-11-30 10:16:41] INFO: Running inference on batch 9/10...
[2025-11-30 10:16:43] INFO: Running inference on batch 10/10...
[2025-11-30 10:16:45] INFO: Inference complete: 500,000 predictions generated
[2025-11-30 10:16:46] INFO: Pipeline Stage 9/10: Results Aggregation Started
[2025-11-30 10:16:47] INFO: Aggregating predictions by customer segment...
[2025-11-30 10:16:49] INFO: Generating summary statistics...
[2025-11-30 10:16:51] INFO: Computing confidence intervals...
[2025-11-30 10:16:53] ERROR: Unexpected data format in aggregation stage`,
			PostContext: `[2025-11-30 10:16:53] ERROR: Stack trace:
Traceback (most recent call last):
  File "pipeline/orchestrator.py", line 234, in run_pipeline
    stage.execute(data_context)
  File "pipeline/stages/aggregation.py", line 89, in execute
    results = self.aggregate_results(context.predictions)
  File "pipeline/stages/aggregation.py", line 156, in aggregate_results
    grouped = predictions.groupby(['segment', 'tier'])
  File "pandas/core/frame.py", line 8402, in groupby
    return DataFrameGroupBy(obj=self, keys=by, **kwargs)
  File "pandas/core/groupby/groupby.py", line 965, in __init__
    raise KeyError(f"Column '{key}' not found in DataFrame")
RuntimeError: Failed to process large batch job after extensive processing pipeline

[2025-11-30 10:16:54] ERROR: Pipeline failed at stage 9/10
[2025-11-30 10:16:54] ERROR: Total execution time: 1m 31s
[2025-11-30 10:16:54] INFO: Cleaning up temporary resources...
[2025-11-30 10:16:55] INFO: Temporary files removed: 15.2GB freed
[2025-11-30 10:16:55] INFO: Closing database connections...
[2025-11-30 10:16:56] INFO: Releasing compute resources...
[2025-11-30 10:16:57] ERROR: Pipeline execution FAILED
[2025-11-30 10:16:57] INFO: Error report saved to: logs/pipeline-error-2025-11-30-10-16-57.json
[2025-11-30 10:16:57] INFO: Performance metrics saved to: metrics/pipeline-2025-11-30.csv
[2025-11-30 10:16:58] INFO: Sending notification to on-call engineer
[2025-11-30 10:16:59] INFO: Notification sent successfully
[2025-11-30 10:17:00] INFO: Exiting with error code 1`,
			Metadata: map[string]string{
				"recurrence_count": "7",
			},
		},
	}
}
