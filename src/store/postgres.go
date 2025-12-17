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
func (s *PostgresStore) GetFindings(ctx context.Context, requestID string) ([]contracts.TriageCard, error) {
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

	var findings []contracts.TriageCard

	for rows.Next() {
		var finding contracts.TriageCard
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

// GetByHash retrieves a single finding by message hash.
func (s *PostgresStore) GetByHash(ctx context.Context, requestID, messageHash string) (contracts.TriageCard, error) {
	query := `
		SELECT
			id, request_id, build_url, job_name, message_hash, severity, confidence_score,
			raw_message, normalized_message, pre_context, post_context,
			source, line_number, chunk_index, metadata, analyzed_at
		FROM findings
		WHERE request_id = $1 AND message_hash = $2
		LIMIT 1
	`

	var finding contracts.TriageCard
	var preContextJSON, postContextJSON, metadataJSON []byte
	var analyzedAt time.Time

	err := s.db.QueryRowContext(ctx, query, requestID, messageHash).Scan(
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
	if err == sql.ErrNoRows {
		return contracts.TriageCard{}, ErrNotFound{RequestID: requestID, MessageHash: messageHash}
	}
	if err != nil {
		return contracts.TriageCard{}, fmt.Errorf("failed to query finding: %w", err)
	}

	// Unmarshal JSON fields
	if err := json.Unmarshal(preContextJSON, &finding.PreContext); err != nil {
		return contracts.TriageCard{}, fmt.Errorf("failed to unmarshal pre_context: %w", err)
	}
	if err := json.Unmarshal(postContextJSON, &finding.PostContext); err != nil {
		return contracts.TriageCard{}, fmt.Errorf("failed to unmarshal post_context: %w", err)
	}
	if err := json.Unmarshal(metadataJSON, &finding.Metadata); err != nil {
		return contracts.TriageCard{}, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	finding.Timestamp = analyzedAt.Format(time.RFC3339)

	return finding, nil
}

// Store saves findings for a request.
// Note: In distributed mode, findings are typically persisted via Kafka sink.
// This method is provided for interface compatibility and testing.
func (s *PostgresStore) Store(ctx context.Context, requestID string, cards []contracts.TriageCard) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO findings (
			request_id, build_url, job_name, message_hash, severity, confidence_score,
			raw_message, normalized_message, pre_context, post_context,
			source, line_number, chunk_index, metadata, analyzed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (request_id, message_hash) DO UPDATE SET
			confidence_score = EXCLUDED.confidence_score,
			metadata = EXCLUDED.metadata
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, card := range cards {
		preContextJSON, err := json.Marshal(card.PreContext)
		if err != nil {
			return fmt.Errorf("failed to marshal pre_context: %w", err)
		}
		postContextJSON, err := json.Marshal(card.PostContext)
		if err != nil {
			return fmt.Errorf("failed to marshal post_context: %w", err)
		}
		metadataJSON, err := json.Marshal(card.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}

		analyzedAt := time.Now().UTC()
		if card.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, card.Timestamp); err == nil {
				analyzedAt = t
			}
		}

		_, err = stmt.ExecContext(ctx,
			card.RequestID,
			card.BuildURL,
			card.JobName,
			card.MessageHash,
			card.Severity,
			card.ConfidenceScore,
			card.RawMessage,
			card.NormalizedMsg,
			preContextJSON,
			postContextJSON,
			card.Source,
			card.LineInChunk,
			card.ChunkIndex,
			metadataJSON,
			analyzedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to insert finding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetLatestRequestByBuildURL retrieves the most recent request ID for a given build URL.
func (s *PostgresStore) GetLatestRequestByBuildURL(ctx context.Context, buildURL string) (string, error) {
	query := `
		SELECT request_id
		FROM requests
		WHERE build_url = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var requestID string
	err := s.db.QueryRowContext(ctx, query, buildURL).Scan(&requestID)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no requests found for build URL: %s", buildURL)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query request: %w", err)
	}

	return requestID, nil
}

// Close closes the database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}
