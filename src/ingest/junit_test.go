package ingest

import (
	"testing"
)

func TestParseJUnitXML_SingleTestSuite(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="MyTests" tests="3" failures="1" errors="0" skipped="1" time="1.5">
  <testcase classname="com.example.FooTest" name="testSuccess" time="0.5"/>
  <testcase classname="com.example.FooTest" name="testFailure" time="0.8">
    <failure message="Expected 3 but got 5" type="AssertionError">
      at com.example.FooTest.testFailure(FooTest.java:42)
    </failure>
  </testcase>
  <testcase classname="com.example.FooTest" name="testSkipped" time="0">
    <skipped message="Not implemented yet"/>
  </testcase>
</testsuite>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Check passed test
	if results[0].TestName != "com.example.FooTest.testSuccess" {
		t.Errorf("wrong test name: %s", results[0].TestName)
	}
	if !results[0].Passed {
		t.Error("testSuccess should be passed")
	}
	if results[0].Duration != 0.5 {
		t.Errorf("wrong duration: %f", results[0].Duration)
	}

	// Check failed test
	if results[1].TestName != "com.example.FooTest.testFailure" {
		t.Errorf("wrong test name: %s", results[1].TestName)
	}
	if results[1].Passed {
		t.Error("testFailure should not be passed")
	}
	if results[1].FailureMessage == "" {
		t.Error("testFailure should have failure message")
	}

	// Check skipped test
	if !results[2].Skipped {
		t.Error("testSkipped should be skipped")
	}
	if !results[2].Passed {
		t.Error("skipped tests should count as passed (not failures)")
	}
}

func TestParseJUnitXML_TestSuites(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="Suite1" tests="1">
    <testcase classname="Suite1Test" name="test1" time="0.1"/>
  </testsuite>
  <testsuite name="Suite2" tests="1">
    <testcase classname="Suite2Test" name="test2" time="0.2"/>
  </testsuite>
</testsuites>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].TestName != "Suite1Test.test1" {
		t.Errorf("wrong test name: %s", results[0].TestName)
	}
	if results[1].TestName != "Suite2Test.test2" {
		t.Errorf("wrong test name: %s", results[1].TestName)
	}
}

func TestParseJUnitXML_NestedSuites(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="ParentSuite">
  <testsuite name="ChildSuite" tests="1">
    <testcase classname="ChildTest" name="test1" time="0.1"/>
  </testsuite>
</testsuite>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].TestName != "ChildTest.test1" {
		t.Errorf("wrong test name: %s", results[0].TestName)
	}
}

func TestParseJUnitXML_ErrorElement(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="ErrorTests" tests="1">
  <testcase classname="ErrorTest" name="testError" time="0.1">
    <error message="NullPointerException" type="java.lang.NullPointerException">
      at ErrorTest.testError(ErrorTest.java:10)
    </error>
  </testcase>
</testsuite>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Passed {
		t.Error("error test should not be passed")
	}
	if results[0].FailureMessage == "" {
		t.Error("error test should have failure message")
	}
}

func TestParseJUnitXML_NotJUnit(t *testing.T) {
	// Valid XML but not JUnit format
	xml := `<?xml version="1.0"?><root><item>value</item></root>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("should not error on non-JUnit XML: %v", err)
	}
	if results != nil {
		t.Error("should return nil for non-JUnit XML")
	}
}

func TestParseJUnitXML_InvalidXML(t *testing.T) {
	xml := `not xml at all`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("should not error on invalid XML: %v", err)
	}
	if results != nil {
		t.Error("should return nil for invalid XML")
	}
}

func TestParseJUnitXML_PythonFormat(t *testing.T) {
	// Python pytest format (no classname, just name)
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="pytest" tests="2">
  <testcase name="test_something" time="0.1"/>
  <testcase classname="tests.test_module" name="test_other" time="0.2">
    <failure message="assert False">
E       assert False
    </failure>
  </testcase>
</testsuite>`

	results, err := ParseJUnitXML([]byte(xml))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// First test has no classname
	if results[0].TestName != "test_something" {
		t.Errorf("wrong test name for no-classname case: %s", results[0].TestName)
	}

	// Second test has classname
	if results[1].TestName != "tests.test_module.test_other" {
		t.Errorf("wrong test name: %s", results[1].TestName)
	}
	if results[1].Passed {
		t.Error("test_other should not be passed")
	}
}

func TestIsXMLFile(t *testing.T) {
	tests := []struct {
		filename string
		want     bool
	}{
		{"report.xml", true},
		{"REPORT.XML", true},
		{"test.xml.gz", false},
		{"test.json", false},
		{"", false},
		{"results.junit.xml", true},
	}

	for _, tt := range tests {
		got := IsXMLFile(tt.filename)
		if got != tt.want {
			t.Errorf("IsXMLFile(%q) = %v, want %v", tt.filename, got, tt.want)
		}
	}
}

func TestBuildFailureMessage_Truncation(t *testing.T) {
	// Very long content should be truncated
	longContent := ""
	for i := 0; i < 100; i++ {
		longContent += "This is a very long line that goes on and on.\n"
	}

	msg := buildFailureMessage("", "", longContent)
	if len(msg) > 510 {
		t.Errorf("message not truncated: len=%d", len(msg))
	}
	if len(msg) >= 500 && msg[len(msg)-3:] != "..." {
		t.Error("truncated message should end with ...")
	}
}
