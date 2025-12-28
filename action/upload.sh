#!/usr/bin/env bash
set -euo pipefail

echo "FlakeGuard: preparing JUnit upload"

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "Error: $name is required" >&2
    exit 1
  fi
}

require_env FLAKEGUARD_URL
require_env PROJECT_SLUG
require_env API_KEY
require_env JUNIT_PATHS

STARTED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

REPO_FULL_NAME="${GITHUB_REPOSITORY:-}"
SHA="${GITHUB_SHA:-}"
GITHUB_RUN_ID_VALUE="${GITHUB_RUN_ID:-}"
GITHUB_RUN_ATTEMPT_VALUE="${GITHUB_RUN_ATTEMPT:-}"
GITHUB_RUN_NUMBER_VALUE="${GITHUB_RUN_NUMBER:-}"
WORKFLOW_NAME="${GITHUB_WORKFLOW:-}"
WORKFLOW_REF_RAW="${GITHUB_WORKFLOW_REF:-}"
JOB_NAME="${GITHUB_JOB:-}"
EVENT_NAME="${GITHUB_EVENT_NAME:-}"
JOB_VARIANT="${JOB_VARIANT:-}"

GITHUB_REF_VALUE="${GITHUB_REF:-}"
GITHUB_REF_NAME_VALUE="${GITHUB_REF_NAME:-}"
GITHUB_HEAD_REF_VALUE="${GITHUB_HEAD_REF:-}"
GITHUB_SERVER_URL_VALUE="${GITHUB_SERVER_URL:-https://github.com}"

if [[ -z "$REPO_FULL_NAME" ]]; then
  echo "Error: GITHUB_REPOSITORY is required (repo_full_name)" >&2
  exit 1
fi
if [[ -z "$WORKFLOW_NAME" ]]; then
  echo "Error: GITHUB_WORKFLOW is required (workflow_name)" >&2
  exit 1
fi
if [[ -z "$JOB_NAME" ]]; then
  echo "Error: GITHUB_JOB is required (job_name)" >&2
  exit 1
fi
if [[ -z "$EVENT_NAME" ]]; then
  echo "Error: GITHUB_EVENT_NAME is required (event)" >&2
  exit 1
fi
if [[ -z "$SHA" ]]; then
  echo "Error: GITHUB_SHA is required (sha)" >&2
  exit 1
fi
if [[ -z "$GITHUB_RUN_ID_VALUE" ]]; then
  echo "Error: GITHUB_RUN_ID is required (github_run_id)" >&2
  exit 1
fi
if [[ -z "$GITHUB_RUN_ATTEMPT_VALUE" ]]; then
  echo "Error: GITHUB_RUN_ATTEMPT is required (github_run_attempt)" >&2
  exit 1
fi
if [[ -z "$GITHUB_RUN_NUMBER_VALUE" ]]; then
  echo "Error: GITHUB_RUN_NUMBER is required (github_run_number)" >&2
  exit 1
fi

BRANCH=""
if [[ -n "$GITHUB_HEAD_REF_VALUE" ]]; then
  BRANCH="$GITHUB_HEAD_REF_VALUE"
elif [[ -n "$GITHUB_REF_NAME_VALUE" ]]; then
  BRANCH="$GITHUB_REF_NAME_VALUE"
elif [[ -n "$GITHUB_REF_VALUE" ]]; then
  if [[ "$GITHUB_REF_VALUE" == refs/heads/* ]]; then
    BRANCH="${GITHUB_REF_VALUE#refs/heads/}"
  elif [[ "$GITHUB_REF_VALUE" == refs/tags/* ]]; then
    BRANCH="${GITHUB_REF_VALUE#refs/tags/}"
  elif [[ "$GITHUB_REF_VALUE" == refs/pull/* ]]; then
    BRANCH="$GITHUB_REF_VALUE"
  else
    BRANCH="$GITHUB_REF_VALUE"
  fi
fi
if [[ -z "$BRANCH" ]]; then
  echo "Error: branch could not be determined from GitHub context" >&2
  exit 1
fi

PR_NUMBER_JSON="null"
if [[ "$EVENT_NAME" == pull_request* ]]; then
  if [[ "$GITHUB_REF_VALUE" =~ ^refs/pull/([0-9]+)/ ]]; then
    PR_NUMBER_JSON="${BASH_REMATCH[1]}"
  fi
fi

WORKFLOW_REF=""
if [[ -n "$WORKFLOW_REF_RAW" ]]; then
  workflow_ref_part="${WORKFLOW_REF_RAW%@*}"
  prefix="${REPO_FULL_NAME}/"
  if [[ "$workflow_ref_part" == "$prefix"* ]]; then
    WORKFLOW_REF="${workflow_ref_part#"$prefix"}"
  else
    WORKFLOW_REF="$workflow_ref_part"
  fi
fi
if [[ -z "$WORKFLOW_REF" ]]; then
  WORKFLOW_REF="$WORKFLOW_NAME"
fi

RUN_URL="${GITHUB_SERVER_URL_VALUE}/${REPO_FULL_NAME}/actions/runs/${GITHUB_RUN_ID_VALUE}"

echo "Searching for JUnit files matching: $JUNIT_PATHS"

FILES=()
pattern="${JUNIT_PATHS#./}"
while IFS= read -r -d '' file; do
  FILES+=("$file")
done < <(find . -type f -path "./$pattern" -print0 2>/dev/null || true)

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "Warning: No JUnit files found matching pattern '$JUNIT_PATHS'"
  echo "Skipping upload (this is not a failure)"
  exit 0
fi

echo "Found ${#FILES[@]} JUnit file(s)"

COMPLETED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

META_FILE="$(mktemp)"
RESP_FILE="$(mktemp)"
cleanup() {
  rm -f "$META_FILE" "$RESP_FILE"
}
trap cleanup EXIT

export FG_META_PROJECT_SLUG="$PROJECT_SLUG"
export FG_META_REPO_FULL_NAME="$REPO_FULL_NAME"
export FG_META_WORKFLOW_NAME="$WORKFLOW_NAME"
export FG_META_WORKFLOW_REF="$WORKFLOW_REF"
export FG_META_GITHUB_RUN_ID="$GITHUB_RUN_ID_VALUE"
export FG_META_GITHUB_RUN_ATTEMPT="$GITHUB_RUN_ATTEMPT_VALUE"
export FG_META_GITHUB_RUN_NUMBER="$GITHUB_RUN_NUMBER_VALUE"
export FG_META_RUN_URL="$RUN_URL"
export FG_META_SHA="$SHA"
export FG_META_BRANCH="$BRANCH"
export FG_META_EVENT="$EVENT_NAME"
export FG_META_PR_NUMBER="$PR_NUMBER_JSON"
export FG_META_JOB_NAME="$JOB_NAME"
export FG_META_JOB_VARIANT="$JOB_VARIANT"
export FG_META_STARTED_AT="$STARTED_AT"
export FG_META_COMPLETED_AT="$COMPLETED_AT"

python3 - "$META_FILE" <<'PY'
import json
import os
import sys

def req(name: str) -> str:
    v = os.environ.get(name, "")
    if not v:
        raise SystemExit(f"missing {name}")
    return v

pr_raw = os.environ.get("FG_META_PR_NUMBER", "null").strip()
pr_number = None if pr_raw in ("", "null") else int(pr_raw)

meta = {
    "project_slug": req("FG_META_PROJECT_SLUG"),
    "repo_full_name": req("FG_META_REPO_FULL_NAME"),
    "workflow_name": req("FG_META_WORKFLOW_NAME"),
    "workflow_ref": req("FG_META_WORKFLOW_REF"),
    "github_run_id": int(req("FG_META_GITHUB_RUN_ID")),
    "github_run_attempt": int(req("FG_META_GITHUB_RUN_ATTEMPT")),
    "github_run_number": int(req("FG_META_GITHUB_RUN_NUMBER")),
    "run_url": req("FG_META_RUN_URL"),
    "sha": req("FG_META_SHA"),
    "branch": req("FG_META_BRANCH"),
    "event": req("FG_META_EVENT"),
    "pr_number": pr_number,
    "job_name": req("FG_META_JOB_NAME"),
    "job_variant": os.environ.get("FG_META_JOB_VARIANT", ""),
    "started_at": req("FG_META_STARTED_AT"),
    "completed_at": req("FG_META_COMPLETED_AT"),
}

with open(sys.argv[1], "w", encoding="utf-8") as f:
    json.dump(meta, f, separators=(",", ":"), ensure_ascii=False)
PY

API_ENDPOINT="${FLAKEGUARD_URL%/}/api/v1/ingest/junit"

CURL_ARGS=()
CURL_ARGS+=("-sS")
CURL_ARGS+=("-X" "POST")
CURL_ARGS+=("-H" "Authorization: Bearer $API_KEY")
CURL_ARGS+=("-F" "meta=@$META_FILE;type=application/json")

for file in "${FILES[@]}"; do
  if [[ -f "$file" ]]; then
    CURL_ARGS+=("-F" "junit=@$file")
  fi
done

echo "Uploading to: $API_ENDPOINT"

set +e
HTTP_CODE=$(curl -o "$RESP_FILE" -w "%{http_code}" "${CURL_ARGS[@]}" "$API_ENDPOINT")
CURL_EXIT_CODE=$?
set -e

RESPONSE_BODY="$(cat "$RESP_FILE")"

if [[ $CURL_EXIT_CODE -ne 0 ]]; then
  echo "Error: curl failed with exit code $CURL_EXIT_CODE" >&2
  echo "$RESPONSE_BODY" >&2
  exit 1
fi

if [[ ! "$HTTP_CODE" =~ ^2[0-9][0-9]$ ]]; then
  echo "Error: Upload failed (HTTP $HTTP_CODE)" >&2
  echo "$RESPONSE_BODY" >&2
  exit 1
fi

echo "Upload successful (HTTP $HTTP_CODE)"

if command -v jq >/dev/null 2>&1; then
  ingestion_id=$(echo "$RESPONSE_BODY" | jq -r '.data.ingestion_id // empty' 2>/dev/null || true)
  junit_files=$(echo "$RESPONSE_BODY" | jq -r '.data.stored.junit_files // empty' 2>/dev/null || true)
  test_results=$(echo "$RESPONSE_BODY" | jq -r '.data.stored.test_results // empty' 2>/dev/null || true)
  flake_events=$(echo "$RESPONSE_BODY" | jq -r '.data.flake_events_created // empty' 2>/dev/null || true)

  if [[ -n "$ingestion_id" ]]; then
    echo "Ingestion: $ingestion_id (stored junit_files=$junit_files, test_results=$test_results, flake_events_created=$flake_events)"
  else
    echo "$RESPONSE_BODY"
  fi
else
  echo "$RESPONSE_BODY"
fi

echo "FlakeGuard: upload completed"
