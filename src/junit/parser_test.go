package junit

import (
	"strings"
	"testing"
)

func TestParse_SingleSuiteWithFailure(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="MyTestSuite" tests="2" failures="1" errors="0" skipped="0" time="1.234">
  <testcase name="testSuccess" classname="com.example.MyTest" time="0.123"/>
  <testcase name="testFailure" classname="com.example.MyTest" time="1.111">
    <failure message="assertion failed" type="AssertionError">
at com.example.MyTest.testFailure(MyTest.java:42)
    </failure>
  </testcase>
</testsuite>`

	failures, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(failures) != 1 {
		t.Fatalf("Expected 1 failure, got %d", len(failures))
	}

	failure := failures[0]
	if failure.TestName != "testFailure" {
		t.Errorf("Expected test name 'testFailure', got '%s'", failure.TestName)
	}
	if failure.ClassName != "com.example.MyTest" {
		t.Errorf("Expected class name 'com.example.MyTest', got '%s'", failure.ClassName)
	}
	if failure.Message != "assertion failed" {
		t.Errorf("Expected message 'assertion failed', got '%s'", failure.Message)
	}
	if failure.Type != "failure" {
		t.Errorf("Expected type 'failure', got '%s'", failure.Type)
	}
	if failure.Duration != 1.111 {
		t.Errorf("Expected duration 1.111, got %f", failure.Duration)
	}
}

func TestParse_MultipleSuitesWithErrors(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="Suite1" tests="1" failures="0" errors="1" skipped="0" time="0.5">
    <testcase name="testError" classname="com.example.Test1" time="0.5">
      <error message="NullPointerException" type="NullPointerException">
Stack trace here
      </error>
    </testcase>
  </testsuite>
  <testsuite name="Suite2" tests="1" failures="1" errors="0" skipped="0" time="0.3">
    <testcase name="testFail" classname="com.example.Test2" time="0.3">
      <failure message="expected true" type="AssertionError"/>
    </testcase>
  </testsuite>
</testsuites>`

	failures, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(failures) != 2 {
		t.Fatalf("Expected 2 failures, got %d", len(failures))
	}

	// First should be error
	if failures[0].Type != "error" {
		t.Errorf("Expected first failure type 'error', got '%s'", failures[0].Type)
	}
	if failures[0].SuiteName != "Suite1" {
		t.Errorf("Expected suite name 'Suite1', got '%s'", failures[0].SuiteName)
	}

	// Second should be failure
	if failures[1].Type != "failure" {
		t.Errorf("Expected second failure type 'failure', got '%s'", failures[1].Type)
	}
	if failures[1].SuiteName != "Suite2" {
		t.Errorf("Expected suite name 'Suite2', got '%s'", failures[1].SuiteName)
	}
}

func TestParse_AllPassing(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="PassingSuite" tests="3" failures="0" errors="0" skipped="0" time="1.0">
  <testcase name="test1" classname="com.example.Test" time="0.3"/>
  <testcase name="test2" classname="com.example.Test" time="0.3"/>
  <testcase name="test3" classname="com.example.Test" time="0.4"/>
</testsuite>`

	failures, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(failures) != 0 {
		t.Errorf("Expected 0 failures for all-passing suite, got %d", len(failures))
	}
}

func TestParse_InvalidXML(t *testing.T) {
	xml := `not even xml at all`

	_, err := Parse([]byte(xml))
	if err == nil {
		t.Error("Expected error for invalid XML, got nil")
	}
}

func TestGenerateHash(t *testing.T) {
	failure1 := TestFailure{
		TestName:  "testFoo",
		ClassName: "com.example.MyTest",
		Message:   "assertion failed",
	}

	failure2 := TestFailure{
		TestName:  "testFoo",
		ClassName: "com.example.MyTest",
		Message:   "assertion failed",
	}

	failure3 := TestFailure{
		TestName:  "testBar",
		ClassName: "com.example.MyTest",
		Message:   "assertion failed",
	}

	hash1 := failure1.GenerateHash()
	hash2 := failure2.GenerateHash()
	hash3 := failure3.GenerateHash()

	if hash1 != hash2 {
		t.Error("Expected identical failures to have same hash")
	}

	if hash1 == hash3 {
		t.Error("Expected different failures to have different hashes")
	}

	if len(hash1) != 16 {
		t.Errorf("Expected hash length 16, got %d", len(hash1))
	}
}

func TestGetDisplayMessage(t *testing.T) {
	tests := []struct {
		name     string
		failure  TestFailure
		expected string
	}{
		{
			name: "with message",
			failure: TestFailure{
				Type:      "failure",
				ClassName: "com.example.Test",
				TestName:  "testFoo",
				Message:   "expected true",
			},
			expected: "[failure] com.example.Test.testFoo: expected true",
		},
		{
			name: "without message",
			failure: TestFailure{
				Type:      "error",
				ClassName: "com.example.Test",
				TestName:  "testBar",
				Message:   "",
			},
			expected: "[error] com.example.Test.testBar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.failure.GetDisplayMessage()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestGetNormalizedName(t *testing.T) {
	tests := []struct {
		name     string
		failure  TestFailure
		expected string
	}{
		{
			name: "with class name",
			failure: TestFailure{
				ClassName: "com.example.Test",
				TestName:  "testFoo",
			},
			expected: "com.example.Test::testFoo",
		},
		{
			name: "without class name",
			failure: TestFailure{
				ClassName: "",
				TestName:  "testBar",
			},
			expected: "testBar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.failure.GetNormalizedName()
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestSplitStackTrace(t *testing.T) {
	failure := TestFailure{
		StackTrace: `
at com.example.Test.method1(Test.java:10)
at com.example.Test.method2(Test.java:20)
at com.example.Test.method3(Test.java:30)
at com.example.Test.method4(Test.java:40)
at com.example.Test.method5(Test.java:50)
`,
	}

	lines := failure.SplitStackTrace(3)
	if len(lines) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(lines))
	}

	if !strings.Contains(lines[0], "method1") {
		t.Error("Expected first line to contain 'method1'")
	}
}

