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

// RecordResults stores multiple test results in a single transaction.
func (th *TestHistory) RecordResults(ctx context.Context, results []TestResult) error {
	tx, err := th.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO test_results
			(pipeline_id, test_name, build_number, passed, failure_message, build_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, result := range results {
		passed := 0
		if result.Passed {
			passed = 1
		}
		createdAt := result.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}

		_, err := stmt.ExecContext(ctx,
			result.PipelineID,
			result.TestName,
			result.BuildNumber,
			passed,
			result.FailureMessage,
			result.BuildURL,
			createdAt.Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to insert result for %s: %w", result.TestName, err)
		}
	}

	return tx.Commit()
}

// GetRecentResults retrieves the last N results for a specific test.
func (th *TestHistory) GetRecentResults(ctx context.Context, pipelineID, testName string, limit int) ([]TestResult, error) {
	query := `
	SELECT id, pipeline_id, test_name, build_number, passed, failure_message, build_url, created_at
	FROM test_results
	WHERE pipeline_id = ? AND test_name = ?
	ORDER BY build_number DESC
	LIMIT ?
	`

	rows, err := th.db.QueryContext(ctx, query, pipelineID, testName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query results: %w", err)
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

// TestStats holds pass/fail statistics for a test.
type TestStats struct {
	TestName    string
	TotalRuns   int
	FailedRuns  int
	FailureRate float64
}

// GetTestStats returns pass/fail stats for a test over the last N builds.
func (th *TestHistory) GetTestStats(ctx context.Context, pipelineID, testName string, windowSize int) (TestStats, error) {
	query := `
	SELECT passed, COUNT(*) as cnt
	FROM (
		SELECT passed
		FROM test_results
		WHERE pipeline_id = ? AND test_name = ?
		ORDER BY build_number DESC
		LIMIT ?
	)
	GROUP BY passed
	`

	rows, err := th.db.QueryContext(ctx, query, pipelineID, testName, windowSize)
	if err != nil {
		return TestStats{}, fmt.Errorf("failed to query stats: %w", err)
	}
	defer rows.Close()

	stats := TestStats{TestName: testName}
	for rows.Next() {
		var passed, count int
		if err := rows.Scan(&passed, &count); err != nil {
			return TestStats{}, fmt.Errorf("failed to scan row: %w", err)
		}
		stats.TotalRuns += count
		if passed == 0 {
			stats.FailedRuns = count
		}
	}

	if stats.TotalRuns > 0 {
		stats.FailureRate = float64(stats.FailedRuns) / float64(stats.TotalRuns)
	}

	return stats, rows.Err()
}

// GetAllTestStats returns stats for all tests in a pipeline over the last N builds.
func (th *TestHistory) GetAllTestStats(ctx context.Context, pipelineID string, windowSize int) ([]TestStats, error) {
	// First get all unique test names for this pipeline
	namesQuery := `SELECT DISTINCT test_name FROM test_results WHERE pipeline_id = ?`
	rows, err := th.db.QueryContext(ctx, namesQuery, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("failed to query test names: %w", err)
	}

	var testNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, fmt.Errorf("failed to scan test name: %w", err)
		}
		testNames = append(testNames, name)
	}
	rows.Close()

	// Get stats for each test
	var allStats []TestStats
	for _, name := range testNames {
		stats, err := th.GetTestStats(ctx, pipelineID, name, windowSize)
		if err != nil {
			return nil, err
		}
		allStats = append(allStats, stats)
	}

	return allStats, nil
}

// Close closes the database connection.
func (th *TestHistory) Close() error {
	return th.db.Close()
}
