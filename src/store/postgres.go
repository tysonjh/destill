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
