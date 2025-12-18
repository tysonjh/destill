package ingest

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// JUnitTestSuites represents a collection of test suites (top-level element in some formats).
type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single test suite.
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
	// Nested test suites (some formats nest them)
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestCase represents a single test case.
type JUnitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure"`
	Error     *JUnitError   `xml:"error"`
	Skipped   *JUnitSkipped `xml:"skipped"`
}

// JUnitFailure represents a test failure.
type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitError represents a test error.
type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Content string `xml:",chardata"`
}

// JUnitSkipped represents a skipped test.
type JUnitSkipped struct {
	Message string `xml:"message,attr"`
}

// ParsedTestResult represents a parsed test result from JUnit XML.
type ParsedTestResult struct {
	TestName       string  // Full name: ClassName.Name
	ClassName      string
	Name           string
	Passed         bool
	Skipped        bool
	FailureMessage string
	Duration       float64 // seconds
}

// ParseJUnitXML parses JUnit XML content and returns test results.
// Returns nil, nil if the content is not valid JUnit XML.
func ParseJUnitXML(data []byte) ([]ParsedTestResult, error) {
	// Try parsing as testsuites (multiple suites)
	var testSuites JUnitTestSuites
	if err := xml.Unmarshal(data, &testSuites); err == nil && len(testSuites.TestSuites) > 0 {
		return extractFromTestSuites(testSuites.TestSuites), nil
	}

	// Try parsing as single testsuite
	var testSuite JUnitTestSuite
	if err := xml.Unmarshal(data, &testSuite); err == nil && (len(testSuite.TestCases) > 0 || len(testSuite.TestSuites) > 0) {
		return extractFromTestSuites([]JUnitTestSuite{testSuite}), nil
	}

	// Not valid JUnit XML
	return nil, nil
}

// extractFromTestSuites recursively extracts test results from test suites.
func extractFromTestSuites(suites []JUnitTestSuite) []ParsedTestResult {
	var results []ParsedTestResult

	for _, suite := range suites {
		// Extract test cases from this suite
		for _, tc := range suite.TestCases {
			result := ParsedTestResult{
				ClassName: tc.ClassName,
				Name:      tc.Name,
				Duration:  parseTime(tc.Time),
			}

			// Build full test name
			if tc.ClassName != "" && tc.Name != "" {
				result.TestName = tc.ClassName + "." + tc.Name
			} else if tc.ClassName != "" {
				result.TestName = tc.ClassName
			} else if tc.Name != "" {
				result.TestName = tc.Name
			}

			// Determine pass/fail/skip status
			if tc.Skipped != nil {
				result.Skipped = true
				result.Passed = true // Skipped tests aren't failures
				if tc.Skipped.Message != "" {
					result.FailureMessage = tc.Skipped.Message
				}
			} else if tc.Failure != nil {
				result.Passed = false
				result.FailureMessage = buildFailureMessage(tc.Failure.Message, tc.Failure.Type, tc.Failure.Content)
			} else if tc.Error != nil {
				result.Passed = false
				result.FailureMessage = buildFailureMessage(tc.Error.Message, tc.Error.Type, tc.Error.Content)
			} else {
				result.Passed = true
			}

			results = append(results, result)
		}

		// Recursively process nested test suites
		if len(suite.TestSuites) > 0 {
			nested := extractFromTestSuites(suite.TestSuites)
			results = append(results, nested...)
		}
	}

	return results
}

// buildFailureMessage constructs a failure message from available fields.
func buildFailureMessage(message, failureType, content string) string {
	var parts []string

	if failureType != "" {
		parts = append(parts, failureType)
	}
	if message != "" {
		parts = append(parts, message)
	}

	msg := strings.Join(parts, ": ")

	// Include first few lines of content if message is short
	if len(msg) < 100 && content != "" {
		content = strings.TrimSpace(content)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 {
			// Take first line of stack trace if available
			firstLine := strings.TrimSpace(lines[0])
			if firstLine != "" && firstLine != msg {
				if msg != "" {
					msg = msg + " - " + firstLine
				} else {
					msg = firstLine
				}
			}
		}
	}

	// Truncate very long messages
	if len(msg) > 500 {
		msg = msg[:497] + "..."
	}

	return msg
}

// parseTime parses a time string to float64 seconds.
func parseTime(s string) float64 {
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// IsXMLFile checks if a filename looks like an XML file.
func IsXMLFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".xml")
}

// SummarizeResults creates summary statistics from parsed results.
func SummarizeResults(results []ParsedTestResult) (total, passed, failed, skipped int) {
	for _, r := range results {
		total++
		if r.Skipped {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

// FormatTestResultsSummary formats a human-readable summary of test results.
func FormatTestResultsSummary(results []ParsedTestResult) string {
	total, passed, failed, skipped := SummarizeResults(results)
	if total == 0 {
		return "No tests found"
	}
	return fmt.Sprintf("%d tests: %d passed, %d failed, %d skipped", total, passed, failed, skipped)
}
