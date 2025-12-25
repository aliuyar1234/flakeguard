#!/bin/bash
set -e

# FlakeGuard JUnit Upload Script
# This script is called by the GitHub Action to upload JUnit XML files to FlakeGuard

# Validate required environment variables
if [[ -z "$FLAKEGUARD_URL" ]]; then
  echo "Error: FLAKEGUARD_URL is required"
  exit 1
fi

if [[ -z "$PROJECT_SLUG" ]]; then
  echo "Error: PROJECT_SLUG is required"
  exit 1
fi

if [[ -z "$API_KEY" ]]; then
  echo "Error: API_KEY is required"
  exit 1
fi

if [[ -z "$JUNIT_PATHS" ]]; then
  echo "Error: JUNIT_PATHS is required"
  exit 1
fi

# Extract GitHub context metadata
REPOSITORY="${GITHUB_REPOSITORY:-}"
COMMIT_SHA="${GITHUB_SHA:-}"
RUN_ID="${GITHUB_RUN_ID:-}"
RUN_ATTEMPT="${GITHUB_RUN_ATTEMPT:-1}"
WORKFLOW_NAME="${GITHUB_WORKFLOW:-}"
JOB_NAME="${GITHUB_JOB:-}"

# Extract branch name from GITHUB_REF
# GITHUB_REF can be: refs/heads/main, refs/pull/123/merge, refs/tags/v1.0.0
BRANCH=""
if [[ -n "$GITHUB_REF" ]]; then
  if [[ "$GITHUB_REF" == refs/heads/* ]]; then
    BRANCH="${GITHUB_REF#refs/heads/}"
  elif [[ "$GITHUB_REF" == refs/pull/* ]]; then
    BRANCH="PR-${GITHUB_REF#refs/pull/}"
    BRANCH="${BRANCH%/merge}"
  elif [[ "$GITHUB_REF" == refs/tags/* ]]; then
    BRANCH="${GITHUB_REF#refs/tags/}"
  else
    BRANCH="$GITHUB_REF"
  fi
fi

# Find all JUnit XML files matching the glob pattern
echo "Searching for JUnit files matching pattern: $JUNIT_PATHS"

# Use find with globstar for pattern matching
shopt -s globstar nullglob
FILES=($JUNIT_PATHS)

# Check if any files were found
if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "Warning: No JUnit files found matching pattern '$JUNIT_PATHS'"
  echo "Skipping upload (this is not a failure)"
  exit 0
fi

echo "Found ${#FILES[@]} JUnit file(s)"

# Build multipart curl command
CURL_ARGS=()
CURL_ARGS+=("-X" "POST")
CURL_ARGS+=("-H" "Authorization: Bearer $API_KEY")

# Add metadata as form parameters
CURL_ARGS+=("-F" "project_slug=$PROJECT_SLUG")

if [[ -n "$REPOSITORY" ]]; then
  CURL_ARGS+=("-F" "repository=$REPOSITORY")
fi

if [[ -n "$COMMIT_SHA" ]]; then
  CURL_ARGS+=("-F" "commit_sha=$COMMIT_SHA")
fi

if [[ -n "$BRANCH" ]]; then
  CURL_ARGS+=("-F" "branch=$BRANCH")
fi

if [[ -n "$RUN_ID" ]]; then
  CURL_ARGS+=("-F" "run_id=$RUN_ID")
fi

if [[ -n "$RUN_ATTEMPT" ]]; then
  CURL_ARGS+=("-F" "run_attempt=$RUN_ATTEMPT")
fi

if [[ -n "$WORKFLOW_NAME" ]]; then
  CURL_ARGS+=("-F" "workflow_name=$WORKFLOW_NAME")
fi

if [[ -n "$JOB_NAME" ]]; then
  CURL_ARGS+=("-F" "job_name=$JOB_NAME")
fi

if [[ -n "$JOB_VARIANT" ]]; then
  CURL_ARGS+=("-F" "job_variant=$JOB_VARIANT")
fi

# Add each JUnit file
for file in "${FILES[@]}"; do
  if [[ -f "$file" ]]; then
    echo "Adding file: $file"
    CURL_ARGS+=("-F" "files=@$file")
  fi
done

# Construct API endpoint
API_ENDPOINT="${FLAKEGUARD_URL}/api/v1/ingest/junit"

echo "Uploading to: $API_ENDPOINT"

# Execute curl and capture response
HTTP_RESPONSE=$(mktemp)
HTTP_CODE=$(curl -w "%{http_code}" -o "$HTTP_RESPONSE" "${CURL_ARGS[@]}" "$API_ENDPOINT" 2>&1)
CURL_EXIT_CODE=$?

# Check for curl failure (network error, etc.)
if [[ $CURL_EXIT_CODE -ne 0 ]]; then
  echo "Error: curl command failed with exit code $CURL_EXIT_CODE"
  echo "Response: $(cat "$HTTP_RESPONSE")"
  rm -f "$HTTP_RESPONSE"
  exit 1
fi

# Extract HTTP status code (last 3 characters)
if [[ ${#HTTP_CODE} -ge 3 ]]; then
  STATUS_CODE="${HTTP_CODE: -3}"
else
  STATUS_CODE="$HTTP_CODE"
fi

# Read response body
RESPONSE_BODY=$(cat "$HTTP_RESPONSE")
rm -f "$HTTP_RESPONSE"

# Check HTTP status code
if [[ ! "$STATUS_CODE" =~ ^2[0-9][0-9]$ ]]; then
  echo "Error: Upload failed with HTTP status $STATUS_CODE"
  echo "Response body:"
  echo "$RESPONSE_BODY"
  exit 1
fi

# Parse and log ingestion summary
echo "Upload successful (HTTP $STATUS_CODE)"
echo "Response:"
echo "$RESPONSE_BODY"

# Try to extract summary from JSON response (if jq is available)
if command -v jq &> /dev/null; then
  FILES_INGESTED=$(echo "$RESPONSE_BODY" | jq -r '.files_ingested // empty' 2>/dev/null || echo "")
  TESTS_TOTAL=$(echo "$RESPONSE_BODY" | jq -r '.tests_total // empty' 2>/dev/null || echo "")

  if [[ -n "$FILES_INGESTED" ]] && [[ -n "$TESTS_TOTAL" ]]; then
    echo "Summary: Ingested $FILES_INGESTED file(s) containing $TESTS_TOTAL test(s)"
  fi
fi

echo "FlakeGuard upload completed successfully"
