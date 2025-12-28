package ingest

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func junitFixturePath(t *testing.T, name string) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)

	return filepath.Join(filepath.Dir(currentFile), "..", "..", "testdata", "junit", name)
}

func TestParseAndExtract_PassingXML(t *testing.T) {
	b, err := os.ReadFile(junitFixturePath(t, "passing.xml"))
	require.NoError(t, err)

	results, err := ParseAndExtract(bytes.NewReader(b))
	require.NoError(t, err)
	require.Len(t, results, 3)

	for _, r := range results {
		require.Equal(t, "passed", r.Status)
		require.Empty(t, r.FailureMessage)
		require.Empty(t, r.FailureOutput)
		require.NotEmpty(t, r.TestIdentifier)
		require.NotEmpty(t, r.Classname)
		require.NotEmpty(t, r.Name)
	}
}

func TestParseAndExtract_FailingXML(t *testing.T) {
	b, err := os.ReadFile(junitFixturePath(t, "failing.xml"))
	require.NoError(t, err)

	results, err := ParseAndExtract(bytes.NewReader(b))
	require.NoError(t, err)
	require.Len(t, results, 2)

	for _, r := range results {
		require.Equal(t, "failed", r.Status)
		require.NotEmpty(t, r.FailureMessage)
		require.NotEmpty(t, r.FailureOutput)
		require.NotEmpty(t, r.TestIdentifier)
	}
}

func TestParseAndExtract_FlakyAttemptPair(t *testing.T) {
	a1, err := os.ReadFile(junitFixturePath(t, "flaky_attempt1.xml"))
	require.NoError(t, err)
	a2, err := os.ReadFile(junitFixturePath(t, "flaky_attempt2.xml"))
	require.NoError(t, err)

	r1, err := ParseAndExtract(bytes.NewReader(a1))
	require.NoError(t, err)
	require.Len(t, r1, 1)
	r2, err := ParseAndExtract(bytes.NewReader(a2))
	require.NoError(t, err)
	require.Len(t, r2, 1)

	require.Equal(t, r1[0].TestIdentifier, r2[0].TestIdentifier)
	require.Equal(t, "failed", r1[0].Status)
	require.Equal(t, "passed", r2[0].Status)
}

func TestExtractTestResult_Truncation(t *testing.T) {
	longMsg := strings.Repeat("a", 2000)
	longOut := strings.Repeat("b", 9000)
	xmlDoc := fmt.Sprintf(
		`<testsuites><testsuite name="t"><testcase classname="c" name="n" time="0.1"><failure message="%s">%s</failure></testcase></testsuite></testsuites>`,
		longMsg,
		longOut,
	)

	results, err := ParseAndExtract(strings.NewReader(xmlDoc))
	require.NoError(t, err)
	require.Len(t, results, 1)

	require.Equal(t, "failed", results[0].Status)
	require.Len(t, results[0].FailureMessage, 1024)
	require.Contains(t, results[0].FailureMessage, "[truncated]")
	require.Len(t, results[0].FailureOutput, 8192)
	require.Contains(t, results[0].FailureOutput, "[truncated]")
	require.Equal(t, 100, results[0].DurationMS)
}
