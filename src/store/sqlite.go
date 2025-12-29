// Package store defines interfaces for persistent data storage.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3" // SQLite driver

	"destill-agent/src/contracts"
)

// TestResult represents a single test execution record.
type TestResult struct {
	ID             int64
	PipelineID     string
	TestName       string
	BuildNumber    int
	Passed         bool
	FailureMessage string
	BuildURL       string
	JobName        string
	CreatedAt      time.Time
}

// FindingResult represents a single finding occurrence in a build.
type FindingResult struct {
	ID          int64
	PipelineID  string
	MessageHash string
	BuildNumber int
	JobName     string
	JobPassed   bool
	Severity    string
	Confidence  float64
	CreatedAt   time.Time
}

// FindingNoveltyInfo contains novelty information for a finding.
type FindingNoveltyInfo struct {
	IsNovel           bool    // Never seen before in this pipeline
	SeenInPassingJobs bool    // Has appeared in passing jobs before
	TotalOccurrences  int     // How many times this finding has appeared
	PassingOccurs     int     // Occurrences in passing jobs
	FailingOccurs     int     // Occurrences in failing jobs
	FirstSeenBuild    int     // Build number where first seen (0 if novel)
	LastSeenBuild     int     // Most recent build where seen (0 if novel)
}

// TestHistory provides persistent storage for test results.
// Used for flaky test detection via sliding window analysis.
type TestHistory struct {
	db *sql.DB
}

// NewTestHistory creates a new TestHistory store.
// If dbPath is empty, uses default location: ~/.destill/history.db
func NewTestHistory(dbPath string) (*TestHistory, error) {
	if dbPath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		destillDir := filepath.Join(homeDir, ".destill")
		if err := os.MkdirAll(destillDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create .destill directory: %w", err)
		}
		dbPath = filepath.Join(destillDir, "history.db")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	th := &TestHistory{db: db}
	if err := th.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return th, nil
}

// migrate creates the schema if it doesn't exist.
func (th *TestHistory) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS test_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pipeline_id TEXT NOT NULL,
		test_name TEXT NOT NULL,
		build_number INTEGER NOT NULL,
		passed INTEGER NOT NULL,
		failure_message TEXT,
		build_url TEXT,
		job_name TEXT,
		created_at TEXT NOT NULL,
		UNIQUE(pipeline_id, test_name, build_number)
	);
	CREATE INDEX IF NOT EXISTS idx_pipeline_test ON test_results(pipeline_id, test_name);

	CREATE TABLE IF NOT EXISTS finding_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pipeline_id TEXT NOT NULL,
		message_hash TEXT NOT NULL,
		build_number INTEGER NOT NULL,
		job_name TEXT NOT NULL,
		job_passed INTEGER NOT NULL,
		severity TEXT,
		confidence REAL,
		created_at TEXT NOT NULL,
		UNIQUE(pipeline_id, message_hash, build_number, job_name)
	);
	CREATE INDEX IF NOT EXISTS idx_finding_pipeline_hash
		ON finding_history(pipeline_id, message_hash);
	CREATE INDEX IF NOT EXISTS idx_finding_pipeline_build
		ON finding_history(pipeline_id, build_number DESC);
	`
	if _, err := th.db.Exec(schema); err != nil {
		return err
	}

	// Add job_name column to existing tables (SQLite ignores if already exists)
	th.db.Exec("ALTER TABLE test_results ADD COLUMN job_name TEXT")
	return nil
}

// RecordBuildData stores both test results and findings for a build.
// This is the unified entry point for persisting build analysis data.
// Either testResults or cards can be nil if not available.
func (th *TestHistory) RecordBuildData(ctx context.Context, pipelineID string, buildNumber int, testResults []contracts.TestResult, cards []contracts.TriageCard) error {
	if pipelineID == "" || buildNumber == 0 {
		return nil // Nothing to record without identifiers
	}

	// Record test results
	for _, tr := range testResults {
		result := TestResult{
			PipelineID:     pipelineID,
			TestName:       tr.TestName,
			BuildNumber:    buildNumber,
			Passed:         tr.Passed,
			FailureMessage: tr.FailureMessage,
			BuildURL:       tr.BuildURL,
			JobName:        tr.JobName,
			CreatedAt:      time.Now().UTC(),
		}
		if err := th.RecordResult(ctx, result); err != nil {
			return fmt.Errorf("failed to record test result: %w", err)
		}
	}

	// Record findings
	if len(cards) > 0 {
		if err := th.RecordFindingsFromCards(ctx, pipelineID, buildNumber, cards); err != nil {
			return fmt.Errorf("failed to record findings: %w", err)
		}
	}

	return nil
}

// RecordResult stores a test result. Uses INSERT OR REPLACE to handle duplicates.
func (th *TestHistory) RecordResult(ctx context.Context, result TestResult) error {
	query := `
	INSERT OR REPLACE INTO test_results
		(pipeline_id, test_name, build_number, passed, failure_message, build_url, job_name, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	passed := 0
	if result.Passed {
		passed = 1
	}
	createdAt := result.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := th.db.ExecContext(ctx, query,
		result.PipelineID,
		result.TestName,
		result.BuildNumber,
		passed,
		result.FailureMessage,
		result.BuildURL,
		result.JobName,
		createdAt.Format(time.RFC3339),
	)
	return err
}

// GetProcessedBuilds returns a set of build numbers that have been processed for a pipeline.
func (th *TestHistory) GetProcessedBuilds(ctx context.Context, pipelineID string) (map[int]bool, error) {
	query := `SELECT DISTINCT build_number FROM test_results WHERE pipeline_id = ?`

	rows, err := th.db.QueryContext(ctx, query, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("failed to query processed builds: %w", err)
	}
	defer rows.Close()

	processed := make(map[int]bool)
	for rows.Next() {
		var buildNumber int
		if err := rows.Scan(&buildNumber); err != nil {
			return nil, fmt.Errorf("failed to scan build number: %w", err)
		}
		processed[buildNumber] = true
	}

	return processed, rows.Err()
}

// HasBuild checks if test results exist for a specific build.
func (th *TestHistory) HasBuild(ctx context.Context, pipelineID string, buildNumber int) (bool, error) {
	query := `SELECT COUNT(*) FROM test_results WHERE pipeline_id = ? AND build_number = ? LIMIT 1`

	var count int
	err := th.db.QueryRowContext(ctx, query, pipelineID, buildNumber).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check build: %w", err)
	}

	return count > 0, nil
}

// GetBuildResults retrieves all test results for a specific build.
func (th *TestHistory) GetBuildResults(ctx context.Context, pipelineID string, buildNumber int) ([]TestResult, error) {
	query := `
	SELECT id, pipeline_id, test_name, build_number, passed, failure_message, build_url, job_name, created_at
	FROM test_results
	WHERE pipeline_id = ? AND build_number = ?
	`

	rows, err := th.db.QueryContext(ctx, query, pipelineID, buildNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to query build results: %w", err)
	}
	defer rows.Close()

	var results []TestResult
	for rows.Next() {
		var r TestResult
		var passed int
		var createdAtStr string
		var failureMsg, buildURL, jobName sql.NullString

		err := rows.Scan(&r.ID, &r.PipelineID, &r.TestName, &r.BuildNumber,
			&passed, &failureMsg, &buildURL, &jobName, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		r.Passed = passed == 1
		if failureMsg.Valid {
			r.FailureMessage = failureMsg.String
		}
		if buildURL.Valid {
			r.BuildURL = buildURL.String
		}
		if jobName.Valid {
			r.JobName = jobName.String
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		results = append(results, r)
	}

	return results, rows.Err()
}

// Close closes the database connection.
func (th *TestHistory) Close() error {
	return th.db.Close()
}


// TestFlakeInfo contains flakiness information for a test.
type TestFlakeInfo struct {
	IsFlaky      bool
	FailureRate  float64
	TotalRuns    int
	FailedRuns   int
	LastFailedAt time.Time // Most recent failure time (zero if never failed)
}

// GetTestFlakeInfo checks if a specific test is flaky based on historical data.
// Excludes the current build from the calculation.
func (th *TestHistory) GetTestFlakeInfo(ctx context.Context, pipelineID, testName string, excludeBuild int) (TestFlakeInfo, error) {
	query := `
	SELECT passed, created_at
	FROM (
		SELECT passed, build_number, created_at
		FROM test_results
		WHERE pipeline_id = ? AND test_name = ? AND build_number != ?
		ORDER BY build_number DESC
		LIMIT ?
	)
	`

	rows, err := th.db.QueryContext(ctx, query, pipelineID, testName, excludeBuild, contracts.FlakeWindowSize)
	if err != nil {
		return TestFlakeInfo{}, fmt.Errorf("failed to query test history: %w", err)
	}
	defer rows.Close()

	var totalRuns, failedRuns int
	var lastFailedAt time.Time
	for rows.Next() {
		var passed int
		var createdAtStr string
		if err := rows.Scan(&passed, &createdAtStr); err != nil {
			return TestFlakeInfo{}, fmt.Errorf("failed to scan row: %w", err)
		}
		totalRuns++
		if passed == 0 {
			failedRuns++
			// Track most recent failure (first one we see since ordered by build_number DESC)
			if lastFailedAt.IsZero() {
				lastFailedAt, _ = time.Parse(time.RFC3339, createdAtStr)
			}
		}
	}

	if err := rows.Err(); err != nil {
		return TestFlakeInfo{}, err
	}

	info := TestFlakeInfo{
		TotalRuns:    totalRuns,
		FailedRuns:   failedRuns,
		LastFailedAt: lastFailedAt,
	}

	if totalRuns > 0 {
		info.FailureRate = float64(failedRuns) / float64(totalRuns)
	}

	// Flaky if: enough samples AND failure rate exceeds threshold
	info.IsFlaky = totalRuns >= contracts.FlakeMinSamples && info.FailureRate >= contracts.FlakeThreshold

	return info, nil
}

// RecordFinding stores a finding occurrence. Uses INSERT OR REPLACE for idempotency.
func (th *TestHistory) RecordFinding(ctx context.Context, finding FindingResult) error {
	query := `
	INSERT OR REPLACE INTO finding_history
		(pipeline_id, message_hash, build_number, job_name, job_passed, severity, confidence, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	jobPassed := 0
	if finding.JobPassed {
		jobPassed = 1
	}
	createdAt := finding.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := th.db.ExecContext(ctx, query,
		finding.PipelineID,
		finding.MessageHash,
		finding.BuildNumber,
		finding.JobName,
		jobPassed,
		finding.Severity,
		finding.Confidence,
		createdAt.Format(time.RFC3339),
	)
	return err
}

// RecordFindingsFromCards batch records findings from TriageCards for a build.
func (th *TestHistory) RecordFindingsFromCards(ctx context.Context, pipelineID string, buildNumber int, cards []contracts.TriageCard) error {
	tx, err := th.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO finding_history
		(pipeline_id, message_hash, build_number, job_name, job_passed, severity, confidence, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)

	for _, card := range cards {
		if card.MessageHash == "" {
			continue
		}

		jobState := card.Metadata["job_state"]
		if jobState == "" {
			continue
		}

		jobPassed := 0
		if jobState == "passed" {
			jobPassed = 1
		}

		_, err := stmt.ExecContext(ctx,
			pipelineID,
			card.MessageHash,
			buildNumber,
			card.JobName,
			jobPassed,
			card.Severity,
			card.ConfidenceScore,
			now,
		)
		if err != nil {
			return fmt.Errorf("failed to insert finding: %w", err)
		}
	}

	return tx.Commit()
}

// GetFindingNoveltyInfo checks if a finding is novel and its history in passing jobs.
// excludeBuild: The current build to exclude from analysis.
func (th *TestHistory) GetFindingNoveltyInfo(ctx context.Context, pipelineID, messageHash string, excludeBuild int) (FindingNoveltyInfo, error) {
	query := `
	SELECT job_passed, build_number
	FROM (
		SELECT job_passed, build_number
		FROM finding_history
		WHERE pipeline_id = ? AND message_hash = ? AND build_number != ?
		ORDER BY build_number DESC
		LIMIT ?
	)
	`

	rows, err := th.db.QueryContext(ctx, query,
		pipelineID, messageHash, excludeBuild, contracts.FindingWindowSize)
	if err != nil {
		return FindingNoveltyInfo{}, fmt.Errorf("failed to query finding history: %w", err)
	}
	defer rows.Close()

	var info FindingNoveltyInfo
	var firstBuild, lastBuild int

	for rows.Next() {
		var jobPassed int
		var buildNumber int
		if err := rows.Scan(&jobPassed, &buildNumber); err != nil {
			return FindingNoveltyInfo{}, fmt.Errorf("failed to scan row: %w", err)
		}

		info.TotalOccurrences++
		if jobPassed == 1 {
			info.PassingOccurs++
			info.SeenInPassingJobs = true
		} else {
			info.FailingOccurs++
		}

		// Track first/last (first row is most recent due to ORDER BY DESC)
		if lastBuild == 0 {
			lastBuild = buildNumber
		}
		firstBuild = buildNumber
	}

	if err := rows.Err(); err != nil {
		return FindingNoveltyInfo{}, err
	}

	info.IsNovel = info.TotalOccurrences == 0
	info.FirstSeenBuild = firstBuild
	info.LastSeenBuild = lastBuild

	return info, nil
}

// LoadFindingNoveltyMap loads novelty info for all findings in pipeline history.
// Returns a map for O(1) lookup by message hash. This is more efficient than
// calling GetFindingNoveltyInfo for each finding when processing many findings.
func (th *TestHistory) LoadFindingNoveltyMap(ctx context.Context, pipelineID string, excludeBuild int) (map[string]FindingNoveltyInfo, error) {
	// Get the build number cutoff for the window
	var cutoffBuild sql.NullInt64
	cutoffQuery := `
	SELECT MIN(build_number) FROM (
		SELECT DISTINCT build_number
		FROM finding_history
		WHERE pipeline_id = ? AND build_number != ?
		ORDER BY build_number DESC
		LIMIT ?
	)
	`
	err := th.db.QueryRowContext(ctx, cutoffQuery, pipelineID, excludeBuild, contracts.FindingWindowSize).Scan(&cutoffBuild)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get cutoff build: %w", err)
	}

	// If no data exists, return empty map
	if !cutoffBuild.Valid {
		return make(map[string]FindingNoveltyInfo), nil
	}

	query := `
	SELECT message_hash,
	       SUM(CASE WHEN job_passed = 1 THEN 1 ELSE 0 END) as passing_occurs,
	       SUM(CASE WHEN job_passed = 0 THEN 1 ELSE 0 END) as failing_occurs,
	       MIN(build_number) as first_seen,
	       MAX(build_number) as last_seen
	FROM finding_history
	WHERE pipeline_id = ?
	  AND build_number != ?
	  AND build_number >= ?
	GROUP BY message_hash
	`

	rows, err := th.db.QueryContext(ctx, query, pipelineID, excludeBuild, cutoffBuild.Int64)
	if err != nil {
		return nil, fmt.Errorf("failed to query finding history: %w", err)
	}
	defer rows.Close()

	result := make(map[string]FindingNoveltyInfo)
	for rows.Next() {
		var hash string
		var info FindingNoveltyInfo
		err := rows.Scan(&hash, &info.PassingOccurs, &info.FailingOccurs,
			&info.FirstSeenBuild, &info.LastSeenBuild)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		info.TotalOccurrences = info.PassingOccurs + info.FailingOccurs
		info.SeenInPassingJobs = info.PassingOccurs > 0
		info.IsNovel = false // If it's in the map, it's not novel
		result[hash] = info
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// GetFindingHistory retrieves the recent history for a specific finding hash.
func (th *TestHistory) GetFindingHistory(ctx context.Context, pipelineID, messageHash string, excludeBuild int) ([]FindingResult, error) {
	query := `
	SELECT id, pipeline_id, message_hash, build_number, job_name, job_passed, severity, confidence, created_at
	FROM finding_history
	WHERE pipeline_id = ? AND message_hash = ? AND build_number != ?
	ORDER BY build_number DESC
	LIMIT ?
	`

	rows, err := th.db.QueryContext(ctx, query,
		pipelineID, messageHash, excludeBuild, contracts.FindingWindowSize)
	if err != nil {
		return nil, fmt.Errorf("failed to query finding history: %w", err)
	}
	defer rows.Close()

	var results []FindingResult
	for rows.Next() {
		var r FindingResult
		var jobPassed int
		var createdAtStr string
		var severity sql.NullString
		var confidence sql.NullFloat64

		err := rows.Scan(&r.ID, &r.PipelineID, &r.MessageHash, &r.BuildNumber,
			&r.JobName, &jobPassed, &severity, &confidence, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		r.JobPassed = jobPassed == 1
		if severity.Valid {
			r.Severity = severity.String
		}
		if confidence.Valid {
			r.Confidence = confidence.Float64
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		results = append(results, r)
	}

	return results, rows.Err()
}

// HasFindingBuild checks if findings exist for a specific build.
func (th *TestHistory) HasFindingBuild(ctx context.Context, pipelineID string, buildNumber int) (bool, error) {
	query := `SELECT COUNT(*) FROM finding_history WHERE pipeline_id = ? AND build_number = ? LIMIT 1`

	var count int
	err := th.db.QueryRowContext(ctx, query, pipelineID, buildNumber).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check finding build: %w", err)
	}

	return count > 0, nil
}

// PruneOldFindings removes findings older than the retention window.
// Keeps the most recent keepBuilds builds per pipeline.
func (th *TestHistory) PruneOldFindings(ctx context.Context, pipelineID string, keepBuilds int) error {
	query := `
	DELETE FROM finding_history
	WHERE pipeline_id = ?
	AND build_number < (
		SELECT MIN(build_number) FROM (
			SELECT DISTINCT build_number
			FROM finding_history
			WHERE pipeline_id = ?
			ORDER BY build_number DESC
			LIMIT ?
		)
	)
	`

	_, err := th.db.ExecContext(ctx, query, pipelineID, pipelineID, keepBuilds)
	if err != nil {
		return fmt.Errorf("failed to prune old findings: %w", err)
	}
	return nil
}
