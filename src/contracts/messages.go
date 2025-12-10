// Package contracts defines message types for the Agentic Data Plane architecture.
package contracts

// TriageCard represents a log entry that has been ingested and is ready for analysis.
// This is the original format still used by the CLI and TUI.
type TriageCard struct {
	// Unique identifier.
	ID string `json:"id"`
	// Originating platform for this message (e.g. "buildkite", "github-actions").
	Source string `json:"source"`
	// Time when the log was created.
	Timestamp string `json:"timestamp"`
	// Log level (e.g., INFO, WARN, ERROR).
	Severity string `json:"severity"`
	// Normalized log message (placeholders like [TIMESTAMP], [IP], [VAR] for grouping/hashing).
	Message string `json:"message"`
	// Original raw log message (for display to humans with actual values).
	RawMessage string `json:"raw_message,omitempty"`
	// Additional key-value pairs associated with the log.
	Metadata map[string]string `json:"metadata"`
	// Original analysis request (e.g., the Build ID from BuildKite).
	RequestID string `json:"request_id"`
	// Unique hash of the normalized failure message for recurrence tracking.
	MessageHash string `json:"message_hash"`
	// Name of the job within the build.
	JobName string `json:"job_name"`
	// Determines its rank in the stack (0.0 to 1.0).
	ConfidenceScore float64 `json:"confidence_score"`
	// Lines of log context immediately preceding the error (typically 5 lines).
	PreContext string `json:"pre_context,omitempty"`
	// Lines of log context immediately following the error (typically 10 lines).
	PostContext string `json:"post_context,omitempty"`
}

// LogChunk represents a raw chunk of log data received from CI systems.
// This is the original format still used by the CLI and agents.
type LogChunk struct {
	// Unique identifier.
	ID string `json:"id"`
	// Original analysis request ID.
	RequestID string `json:"request_id"`
	// Name of the job within the build.
	JobName string `json:"job_name"`
	// Raw log content.
	Content string `json:"content"`
	// Time when the log chunk was created.
	Timestamp string `json:"timestamp"`
	// Optional map for any initial metadata captured by the Ingestion Agent.
	Metadata map[string]string `json:"metadata"`
}

// LogChunkV2 represents a chunk of log data for the agentic architecture.
// Published to: destill.logs.raw
// Key: {build_id}
type LogChunkV2 struct {
	RequestID   string            `json:"request_id"`
	BuildID     string            `json:"build_id"`
	JobName     string            `json:"job_name"`
	JobID       string            `json:"job_id"`
	ChunkIndex  int               `json:"chunk_index"`
	TotalChunks int               `json:"total_chunks"`
	Content     string            `json:"content"`
	LineStart   int               `json:"line_start"`  // First line number in this chunk
	LineEnd     int               `json:"line_end"`    // Last line number in this chunk
	Metadata    map[string]string `json:"metadata"`
}

// TriageCardV2 represents an analysis finding with chunk-aware context.
// Published to: destill.analysis.findings
// Key: {request_id}
type TriageCardV2 struct {
	// Identity
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	MessageHash string `json:"message_hash"`
	
	// Source
	Source   string `json:"source"`
	JobName  string `json:"job_name"`
	BuildURL string `json:"build_url"`
	
	// Content
	Severity        string   `json:"severity"`
	RawMessage      string   `json:"raw_message"`
	NormalizedMsg   string   `json:"normalized_message"`
	ConfidenceScore float64  `json:"confidence_score"`
	
	// Context (from chunk only - may be truncated)
	PreContext  []string `json:"pre_context"`   // Up to 15 lines before
	PostContext []string `json:"post_context"`  // Up to 30 lines after
	ContextNote string   `json:"context_note"`  // e.g., "truncated at chunk start"
	
	// Chunk info (for debugging/tracing)
	ChunkIndex  int `json:"chunk_index"`
	LineInChunk int `json:"line_in_chunk"`
	
	// Metadata
	Metadata  map[string]string `json:"metadata"`
	Timestamp string            `json:"timestamp"`
}

// AnalysisRequest represents a request to analyze a build.
// Published to: destill.requests
// Key: {request_id}
type AnalysisRequest struct {
	RequestID string `json:"request_id"`
	BuildURL  string `json:"build_url"`
	Timestamp string `json:"timestamp"`
}

// RequestStatus represents the status of an analysis request.
type RequestStatus struct {
	RequestID       string
	BuildURL        string
	Status          string // pending, processing, completed, failed
	ChunksTotal     int
	ChunksProcessed int
	FindingsCount   int
}

// TopicNames defines the Redpanda topic names used in the agentic architecture
const (
	// TopicLogsRaw contains raw log chunks (~500KB each)
	TopicLogsRaw = "destill.logs.raw"
	
	// TopicAnalysisFindings contains analysis findings (triage cards)
	TopicAnalysisFindings = "destill.analysis.findings"
	
	// TopicRequests contains build analysis requests
	TopicRequests = "destill.requests"
)

