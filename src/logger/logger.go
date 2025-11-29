package logger

import (
	"fmt"
	"os"
)

// Logger defines the interface for logging throughout the application.
// Different implementations can be used for different contexts (console, silent, structured, etc.)
type Logger interface {
	Info(msg string, args ...interface{})
	Error(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
}

// ConsoleLogger writes human-readable logs to stdout/stderr.
// Used for normal operation and debugging.
type ConsoleLogger struct{}

func NewConsoleLogger() *ConsoleLogger {
	return &ConsoleLogger{}
}

func (c *ConsoleLogger) Info(msg string, args ...interface{}) {
	fmt.Printf("[INFO] "+msg+"\n", args...)
}

func (c *ConsoleLogger) Error(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+msg+"\n", args...)
}

func (c *ConsoleLogger) Debug(msg string, args ...interface{}) {
	fmt.Printf("[DEBUG] "+msg+"\n", args...)
}

// SilentLogger discards all log messages.
// Used when running in TUI mode to prevent log output from interfering with the display.
type SilentLogger struct{}

func NewSilentLogger() *SilentLogger {
	return &SilentLogger{}
}

func (s *SilentLogger) Info(msg string, args ...interface{})  {}
func (s *SilentLogger) Error(msg string, args ...interface{}) {}
func (s *SilentLogger) Debug(msg string, args ...interface{}) {}
