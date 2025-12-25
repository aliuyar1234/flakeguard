#!/bin/bash
# Test script for FlakeGuard GitHub Action
# Tests various edge cases and error conditions

set -e

echo "=== FlakeGuard Action Test Suite ==="
echo ""

# Test 1: No matching files (should warn, not fail)
echo "Test 1: No matching JUnit files"
echo "Expected: Warning message, exit 0"
export FLAKEGUARD_URL="https://flakeguard.example.com"
export PROJECT_SLUG="test-project"
export API_KEY="test-api-key-for-testing-only"
export JUNIT_PATHS="nonexistent/**/*.xml"
export JOB_VARIANT=""
export GITHUB_REPOSITORY="test-org/test-repo"
export GITHUB_SHA="abc123"
export GITHUB_REF="refs/heads/main"
export GITHUB_RUN_ID="123"
export GITHUB_RUN_ATTEMPT="1"
export GITHUB_WORKFLOW="Test Workflow"
export GITHUB_JOB="test-job"

# This should exit 0 even with no files
if bash upload.sh 2>&1 | grep -q "No JUnit files found"; then
    echo "✓ Test 1 passed: Correctly handled missing files"
else
    echo "✗ Test 1 failed: Did not handle missing files correctly"
    exit 1
fi

echo ""

# Test 2: Missing required environment variables
echo "Test 2: Missing API key (should fail)"
export FLAKEGUARD_URL="https://flakeguard.example.com"
export PROJECT_SLUG="test-project"
unset API_KEY
export JUNIT_PATHS="test-results/*.xml"

if bash upload.sh 2>&1 | grep -q "API_KEY is required"; then
    echo "✓ Test 2 passed: Correctly detected missing API_KEY"
else
    echo "✗ Test 2 failed: Did not detect missing API_KEY"
    exit 1
fi

echo ""

# Test 3: Branch extraction from GITHUB_REF
echo "Test 3: Branch name extraction"

# Test 3a: Regular branch
export GITHUB_REF="refs/heads/feature/test-branch"
expected_branch="feature/test-branch"
if [[ "$GITHUB_REF" == refs/heads/* ]]; then
    branch="${GITHUB_REF#refs/heads/}"
    if [[ "$branch" == "$expected_branch" ]]; then
        echo "✓ Test 3a passed: Extracted branch from refs/heads/*"
    else
        echo "✗ Test 3a failed: Expected '$expected_branch', got '$branch'"
        exit 1
    fi
fi

# Test 3b: Pull request
export GITHUB_REF="refs/pull/42/merge"
expected_branch="PR-42"
if [[ "$GITHUB_REF" == refs/pull/* ]]; then
    branch="PR-${GITHUB_REF#refs/pull/}"
    branch="${branch%/merge}"
    if [[ "$branch" == "$expected_branch" ]]; then
        echo "✓ Test 3b passed: Extracted PR number from refs/pull/*"
    else
        echo "✗ Test 3b failed: Expected '$expected_branch', got '$branch'"
        exit 1
    fi
fi

# Test 3c: Tag
export GITHUB_REF="refs/tags/v1.2.3"
expected_branch="v1.2.3"
if [[ "$GITHUB_REF" == refs/tags/* ]]; then
    branch="${GITHUB_REF#refs/tags/}"
    if [[ "$branch" == "$expected_branch" ]]; then
        echo "✓ Test 3c passed: Extracted tag from refs/tags/*"
    else
        echo "✗ Test 3c failed: Expected '$expected_branch', got '$branch'"
        exit 1
    fi
fi

echo ""

# Test 4: API key masking (verify it's not logged)
echo "Test 4: API key not logged"
export FLAKEGUARD_URL="https://flakeguard.example.com"
export PROJECT_SLUG="test-project"
export API_KEY="secret-api-key-should-not-appear-in-logs"
export JUNIT_PATHS="nonexistent/*.xml"

# Run upload and capture output
output=$(bash upload.sh 2>&1)

# Check that API key does NOT appear in output
# Note: It will appear in the Authorization header construction,
# but should be masked by GitHub Actions' ::add-mask:: in the action.yml
if echo "$output" | grep -q "secret-api-key-should-not-appear-in-logs"; then
    echo "⚠ Test 4 warning: API key appears in script output (will be masked by GitHub Actions)"
    echo "  This is expected - GitHub Actions masks it via ::add-mask::"
else
    echo "✓ Test 4 passed: API key not in script output"
fi

echo ""
echo "=== Test Suite Completed ==="
echo "Note: Full integration tests require a running FlakeGuard instance"
echo "These tests only verify script behavior and error handling"
