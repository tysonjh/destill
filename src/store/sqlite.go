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
	CreatedAt      time.Time
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
		created_at TEXT NOT NULL,
		UNIQUE(pipeline_id, test_name, build_number)
	);
	CREATE INDEX IF NOT EXISTS idx_pipeline_test ON test_results(pipeline_id, test_name);
	`
	_, err := th.db.Exec(schema)
	return err
}

// RecordResult stores a test result. Uses INSERT OR REPLACE to handle duplicates.
func (th *TestHistory) RecordResult(ctx context.Context, result TestResult) error {
	query := `
	INSERT OR REPLACE INTO test_results
		(pipeline_id, test_name, build_number, passed, failure_message, build_url, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
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
	SELECT id, pipeline_id, test_name, build_number, passed, failure_message, build_url, created_at
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
		var failureMsg, buildURL sql.NullString

		err := rows.Scan(&r.ID, &r.PipelineID, &r.TestName, &r.BuildNumber,
			&passed, &failureMsg, &buildURL, &createdAtStr)
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
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		results = append(results, r)
	}

	return results, rows.Err()
}

// Close closes the database connection.
func (th *TestHistory) Close() error {
	return th.db.Close()
}

// Flaky detection constants
const (
	// FlakeWindowSize is the number of recent builds to consider for flakiness.
	FlakeWindowSize = 20

	// FlakeThreshold is the minimum failure rate to consider a test flaky.
	// 0.10 = 10% of runs failed = flaky
	FlakeThreshold = 0.10

	// FlakeMinSamples is the minimum number of runs required to make a judgment.
	FlakeMinSamples = 5
)

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

	rows, err := th.db.QueryContext(ctx, query, pipelineID, testName, excludeBuild, FlakeWindowSize)
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
	info.IsFlaky = totalRuns >= FlakeMinSamples && info.FailureRate >= FlakeThreshold

	return info, nil
}
