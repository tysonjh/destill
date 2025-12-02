// Package contracts defines the interfaces and data structures for inter-agent communication.
package contracts

// TriageCard represents a log entry that has been ingested and is ready for analysis.
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

// MessageBroker defines the interface for communication between agents.
// Implementations simulate a Kafka-like API for local development.
type MessageBroker interface {
	// Publish sends a message to the specified topic.
	Publish(topic string, message []byte) error
	// Subscribe returns a channel for receiving messages from the specified topic.
	Subscribe(topic string) (<-chan []byte, error)
	// Close shuts down the broker connection gracefully.
	Close() error
}
