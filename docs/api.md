# FlakeGuard API

This document summarizes the JSON endpoints and response envelopes.

## Authentication

- **CI/agents**: `Authorization: Bearer <project_api_key>` (requires scope `ingest:write`)
- **Dashboard users**: session cookie via `/api/v1/auth/*`

Session-protected endpoints require CSRF:

- Cookie: `fg_csrf`
- Header: `X-CSRF-Token: <value>`

## Response envelopes

Success:

```json
{ "request_id": "req_01H...", "data": { } }
```

Error:

```json
{ "error": { "code": "bad_request", "message": "Invalid request body", "request_id": "req_01H..." } }
```

## Auth

- `POST /api/v1/auth/signup` (`email`, `password`)
- `POST /api/v1/auth/login` (`email`, `password`)
- `POST /api/v1/auth/logout`

## Organizations

- `POST /api/v1/orgs` (create org)
- `GET /api/v1/orgs` (list orgs)

Members:

- `GET /api/v1/orgs/{org_id}/members`
- `PUT /api/v1/orgs/{org_id}/members/{user_id}` (update role)
- `DELETE /api/v1/orgs/{org_id}/members/{user_id}` (remove member / leave)

Invites:

- `POST /api/v1/orgs/{org_id}/invites` (create invite; returns `data.invite.accept_url` once)
- `GET /api/v1/orgs/{org_id}/invites` (active invites)
- `DELETE /api/v1/orgs/{org_id}/invites/{invite_id}` (revoke)
- `POST /api/v1/orgs/invites/accept` (accept invite; requires logged-in user)

Audit:

- `GET /api/v1/orgs/{org_id}/audit?limit=50` (OWNER/ADMIN)

## Projects

Under an org:

- `POST /api/v1/orgs/{org_id}/projects` (create project)
- `GET /api/v1/orgs/{org_id}/projects` (list projects)

Project settings:

- `PUT /api/v1/projects/{project_id}/slack`
- `DELETE /api/v1/projects/{project_id}/slack`

API keys:

- `POST /api/v1/projects/{project_id}/api-keys`
- `GET /api/v1/projects/{project_id}/api-keys`
- `DELETE /api/v1/projects/{project_id}/api-keys/{api_key_id}`

Flakes:

- `GET /api/v1/projects/{project_id}/flakes?days=30&repo=...&job_name=...`
- `GET /api/v1/projects/{project_id}/flakes/{test_case_id}?days=30`

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
