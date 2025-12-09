// Package store provides a Postgres store implementation.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq" // Postgres driver

	"destill-agent/src/contracts"
)

// PostgresStore is a Postgres implementation of Store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new Postgres store.
// dsn format: "postgres://user:password@host:port/dbname?sslmode=disable"
func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &PostgresStore{db: db}, nil
}

// CreateRequest creates a new analysis request record.
func (s *PostgresStore) CreateRequest(ctx context.Context, requestID string, buildURL string) error {
	query := `
		INSERT INTO requests (request_id, build_url, status, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (request_id) DO NOTHING
	`

	_, err := s.db.ExecContext(ctx, query, requestID, buildURL, "pending", time.Now())
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	return nil
}

// GetRequestStatus returns the status of a request.
func (s *PostgresStore) GetRequestStatus(ctx context.Context, requestID string) (*contracts.RequestStatus, error) {
	query := `
		SELECT request_id, build_url, status, chunks_total, chunks_processed, findings_count
		FROM requests
		WHERE request_id = $1
	`

	var status contracts.RequestStatus
	err := s.db.QueryRowContext(ctx, query, requestID).Scan(
		&status.RequestID,
		&status.BuildURL,
		&status.Status,
		&status.ChunksTotal,
		&status.ChunksProcessed,
		&status.FindingsCount,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("request not found: %s", requestID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get request status: %w", err)
	}

	return &status, nil
}

// UpdateRequestStatus updates the status of a request.
func (s *PostgresStore) UpdateRequestStatus(ctx context.Context, status *contracts.RequestStatus) error {
	query := `
		UPDATE requests
		SET status = $2,
		    chunks_total = $3,
		    chunks_processed = $4,
		    findings_count = $5,
		    completed_at = CASE WHEN $2 = 'completed' THEN NOW() ELSE completed_at END
		WHERE request_id = $1
	`

	result, err := s.db.ExecContext(ctx, query,
		status.RequestID,
		status.Status,
		status.ChunksTotal,
		status.ChunksProcessed,
		status.FindingsCount,
	)
	if err != nil {
		return fmt.Errorf("failed to update request status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("request not found: %s", status.RequestID)
	}

	return nil
}

// SaveFinding saves a single finding.
func (s *PostgresStore) SaveFinding(ctx context.Context, finding *contracts.TriageCardV2) error {
	// Marshal context arrays to JSON
	preContextJSON, err := json.Marshal(finding.PreContext)
	if err != nil {
		return fmt.Errorf("failed to marshal pre_context: %w", err)
	}

	postContextJSON, err := json.Marshal(finding.PostContext)
	if err != nil {
		return fmt.Errorf("failed to marshal post_context: %w", err)
	}

	metadataJSON, err := json.Marshal(finding.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO findings (
			request_id, build_url, job_name, message_hash, severity, confidence_score,
			raw_message, normalized_message, pre_context, post_context,
			source, line_number, chunk_index, metadata, analyzed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err = s.db.ExecContext(ctx, query,
		finding.RequestID,
		finding.BuildURL,
		finding.JobName,
		finding.MessageHash,
		finding.Severity,
		finding.ConfidenceScore,
		finding.RawMessage,
		finding.NormalizedMsg,
		preContextJSON,
		postContextJSON,
		finding.Source,
		finding.LineInChunk,
		finding.ChunkIndex,
		metadataJSON,
		time.Now(),
	)

	if err != nil {
		return fmt.Errorf("failed to save finding: %w", err)
	}

	return nil
}

// GetFindings retrieves all findings for a request.
func (s *PostgresStore) GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCardV2, error) {
	query := `
		SELECT 
			id, request_id, build_url, job_name, message_hash, severity, confidence_score,
			raw_message, normalized_message, pre_context, post_context,
			source, line_number, chunk_index, metadata, analyzed_at
		FROM findings
		WHERE request_id = $1
		ORDER BY confidence_score DESC, analyzed_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, requestID)
	if err != nil {
		return nil, fmt.Errorf("failed to query findings: %w", err)
	}
	defer rows.Close()

	var findings []contracts.TriageCardV2

	for rows.Next() {
		var finding contracts.TriageCardV2
		var preContextJSON, postContextJSON, metadataJSON []byte
		var analyzedAt time.Time

		err := rows.Scan(
			&finding.ID,
			&finding.RequestID,
			&finding.BuildURL,
			&finding.JobName,
			&finding.MessageHash,
			&finding.Severity,
			&finding.ConfidenceScore,
			&finding.RawMessage,
			&finding.NormalizedMsg,
			&preContextJSON,
			&postContextJSON,
			&finding.Source,
			&finding.LineInChunk,
			&finding.ChunkIndex,
			&metadataJSON,
			&analyzedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan finding: %w", err)
		}

		// Unmarshal JSON fields
		if err := json.Unmarshal(preContextJSON, &finding.PreContext); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pre_context: %w", err)
		}
		if err := json.Unmarshal(postContextJSON, &finding.PostContext); err != nil {
			return nil, fmt.Errorf("failed to unmarshal post_context: %w", err)
		}
		if err := json.Unmarshal(metadataJSON, &finding.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		finding.Timestamp = analyzedAt.Format(time.RFC3339)

		findings = append(findings, finding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating findings: %w", err)
	}

	return findings, nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

