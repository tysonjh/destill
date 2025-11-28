// Package contracts defines the interfaces and data structures for inter-agent communication.
package contracts

// TriageCard represents a log entry that has been ingested and is ready for analysis.
type TriageCard struct {
	// ID is a unique identifier for the triage card.
	ID string
	// Source indicates where the log entry originated.
	Source string
	// Timestamp is the time when the log was created.
	Timestamp string
	// Severity represents the log level (e.g., INFO, WARN, ERROR).
	Severity string
	// Message contains the log message content.
	Message string
	// Metadata holds additional key-value pairs associated with the log.
	Metadata map[string]string
}

// MessageBroker defines the interface for communication between agents.
// Implementations of this interface handle the publishing and subscribing of TriageCards.
type MessageBroker interface {
	// Publish sends a TriageCard to the specified topic.
	Publish(topic string, card TriageCard) error
	// Subscribe registers a handler function for messages on the specified topic.
	Subscribe(topic string, handler func(TriageCard) error) error
	// Close shuts down the broker connection gracefully.
	Close() error
}
