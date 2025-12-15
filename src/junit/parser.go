// Package junit provides JUnit XML parsing for test result analysis.
package junit

import (
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"strings"
)

// TestSuites is the root element for multiple test suites.
type TestSuites struct {
	XMLName    xml.Name    `xml:"testsuites"`
	TestSuites []TestSuite `xml:"testsuite"`
}

// TestSuite represents a <testsuite> element.
type TestSuite struct {
	Name      string     `xml:"name,attr"`
	Tests     int        `xml:"tests,attr"`
	Failures  int        `xml:"failures,attr"`
	Errors    int        `xml:"errors,attr"`
	Skipped   int        `xml:"skipped,attr"`
	Time      float64    `xml:"time,attr"`
	TestCases []TestCase `xml:"testcase"`
}

// TestCase represents a <testcase> element.
type TestCase struct {
	Name      string   `xml:"name,attr"`
	ClassName string   `xml:"classname,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   *Failure `xml:"failure"`
	Error     *Error   `xml:"error"`
	Skipped   *Skipped `xml:"skipped"`
}

// Failure represents a test failure.
type Failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// Error represents a test error.
type Error struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// Skipped represents a skipped test.
type Skipped struct {
	Message string `xml:"message,attr"`
}

// TestFailure represents a parsed test failure for creating findings.
type TestFailure struct {
	TestName   string
	ClassName  string
	SuiteName  string
	Message    string
	Type       string // "failure" or "error"
	StackTrace string
	Duration   float64
}

// Parse parses JUnit XML data and returns only test failures and errors.
// Returns an empty slice if all tests passed.
func Parse(data []byte) ([]TestFailure, error) {
	// Try parsing as <testsuites> (multiple suites) first
	var suites TestSuites
	if err := xml.Unmarshal(data, &suites); err == nil && len(suites.TestSuites) > 0 {
		return extractFailures(suites.TestSuites), nil
	}

	// Try parsing as single <testsuite>
	var suite TestSuite
	if err := xml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to parse JUnit XML: %w", err)
	}

	return extractFailures([]TestSuite{suite}), nil
}

// extractFailures extracts failures and errors from test suites.
func extractFailures(suites []TestSuite) []TestFailure {
	var failures []TestFailure

	for _, suite := range suites {
		for _, testCase := range suite.TestCases {
			// Check for failure
			if testCase.Failure != nil {
				failures = append(failures, TestFailure{
					TestName:   testCase.Name,
					ClassName:  testCase.ClassName,
					SuiteName:  suite.Name,
					Message:    testCase.Failure.Message,
					Type:       "failure",
					StackTrace: strings.TrimSpace(testCase.Failure.Content),
					Duration:   testCase.Time,
				})
			}

			// Check for error
			if testCase.Error != nil {
				failures = append(failures, TestFailure{
					TestName:   testCase.Name,
					ClassName:  testCase.ClassName,
					SuiteName:  suite.Name,
					Message:    testCase.Error.Message,
					Type:       "error",
					StackTrace: strings.TrimSpace(testCase.Error.Content),
					Duration:   testCase.Time,
				})
			}
		}
	}

	return failures
}

// GenerateHash creates a deterministic hash for the test failure.
// This allows grouping the same test failure across different builds.
func (tf *TestFailure) GenerateHash() string {
	// Use class name + test name + message for hash
	// This groups identical test failures together
	key := fmt.Sprintf("%s::%s::%s", tf.ClassName, tf.TestName, tf.Message)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash[:8]) // First 16 hex chars
}

// GetDisplayMessage returns a human-readable message for the failure.
func (tf *TestFailure) GetDisplayMessage() string {
	if tf.Message != "" {
		return fmt.Sprintf("[%s] %s.%s: %s", tf.Type, tf.ClassName, tf.TestName, tf.Message)
	}
	return fmt.Sprintf("[%s] %s.%s", tf.Type, tf.ClassName, tf.TestName)
}

// GetNormalizedName returns a normalized identifier for the test.
// This is used for grouping and searching.
func (tf *TestFailure) GetNormalizedName() string {
	if tf.ClassName != "" {
		return fmt.Sprintf("%s::%s", tf.ClassName, tf.TestName)
	}
	return tf.TestName
}

// SplitStackTrace splits the stack trace into lines, limiting to maxLines.
func (tf *TestFailure) SplitStackTrace(maxLines int) []string {
	if tf.StackTrace == "" {
		return []string{}
	}

	lines := strings.Split(tf.StackTrace, "\n")

	// Trim empty lines from start and end
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := len(lines)
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	lines = lines[start:end]

	// Limit to maxLines
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	return lines
}
