#!/usr/bin/env bash
set -euo pipefail

echo "=== FlakeGuard Action Test Suite ==="

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export FLAKEGUARD_URL="https://flakeguard.example.com"
export PROJECT_SLUG="test-project"
export API_KEY="test-api-key-for-testing-only"
export JOB_VARIANT=""

export GITHUB_REPOSITORY="test-org/test-repo"
export GITHUB_SHA="abc123"
export GITHUB_REF="refs/heads/main"
export GITHUB_REF_NAME="main"
export GITHUB_RUN_ID="123"
export GITHUB_RUN_ATTEMPT="1"
export GITHUB_RUN_NUMBER="42"
export GITHUB_WORKFLOW="Test Workflow"
export GITHUB_WORKFLOW_REF="test-org/test-repo/.github/workflows/ci.yml@refs/heads/main"
export GITHUB_JOB="test"
export GITHUB_EVENT_NAME="push"
export GITHUB_SERVER_URL="https://github.com"

echo ""
echo "Test 1: No matching files (should warn, exit 0)"
export JUNIT_PATHS="nonexistent/**/*.xml"
if bash "$SCRIPT_DIR/upload.sh" 2>&1 | grep -q "No JUnit files found"; then
  echo "OK: no matching files handled"
else
  echo "FAIL: expected missing-files warning"
  exit 1
fi

echo ""
echo "Test 2: Missing API_KEY (should fail)"
unset API_KEY
export JUNIT_PATHS="test-results/**/*.xml"
if bash "$SCRIPT_DIR/upload.sh" 2>&1 | grep -q "API_KEY is required"; then
  echo "OK: missing API_KEY detected"
else
  echo "FAIL: expected missing API_KEY error"
  exit 1
fi

echo ""
echo "=== Test Suite Completed ==="
echo "Note: Full integration tests require a running FlakeGuard instance."
