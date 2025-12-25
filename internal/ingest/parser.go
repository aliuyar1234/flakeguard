package ingest

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// JUnitTestSuites represents the root XML element containing test suites
type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

// JUnitTestSuite represents a single test suite
type JUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      float64         `xml:"time,attr"`
	TestCases []JUnitTestCase `xml:"testcase"`
}

// JUnitTestCase represents a single test case
type JUnitTestCase struct {
	XMLName   xml.Name          `xml:"testcase"`
	Classname string            `xml:"classname,attr"`
	Name      string            `xml:"name,attr"`
	Time      float64           `xml:"time,attr"`
	Failure   *JUnitFailure     `xml:"failure"`
	Error     *JUnitError       `xml:"error"`
	Skipped   *JUnitSkipped     `xml:"skipped"`
}

// JUnitFailure represents a test failure
type JUnitFailure struct {
	XMLName xml.Name `xml:"failure"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
	Content string   `xml:",chardata"`
}

// JUnitError represents a test error
type JUnitError struct {
	XMLName xml.Name `xml:"error"`
	Message string   `xml:"message,attr"`
	Type    string   `xml:"type,attr"`
	Content string   `xml:",chardata"`
}

// JUnitSkipped represents a skipped test
type JUnitSkipped struct {
	XMLName xml.Name `xml:"skipped"`
	Message string   `xml:"message,attr"`
}

// TestResult represents a parsed test result
type TestResult struct {
	TestIdentifier string
	Classname      string
	Name           string
	Status         string // "passed", "failed", "skipped", "error"
	DurationMS     int
	FailureMessage string
	FailureOutput  string
}

// ParseJUnitXML parses JUnit XML from a reader
func ParseJUnitXML(r io.Reader) (*JUnitTestSuites, error) {
	var suites JUnitTestSuites
	decoder := xml.NewDecoder(r)

	if err := decoder.Decode(&suites); err != nil {
		return nil, fmt.Errorf("failed to parse JUnit XML: %w", err)
	}

	return &suites, nil
}

// ExtractTestResults extracts all test results from parsed JUnit XML
func ExtractTestResults(suites *JUnitTestSuites) []TestResult {
	var results []TestResult

	for _, suite := range suites.TestSuites {
		for _, testCase := range suite.TestCases {
			result := extractTestResult(&testCase)
			results = append(results, result)
		}
	}

	return results
}

// extractTestResult converts a JUnit test case to a TestResult
func extractTestResult(tc *JUnitTestCase) TestResult {
	result := TestResult{
		Classname:      tc.Classname,
		Name:           tc.Name,
		TestIdentifier: deriveTestIdentifier(tc.Classname, tc.Name),
		DurationMS:     int(tc.Time * 1000), // Convert seconds to milliseconds
	}

	// Determine status and extract failure information
	if tc.Error != nil {
		result.Status = "error"
		result.FailureMessage = truncateString(tc.Error.Message, 1024)
		result.FailureOutput = truncateString(tc.Error.Content, 8192)
	} else if tc.Failure != nil {
		result.Status = "failed"
		result.FailureMessage = truncateString(tc.Failure.Message, 1024)
		result.FailureOutput = truncateString(tc.Failure.Content, 8192)
	} else if tc.Skipped != nil {
		result.Status = "skipped"
	} else {
		result.Status = "passed"
	}

	return result
}

// deriveTestIdentifier creates a test identifier from classname and name
// Format: classname#name
func deriveTestIdentifier(classname, name string) string {
	return fmt.Sprintf("%s#%s", classname, name)
}

// truncateString truncates a string to maxBytes and adds a truncation indicator
func truncateString(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	truncated := s[:maxBytes]
	indicator := "\n... [truncated]"

	// Make sure we have room for the indicator
	if len(truncated)+len(indicator) > maxBytes {
		truncated = truncated[:maxBytes-len(indicator)]
	}

	return truncated + indicator
}

// ParseAndExtract is a convenience function that parses JUnit XML and extracts results
func ParseAndExtract(r io.Reader) ([]TestResult, error) {
	suites, err := ParseJUnitXML(r)
	if err != nil {
		return nil, err
	}

	return ExtractTestResults(suites), nil
}

// NormalizeEventType converts event_type to CI event enum
func NormalizeEventType(eventType string) string {
	eventType = strings.ToLower(eventType)

	switch eventType {
	case "push":
		return "push"
	case "pull_request":
		return "pull_request"
	case "workflow_dispatch":
		return "workflow_dispatch"
	case "schedule":
		return "schedule"
	default:
		return "other"
	}
}
