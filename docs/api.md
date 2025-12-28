# FlakeGuard API (MVP)

This document summarizes the integration endpoints and response envelopes.

## Auth

- **CI/agents**: `Authorization: Bearer <project_api_key>` (scope `ingest:write`)
- **Dashboard users**: session cookie via `/api/v1/auth/*` (JSON-only endpoints)

## Response Envelopes

Success:

```json
{ "request_id": "req_01H...", "data": { } }
```

Error:

```json
{ "error": { "code": "invalid_meta", "message": "meta.branch is required", "request_id": "req_01H..." } }
```

## Ingestion

### POST `/api/v1/ingest/junit`

Auth: Bearer API key (`ingest:write`)

Content-Type: `multipart/form-data`

- `meta` (application/json, required)
- `junit` (file, required; may be repeated)

Response: `202 Accepted`

```json
{
  "request_id": "req_01H...",
  "data": {
    "ingestion_id": "7a2f0df1-bac9-4d3e-9b2d-4a2a1f2d0eaa",
    "stored": { "junit_files": 2, "test_results": 842 },
    "flake_events_created": 1
  }
}
```

## Dashboard (Read)

### GET `/api/v1/projects/{project_id}/flakes`

Query params:

- `days` (int)
- `repo` (string)
- `job_name` (string)

Response: `200 OK` with `data.flakes[]`.

### GET `/api/v1/projects/{project_id}/flakes/{test_case_id}`

Query params:

- `days` (int)

Response: `200 OK` with `data.flake` and `data.flake.evidence[]`.
