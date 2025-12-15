// Package contracts defines message types for the Agentic Data Plane architecture.
package contracts

// LogChunk represents a chunk of log data for the agentic architecture.
// Published to: destill.logs.raw
// Key: {build_id}
type LogChunk struct {
	RequestID   string            `json:"request_id"`
	BuildID     string            `json:"build_id"`
	JobName     string            `json:"job_name"`
	JobID       string            `json:"job_id"`
	ChunkIndex  int               `json:"chunk_index"`
	TotalChunks int               `json:"total_chunks"`
	Content     string            `json:"content"`
	LineStart   int               `json:"line_start"` // First line number in this chunk
	LineEnd     int               `json:"line_end"`   // Last line number in this chunk
	Metadata    map[string]string `json:"metadata"`
}

// TriageCard represents an analysis finding with chunk-aware context.
// Published to: destill.analysis.findings
// Key: {request_id}
type TriageCard struct {
	// Identity
	ID          string `json:"id"`
	RequestID   string `json:"request_id"`
	MessageHash string `json:"message_hash"`

	// Source
	Source   string `json:"source"`
	JobName  string `json:"job_name"`
	BuildURL string `json:"build_url"`

	// Content
	Severity        string  `json:"severity"`
	RawMessage      string  `json:"raw_message"`
	NormalizedMsg   string  `json:"normalized_message"`
	ConfidenceScore float64 `json:"confidence_score"`

	// Context (from chunk only - may be truncated)
	PreContext  []string `json:"pre_context"`  // Up to 15 lines before
	PostContext []string `json:"post_context"` // Up to 30 lines after
	ContextNote string   `json:"context_note"` // e.g., "truncated at chunk start"

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

// ProgressUpdate represents progress during build analysis.
// Published to: destill.progress
// Key: {request_id}
type ProgressUpdate struct {
	RequestID string `json:"request_id"`
	Stage     string `json:"stage"`   // e.g., "Downloading build metadata", "Fetching logs"
	Current   int    `json:"current"` // Current item number (0 if not applicable)
	Total     int    `json:"total"`   // Total items (0 if not applicable)
	Timestamp string `json:"timestamp"`
}

// TopicNames defines the Redpanda topic names used in the agentic architecture
const (
	// TopicLogsRaw contains raw log chunks (~500KB each)
	TopicLogsRaw = "destill.logs.raw"

	// TopicAnalysisFindings contains analysis findings (triage cards)
	TopicAnalysisFindings = "destill.analysis.findings"

	// TopicRequests contains build analysis requests
	TopicRequests = "destill.requests"

	// TopicProgress contains progress updates during analysis
	TopicProgress = "destill.progress"
)
