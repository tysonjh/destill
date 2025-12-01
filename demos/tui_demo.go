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
	}
}