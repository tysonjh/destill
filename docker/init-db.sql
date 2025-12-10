-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Findings table: stores triage cards from analysis
CREATE TABLE findings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) NOT NULL,
    build_url TEXT NOT NULL,
    job_name VARCHAR(255) NOT NULL,
    
    -- The actual finding
    message_hash VARCHAR(64) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    confidence_score DECIMAL(3,2) NOT NULL,
    
    -- Content (stored as JSONB for flexibility)
    raw_message TEXT NOT NULL,
    normalized_message TEXT NOT NULL,
    pre_context JSONB NOT NULL DEFAULT '[]',  -- Array of context lines
    post_context JSONB NOT NULL DEFAULT '[]', -- Array of context lines
    
    -- Metadata
    source VARCHAR(50) NOT NULL DEFAULT 'buildkite',
    line_number INTEGER,
    chunk_index INTEGER,
    metadata JSONB NOT NULL DEFAULT '{}',
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    analyzed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    
    -- Indexes for common queries
    CONSTRAINT findings_confidence_check CHECK (confidence_score >= 0 AND confidence_score <= 1)
);

-- Indexes
CREATE INDEX idx_findings_request_id ON findings(request_id);
CREATE INDEX idx_findings_message_hash ON findings(message_hash);
CREATE INDEX idx_findings_job_name ON findings(job_name);
CREATE INDEX idx_findings_confidence ON findings(confidence_score DESC);
CREATE INDEX idx_findings_created_at ON findings(created_at DESC);

-- Requests table: tracks analysis requests
CREATE TABLE requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    request_id VARCHAR(255) UNIQUE NOT NULL,
    build_url TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    
    -- Counts
    chunks_total INTEGER DEFAULT 0,
    chunks_processed INTEGER DEFAULT 0,
    findings_count INTEGER DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    
    CONSTRAINT requests_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed'))
);

CREATE INDEX idx_requests_status ON requests(status);
CREATE INDEX idx_requests_created_at ON requests(created_at DESC);

-- View for aggregated findings by hash (recurrence tracking)
CREATE VIEW findings_summary AS
SELECT 
    message_hash,
    normalized_message,
    MAX(severity) as severity,
    AVG(confidence_score) as avg_confidence,
    COUNT(*) as recurrence_count,
    array_agg(DISTINCT job_name) as job_names,
    MIN(created_at) as first_seen,
    MAX(created_at) as last_seen
FROM findings
GROUP BY message_hash, normalized_message;

