# FlakeGuard SSOT (Single Source of Truth) Specification
Version: 1.0.0  
Last Updated: 2025-12-25  
Product: FlakeGuard (CI Flake Firewall)

---

## 0. SSOT-Regeln

### 0.1 SSOT Vorrangregeln (Normativ)
1. This document is the **only** authoritative truth for what MUST be built.
2. If anything conflicts (README, code comments, tickets, Slack), **this SSOT wins**.
3. Anything not explicitly specified here is **OUT OF SCOPE** and MUST NOT be implemented.
4. No “reasonable assumptions” are allowed. If a detail is missing, implementation MUST stop and the SSOT MUST be updated first.
5. All behavior MUST be deterministic and testable.
6. All external integrations MUST be feature-flagged and safe-by-default (no destructive actions).
7. All config MUST be provided via environment variables (12-factor). No hidden config files in production.
8. All migrations MUST be idempotent and MUST run cleanly on a fresh PostgreSQL database.
9. All API responses MUST be stable and backward-compatible within the same major version (`v1`).
10. The system MUST be multi-tenant from day 1 (Organizations), even if single-user initially.

### 0.2 Definition of Done (Global)
A feature is DONE only if:
- It is implemented exactly as specified.
- It has unit tests for core logic and integration tests for persistence.
- It is accessible through API (and UI where required).
- It is documented minimally in `/docs/` (as specified).
- It is covered by acceptance criteria in Section 19.

### 0.3 “Claude Code CLI” Execution Rules (Normativ)
When using Claude Code CLI to implement this:
- Claude MUST follow the milestones in Section 18 in order.
- Claude MUST NOT refactor scope or introduce new dependencies beyond Section 11.
- Claude MUST run tests after each milestone and fix failures before continuing.
- Claude MUST keep changes small and commit-ready per milestone.

---

## 1. Executive Summary

### 1.1 One-liner
**FlakeGuard automatically detects flaky CI tests from reruns and turns them into actionable signals (dashboard + Slack).**

### 1.2 What / Why / For whom
- **What:** A SaaS service + ingestion API that receives JUnit test results from GitHub Actions, detects flaky tests using rerun attempts, and surfaces them with evidence.
- **Why:** Flaky tests waste engineering time, destroy trust in CI, increase reruns, and hide real regressions.
- **For whom:** Platform Engineering / DevEx / Engineering Managers running GitHub Actions with JUnit outputs.

### 1.3 MVP Scope (Canonical)
MVP MUST include:
1. A multi-tenant web app (Organizations + Projects).
2. An ingestion API secured by project API keys.
3. JUnit parsing + persistent storage of test outcomes.
4. Flake detection using **GitHub rerun attempts** (`GITHUB_RUN_ATTEMPT`).
5. A dashboard showing flaky tests with evidence.
6. Slack webhook notifications for new flake evidence.
7. A GitHub Action (composite action) to upload JUnit files to the ingestion API.

---

## 2. Problem Statement

### 2.1 Pain (Concrete)
Teams experience:
- CI failures that disappear on rerun (“green after rerun”).
- Devs re-running workflows blindly, wasting minutes to hours per day.
- On-call / EMs losing trust: “Is CI red real or flaky?”
- Flaky tests persisting for months because there is no ownership, evidence, or tracking.

### 2.2 Cost
Costs MUST be framed as:
- **Developer time:** reruns + debugging phantom failures.
- **CI compute cost:** extra minutes per rerun.
- **Risk:** real regressions slip when teams habituate to ignoring red builds.

### 2.3 Workarounds Today
Common workarounds:
- Manual rerun + shrug.
- “Quarantine list” in a wiki with no enforcement.
- Custom scripts that don’t persist evidence and don’t work across repos.

### 2.4 Why Current Tooling Fails
- GitHub Actions UI shows logs, but not **cross-run** flake tracking.
- CI providers don’t cluster flaky failures into durable, actionable objects.
- Most teams lack a lightweight system to turn reruns into metrics + notifications.

---

## 3. Zielgruppe & Personas

### 3.1 Target Segments
| Segment | Company Stage | CI Setup | Will Pay Because |
|---|---|---|---|
| DevEx / Platform team | Series A–C | GitHub Actions, JUnit reports | CI stability is a KPI |
| Engineering managers | 30–300 engineers | multiple repos | lost time + delivery risk |
| QA automation leads | mid-market | flaky E2E suites | productivity + credibility |

### 3.2 Personas (Normativ)
1. **Paula (Platform Engineer, 60-person SaaS)**
   - Owns CI reliability.
   - Wants top flaky tests and proof of flakiness.
   - Needs Slack notifications without noise.

2. **Evan (Engineering Manager, 8 squads)**
   - Wants “what to fix first” and trend visibility.
   - Doesn’t want to read raw logs.

3. **Quinn (QA Automation Lead)**
   - Wants stable E2E runs.
   - Wants a quarantine list and evidence for prioritization.

### 3.3 Buyers
- Primary buyer MUST be: **Head of Platform Engineering / DevEx Lead**.
- Secondary buyer MAY be: **Engineering Manager**.

---

## 4. Produkt-Vision & Phasen

### 4.1 MVP (this SSOT)
- GitHub Actions → Upload JUnit → Detect flake via rerun attempts → Dashboard + Slack.
- No automatic PR creation.
- No advanced RBAC beyond org roles.
- No billing.

### 4.2 v1 (Post-MVP)
- Ownership rules (regex/prefix) used in Slack notifications.
- Daily digest scheduling per project timezone.
- “Quarantine snippet” export for `.flakeguard.yml`.

### 4.3 v2
- GitHub App integration for deeper metadata (optional).
- PR automation to quarantine known flakes (opt-in).
- Trend analytics (flake score over time).

### 4.4 Enterprise
- SSO (SAML/OIDC), SCIM.
- Advanced audit logs.
- Dedicated retention controls and data residency.

---

## 5. Prinzipien

### 5.1 Security Principles
- API keys MUST be stored hashed (never in plaintext).
- Passwords MUST be stored using bcrypt with cost 12.
- All inbound ingest MUST be authenticated and rate-limited.
- Multi-tenancy MUST be enforced at query-level (project/org scoping).
- Sensitive data in logs MUST be minimized (no raw API keys; no full JUnit payload dumps).

### 5.2 UX Principles
- Dashboard MUST answer in < 10 seconds: “Which tests are flaky and why?”
- Evidence MUST be concrete: show run attempts where failure turned into pass.
- Slack notifications MUST be concise, link to details.

### 5.3 Architecture Principles
- Single Go binary (API + UI + background jobs) for MVP.
- PostgreSQL is the system of record.
- Stateless app instances; no local state required beyond caching.

---

## 6. Domain Model & Glossar (Canonical Objects)

### 6.1 Canonical Objects
| Object | Description | Key Fields |
|---|---|---|
| Organization | Tenant boundary | `id`, `name`, `slug` |
| User | Human account | `id`, `email`, `password_hash` |
| Membership | User ↔ Org role | `role` |
| Project | A logical grouping (usually one GitHub org) | `id`, `org_id`, `name` |
| Repo | A GitHub repository tracked by a project | `full_name` (`owner/repo`) |
| API Key | Agent credential for ingest | `token_hash`, `scopes` |
| CI Run | GitHub workflow run | `github_run_id`, `sha`, `branch` |
| CI Attempt | A rerun attempt number | `attempt_number` |
| CI Job | One job execution per attempt | `job_name`, `job_variant` |
| Test Case | Canonical test identity | `test_identifier` |
| Test Result | Outcome of a test in a job attempt | `status`, `duration_ms` |
| Flake Event | Evidence: fail then pass across attempts | links to results |
| Flake Stat | Aggregated flakiness score and counts | `flake_score`, `last_seen_at` |
| Notification Channel | Slack webhook per project | `webhook_url` |
| Audit Log | Security-relevant actions | `actor_user_id`, `action`, `meta` |

### 6.2 Glossary (Normativ)
- **JUnit**: XML format representing test suites, test cases, failures.
- **Flaky test**: A test that produces different outcomes (fail/pass) on rerun attempts for the same workflow run context.
- **Evidence**: A pair (or set) of attempts showing fail then pass.
- **Project API Key**: A bearer token used by CI to ingest results.

---

## 7. Funktionale Anforderungen

### 7.1 Modules Overview (Normativ)
| Module | MUST Provide |
|---|---|
| Auth (Human) | Signup, login, session auth |
| Org & Project Mgmt | CRUD orgs/projects, membership roles |
| API Keys | Create/revoke keys; show plaintext once |
| Ingestion | Multipart JUnit upload + metadata; dedupe |
| Parsing | Parse JUnit XML into canonical test results |
| Flake Detection | Detect fail→pass across attempts; store events |
| Aggregation | Maintain flake stats per test case |
| Dashboard UI | Flake list + detail evidence view |
| Slack Notifications | Post on new flake event; include link |
| Audit Logging | Record key security actions |

### 7.2 Auth (Human)
- Users MUST be able to register with email + password.
- Users MUST be able to login and receive a session cookie (HTTP-only).
- Sessions MUST be JWT-based stored in a cookie named `fg_session`.
- JWT MUST be signed with `FG_JWT_SECRET` using HS256.

### 7.3 Organization & Membership
- A user creating an org MUST become `OWNER`.
- Roles MUST include: `OWNER`, `ADMIN`, `MEMBER`, `VIEWER`.
- Only `OWNER` and `ADMIN` can manage projects and API keys.
- `VIEWER` can read dashboards but cannot create keys or change settings.

### 7.4 Projects
- Each Organization MAY have multiple projects.
- Each project MUST have:
  - A name
  - A slug unique within the org
  - An optional Slack webhook config
  - Retention settings (MVP: fixed defaults, not configurable)

### 7.5 API Keys (Agent Auth)
- API Keys MUST be created per project.
- API Keys MUST have scopes (MVP scopes):
  - `ingest:write`
  - `read:project`
- The plaintext token MUST be shown exactly once at creation time.
- The DB MUST store only a hash of the token (`sha256`).
- Requests MUST send `Authorization: Bearer <token>`.

### 7.6 Ingestion
- Ingestion MUST accept:
  - metadata JSON (run/job info)
  - one or more JUnit XML files
- Ingestion MUST:
  - validate auth scope `ingest:write`
  - enforce rate limit per API key
  - parse files
  - store outcomes
  - compute flake events
  - emit Slack notification if a new flake event is created

### 7.7 Flake Detection (MVP Algorithm) (Normativ)
A **Flake Event** MUST be created when:
- For the same `(project_id, repo_full_name, github_run_id, job_name, job_variant, test_identifier)`:
  - the test has status `failed` in an earlier attempt, AND
  - the test has status `passed` in a later attempt
- The attempts MUST be within the same `github_run_id` group.

A test MUST be marked “Flaky” in the dashboard if:
- It has at least 1 Flake Event within the last 30 days.

Flake score MUST be:
- `flake_score = mixed_outcome_runs / total_runs_seen`
Where:
- `mixed_outcome_runs` = count of distinct `github_run_id` where the test had fail and pass across attempts
- `total_runs_seen` = count of distinct `github_run_id` where the test appeared (any outcome)

### 7.8 Dashboard
Dashboard MUST provide:
- A list of flaky tests sorted by `flake_score DESC`, then `last_seen_at DESC`.
- Filters:
  - repo
  - branch
  - job_name
  - date range (MVP: last 7/30 days)
- Flake detail page:
  - test identifier
  - flake score, counts
  - evidence list: attempts and outcomes
  - last failure message snippet (truncated)

### 7.9 Slack Notification
On creation of a new Flake Event, system MUST post to Slack webhook:
- repo, workflow, job, test identifier
- link to flake detail page
- “Evidence: failed on attempt X, passed on attempt Y”

Slack integration MUST be:
- A single webhook URL per project (MVP).
- Optional (if not set, no Slack notifications).

### 7.10 Audit Logging
System MUST record audit events for:
- user signup
- org creation
- project creation
- API key creation/revocation
- slack webhook set/updated/cleared
- login failures (rate-limited storage)

---

## 8. API Specification

### 8.1 API Conventions (Normativ)
- Base path MUST be `/api/v1`.
- All responses MUST be JSON with `Content-Type: application/json`.
- Errors MUST use a stable envelope:
```json
{
  "error": {
    "code": "string",
    "message": "string",
    "request_id": "string"
  }
}
Success responses MUST include request_id:
json
Code kopieren
{
  "request_id": "string",
  "data": {}
}
IDs MUST be UUID v4 strings.
Timestamps MUST be RFC3339 with timezone.
8.2 Authentication
8.2.1 Human Auth (session cookie)
UI + JSON endpoints for humans MUST rely on cookie fg_session.
The cookie MUST be HttpOnly, Secure (in production), SameSite=Lax.
8.2.2 Agent Auth (API key)
Ingestion endpoints MUST require Authorization: Bearer <api_key>.
API key tokens MUST be random 32 bytes, base64url encoded (no padding).
8.3 Endpoints
8.3.1 Health
GET /healthz
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "status": "ok"
  }
}
GET /readyz
MUST check DB connectivity.
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "status": "ready",
    "db": "ok"
  }
}
8.4 Human Auth API
8.4.1 Signup
POST /api/v1/auth/signup
Request:
json
Code kopieren
{
  "email": "paula@example.com",
  "password": "CorrectHorseBatteryStaple!"
}
Response 201:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "user": {
      "id": "3c2f7d9f-1c1b-4c1d-9a0c-3e3d1b2b0c11",
      "email": "paula@example.com",
      "created_at": "2025-12-25T10:00:00Z"
    }
  }
}
8.4.2 Login
POST /api/v1/auth/login
Request:
json
Code kopieren
{
  "email": "paula@example.com",
  "password": "CorrectHorseBatteryStaple!"
}
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "user": {
      "id": "3c2f7d9f-1c1b-4c1d-9a0c-3e3d1b2b0c11",
      "email": "paula@example.com"
    }
  }
}
MUST set cookie fg_session.
8.4.3 Logout
POST /api/v1/auth/logout
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "logged_out": true
  }
}
MUST clear cookie.
8.5 Organizations & Membership
8.5.1 Create Organization
POST /api/v1/orgs
Request:
json
Code kopieren
{
  "name": "Acme Inc",
  "slug": "acme"
}
Response 201:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "org": {
      "id": "b5f6f2cc-2a0c-4cb7-93b0-6f3f7dcb9c11",
      "name": "Acme Inc",
      "slug": "acme",
      "created_at": "2025-12-25T10:05:00Z"
    }
  }
}
8.5.2 List Organizations (for current user)
GET /api/v1/orgs
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "orgs": [
      {
        "id": "b5f6f2cc-2a0c-4cb7-93b0-6f3f7dcb9c11",
        "name": "Acme Inc",
        "slug": "acme",
        "role": "OWNER"
      }
    ]
  }
}
8.5.3 List Members
GET /api/v1/orgs/{org_id}/members
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "members": [
      {
        "user_id": "3c2f7d9f-1c1b-4c1d-9a0c-3e3d1b2b0c11",
        "email": "paula@example.com",
        "role": "OWNER",
        "created_at": "2025-12-25T10:05:10Z"
      }
    ]
  }
}
8.6 Projects
8.6.1 Create Project
POST /api/v1/orgs/{org_id}/projects
Request:
json
Code kopieren
{
  "name": "GitHub Actions CI",
  "slug": "gha-ci",
  "default_branch": "main"
}
Response 201:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "project": {
      "id": "1a4a7e0e-8f12-4d7f-8a6e-2f29a2b7c1e1",
      "org_id": "b5f6f2cc-2a0c-4cb7-93b0-6f3f7dcb9c11",
      "name": "GitHub Actions CI",
      "slug": "gha-ci",
      "default_branch": "main",
      "created_at": "2025-12-25T10:10:00Z"
    }
  }
}
8.6.2 List Projects
GET /api/v1/orgs/{org_id}/projects
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "projects": [
      {
        "id": "1a4a7e0e-8f12-4d7f-8a6e-2f29a2b7c1e1",
        "name": "GitHub Actions CI",
        "slug": "gha-ci",
        "default_branch": "main"
      }
    ]
  }
}
8.7 Project Settings (Slack)
8.7.1 Set Slack Webhook
PUT /api/v1/projects/{project_id}/slack
Request:
json
Code kopieren
{
  "webhook_url": "https://hooks.slack.com/services/T000/B000/XXXX",
  "enabled": true
}
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "slack": {
      "enabled": true,
      "webhook_url_set": true
    }
  }
}
Response MUST NOT echo the full webhook URL. It MUST only confirm it is set.
8.7.2 Clear Slack Webhook
DELETE /api/v1/projects/{project_id}/slack
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "slack": {
      "enabled": false,
      "webhook_url_set": false
    }
  }
}
8.8 API Keys
8.8.1 Create API Key
POST /api/v1/projects/{project_id}/api-keys
Request:
json
Code kopieren
{
  "name": "github-actions-uploader",
  "scopes": ["ingest:write"]
}
Response 201:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "api_key": {
      "id": "c3c3c2a4-9d56-4d53-a0a2-4d12d9c7d111",
      "name": "github-actions-uploader",
      "scopes": ["ingest:write"],
      "token": "fgk_5HcQfX3oJv1p...base64url...",
      "created_at": "2025-12-25T10:12:00Z"
    }
  }
}
token MUST be returned only once.
8.8.2 List API Keys
GET /api/v1/projects/{project_id}/api-keys
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "api_keys": [
      {
        "id": "c3c3c2a4-9d56-4d53-a0a2-4d12d9c7d111",
        "name": "github-actions-uploader",
        "scopes": ["ingest:write"],
        "created_at": "2025-12-25T10:12:00Z",
        "revoked_at": null,
        "last_used_at": "2025-12-25T11:00:00Z"
      }
    ]
  }
}
8.8.3 Revoke API Key
DELETE /api/v1/projects/{project_id}/api-keys/{api_key_id}
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "revoked": true
  }
}
8.9 Ingestion API (Agent)
8.9.1 Ingest JUnit Results
POST /api/v1/ingest/junit
Auth: Authorization: Bearer <project_api_key>
Content-Type: multipart/form-data Form fields:
meta (application/json) REQUIRED
junit (file) REQUIRED; MAY appear multiple times
meta schema:
json
Code kopieren
{
  "project_slug": "gha-ci",
  "repo_full_name": "acme/payment-service",
  "workflow_name": "CI",
  "workflow_ref": ".github/workflows/ci.yml",
  "github_run_id": 1234567890,
  "github_run_attempt": 1,
  "github_run_number": 421,
  "run_url": "https://github.com/acme/payment-service/actions/runs/1234567890",
  "sha": "a3c1e7b9f2a0...",
  "branch": "main",
  "event": "push",
  "pr_number": null,
  "job_name": "test",
  "job_variant": "ubuntu-latest",
  "started_at": "2025-12-25T09:58:00Z",
  "completed_at": "2025-12-25T10:03:12Z"
}
Response 202:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "ingestion_id": "7a2f0df1-bac9-4d3e-9b2d-4a2a1f2d0eaa",
    "stored": {
      "junit_files": 2,
      "test_results": 842
    },
    "flake_events_created": 1
  }
}
Error 400 (invalid meta):
json
Code kopieren
{
  "error": {
    "code": "invalid_meta",
    "message": "meta.branch is required",
    "request_id": "req_01H..."
  }
}
8.10 Dashboard APIs (Read)
8.10.1 List Flaky Tests
GET /api/v1/projects/{project_id}/flakes?days=30&repo=acme/payment-service&job_name=test
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "flakes": [
      {
        "test_case_id": "0b5d7d10-2bd4-4c7e-b4a1-9a1f1bcae123",
        "repo_full_name": "acme/payment-service",
        "job_name": "test",
        "job_variant": "ubuntu-latest",
        "test_identifier": "github.com/acme/payment-service/pkg/payments#TestRefunds",
        "flake_score": 0.25,
        "mixed_outcome_runs": 2,
        "total_runs_seen": 8,
        "first_seen_at": "2025-12-20T12:01:00Z",
        "last_seen_at": "2025-12-25T10:03:12Z"
      }
    ]
  }
}
8.10.2 Flake Detail (Evidence)
GET /api/v1/projects/{project_id}/flakes/{test_case_id}?days=30
Response 200:
json
Code kopieren
{
  "request_id": "req_01H...",
  "data": {
    "flake": {
      "test_case_id": "0b5d7d10-2bd4-4c7e-b4a1-9a1f1bcae123",
      "repo_full_name": "acme/payment-service",
      "job_name": "test",
      "job_variant": "ubuntu-latest",
      "test_identifier": "github.com/acme/payment-service/pkg/payments#TestRefunds",
      "flake_score": 0.25,
      "mixed_outcome_runs": 2,
      "total_runs_seen": 8,
      "last_failure_message": "expected status 200 got 500 ... (truncated)",
      "evidence": [
        {
          "github_run_id": 1234567890,
          "run_url": "https://github.com/acme/payment-service/actions/runs/1234567890",
          "sha": "a3c1e7b9f2a0...",
          "attempt_failed": 1,
          "attempt_passed": 2,
          "failed_at": "2025-12-25T10:03:12Z",
          "passed_at": "2025-12-25T10:08:40Z"
        }
      ]
    }
  }
}
9. Datenmodell (SQL Schema, vollständig)
9.1 PostgreSQL Assumptions (Normativ)
PostgreSQL version MUST be 16.x.
Extensions MUST be enabled:
pgcrypto (UUID generation)
citext (case-insensitive email)
9.2 Full SQL (psql-executable)
The following SQL MUST run on a fresh database via psql -f schema.sql with no edits.
sql
Code kopieren
BEGIN;

-- Extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

-- Enums
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'org_role') THEN
    CREATE TYPE org_role AS ENUM ('OWNER','ADMIN','MEMBER','VIEWER');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'api_key_scope') THEN
    CREATE TYPE api_key_scope AS ENUM ('ingest:write','read:project');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'test_status') THEN
    CREATE TYPE test_status AS ENUM ('passed','failed','skipped','error');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'ci_event') THEN
    CREATE TYPE ci_event AS ENUM ('push','pull_request','workflow_dispatch','schedule','other');
  END IF;
END $$;

-- Helper function for updated_at
CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- USERS
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email CITEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

DROP TRIGGER IF EXISTS trg_users_updated_at ON users;
CREATE TRIGGER trg_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ORGANIZATIONS
CREATE TABLE IF NOT EXISTS orgs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT orgs_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

DROP TRIGGER IF EXISTS trg_orgs_updated_at ON orgs;
CREATE TRIGGER trg_orgs_updated_at
BEFORE UPDATE ON orgs
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- MEMBERSHIPS
CREATE TABLE IF NOT EXISTS org_memberships (
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role org_role NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (org_id, user_id)
);

-- PROJECTS
CREATE TABLE IF NOT EXISTS projects (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  default_branch TEXT NOT NULL DEFAULT 'main',
  slack_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  slack_webhook_url TEXT NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (org_id, slug),
  CONSTRAINT projects_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$')
);

DROP TRIGGER IF EXISTS trg_projects_updated_at ON projects;
CREATE TRIGGER trg_projects_updated_at
BEFORE UPDATE ON projects
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- API KEYS (store only hash)
CREATE TABLE IF NOT EXISTS api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  token_hash BYTEA NOT NULL,
  scopes api_key_scope[] NOT NULL,
  revoked_at TIMESTAMPTZ NULL,
  last_used_at TIMESTAMPTZ NULL,
  created_by_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, name)
);

DROP TRIGGER IF EXISTS trg_api_keys_updated_at ON api_keys;
CREATE TRIGGER trg_api_keys_updated_at
BEFORE UPDATE ON api_keys
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE INDEX IF NOT EXISTS idx_api_keys_project_id ON api_keys(project_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash);

-- INGESTIONS
CREATE TABLE IF NOT EXISTS ingestions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  api_key_id UUID NOT NULL REFERENCES api_keys(id) ON DELETE RESTRICT,
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  meta JSONB NOT NULL,
  junit_files_count INT NOT NULL DEFAULT 0,
  test_results_count INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_ingestions_project_received ON ingestions(project_id, received_at DESC);

-- CI RUNS (per github_run_id)
CREATE TABLE IF NOT EXISTS ci_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  workflow_name TEXT NOT NULL,
  workflow_ref TEXT NOT NULL,
  github_run_id BIGINT NOT NULL,
  github_run_number BIGINT NOT NULL,
  run_url TEXT NOT NULL,
  sha TEXT NOT NULL,
  branch TEXT NOT NULL,
  event ci_event NOT NULL,
  pr_number BIGINT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, github_run_id)
);

CREATE INDEX IF NOT EXISTS idx_ci_runs_project_repo ON ci_runs(project_id, repo_full_name);
CREATE INDEX IF NOT EXISTS idx_ci_runs_project_last_seen ON ci_runs(project_id, last_seen_at DESC);

-- CI ATTEMPTS
CREATE TABLE IF NOT EXISTS ci_run_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  attempt_number INT NOT NULL,
  started_at TIMESTAMPTZ NULL,
  completed_at TIMESTAMPTZ NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_id, attempt_number),
  CONSTRAINT attempt_number_positive CHECK (attempt_number >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ci_attempts_run ON ci_run_attempts(ci_run_id, attempt_number);

-- CI JOBS (one per attempt + job_name + job_variant)
CREATE TABLE IF NOT EXISTS ci_jobs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ci_run_attempt_id UUID NOT NULL REFERENCES ci_run_attempts(id) ON DELETE CASCADE,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (ci_run_attempt_id, job_name, job_variant)
);

CREATE INDEX IF NOT EXISTS idx_ci_jobs_attempt ON ci_jobs(ci_run_attempt_id);

-- JUNIT FILES (optional raw content, truncated)
CREATE TABLE IF NOT EXISTS junit_files (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  ingestion_id UUID NOT NULL REFERENCES ingestions(id) ON DELETE CASCADE,
  filename TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  size_bytes INT NOT NULL,
  content_truncated BOOLEAN NOT NULL DEFAULT FALSE,
  content BYTEA NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_junit_files_ingestion ON junit_files(ingestion_id);

-- TEST CASES (canonical identity per project+repo+job+variant+identifier)
CREATE TABLE IF NOT EXISTS test_cases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  repo_full_name TEXT NOT NULL,
  job_name TEXT NOT NULL,
  job_variant TEXT NOT NULL DEFAULT '',
  test_identifier TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (project_id, repo_full_name, job_name, job_variant, test_identifier)
);

CREATE INDEX IF NOT EXISTS idx_test_cases_project_last_seen ON test_cases(project_id, last_seen_at DESC);

-- TEST RESULTS (per job execution)
CREATE TABLE IF NOT EXISTS test_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
  ci_job_id UUID NOT NULL REFERENCES ci_jobs(id) ON DELETE CASCADE,
  status test_status NOT NULL,
  duration_ms INT NULL,
  failure_message TEXT NULL,
  failure_output TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (test_case_id, ci_job_id)
);

CREATE INDEX IF NOT EXISTS idx_test_results_test_case ON test_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_results_ci_job ON test_results(ci_job_id);

-- FLAKE EVENTS (evidence of flake: fail->pass across attempts for same run_id)
CREATE TABLE IF NOT EXISTS flake_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
  ci_run_id UUID NOT NULL REFERENCES ci_runs(id) ON DELETE CASCADE,
  failed_attempt_number INT NOT NULL,
  passed_attempt_number INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT flake_attempt_order CHECK (passed_attempt_number > failed_attempt_number)
);

CREATE INDEX IF NOT EXISTS idx_flake_events_test_case_created ON flake_events(test_case_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_flake_events_run ON flake_events(ci_run_id);

-- FLAKE STATS (aggregated)
CREATE TABLE IF NOT EXISTS flake_stats (
  test_case_id UUID PRIMARY KEY REFERENCES test_cases(id) ON DELETE CASCADE,
  mixed_outcome_runs INT NOT NULL DEFAULT 0,
  total_runs_seen INT NOT NULL DEFAULT 0,
  flake_score DOUBLE PRECISION NOT NULL DEFAULT 0,
  last_failure_message TEXT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT flake_counts_nonnegative CHECK (mixed_outcome_runs >= 0 AND total_runs_seen >= 0),
  CONSTRAINT flake_score_range CHECK (flake_score >= 0 AND flake_score <= 1)
);

DROP TRIGGER IF EXISTS trg_flake_stats_updated_at ON flake_stats;
CREATE TRIGGER trg_flake_stats_updated_at
BEFORE UPDATE ON flake_stats
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- AUDIT LOG
CREATE TABLE IF NOT EXISTS audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NULL REFERENCES orgs(id) ON DELETE CASCADE,
  project_id UUID NULL REFERENCES projects(id) ON DELETE CASCADE,
  actor_user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_log_org_created ON audit_log(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_project_created ON audit_log(project_id, created_at DESC);

COMMIT;
10. System-Architektur (Komponenten, Datenfluss)
10.1 Components (MVP)
Go App Server (single binary):
JSON API (/api/v1)
Server-rendered UI (HTML templates)
Background worker (flake aggregation + Slack notifications)
PostgreSQL (system of record)
GitHub Action (composite action) to upload JUnit
10.2 Data Flow (Mermaid)
mermaid
Code kopieren
flowchart LR
  A[GitHub Actions Job] -->|JUnit XML + meta| B[FlakeGuard Ingest API]
  B --> C[(PostgreSQL)]
  B --> D[Flake Detector]
  D --> C
  D -->|new flake event| E[Slack Webhook]
  C --> F[Dashboard UI/API]
10.3 Request Flow: Ingestion (Mermaid)
mermaid
Code kopieren
sequenceDiagram
  participant GH as GitHub Action
  participant API as FlakeGuard API
  participant DB as PostgreSQL
  participant Slack as Slack

  GH->>API: POST /api/v1/ingest/junit (meta + junit files)
  API->>DB: upsert ci_run, attempt, job
  API->>DB: upsert test_cases, insert test_results
  API->>DB: compute & insert flake_events
  API->>DB: update flake_stats
  alt new flake event created and slack enabled
    API->>Slack: POST webhook message
  end
  API-->>GH: 202 Accepted (counts + flake_events_created)
11. Tech Stack (konkrete Libraries, Versionen)
11.1 Runtime & Infra
Go: 1.22.2
PostgreSQL: 16.4
Docker: required
Docker Compose: required
11.2 Go Dependencies (Pinned)
Go module MUST include exactly these direct dependencies (no extras unless required by this SSOT):
Router: github.com/go-chi/chi/v5 v5.0.12
CORS: github.com/go-chi/cors v1.2.1
Rate limit: github.com/go-chi/httprate v0.9.0
Postgres driver: github.com/jackc/pgx/v5 v5.5.5
Logging: github.com/rs/zerolog v1.33.0
JWT: github.com/golang-jwt/jwt/v5 v5.2.1
UUID: github.com/google/uuid v1.6.0
Env loader (dev only): github.com/joho/godotenv v1.5.1
Cron scheduler: github.com/robfig/cron/v3 v3.0.1
Password hashing: golang.org/x/crypto v0.24.0
Testing: github.com/stretchr/testify v1.9.0
11.3 Frontend
No SPA framework in MVP.
UI MUST be server-rendered HTML using Go html/template.
Minimal JS MAY be used (vanilla) for form UX.
CSS MUST be a single file under /web/static/app.css.
12. Repository-Struktur (Ordner, Dateien)
12.1 Monorepo Layout (MVP)
This exact structure MUST exist.
text
Code kopieren
flakeguard/
  README.md
  SSOT.md
  go.mod
  go.sum
  Makefile
  docker-compose.yml
  Dockerfile
  .env.example

  cmd/
    flakeguard/
      main.go

  internal/
    app/
      app.go
      router.go
      middleware.go
      request_id.go
      errors.go

    config/
      config.go

    db/
      db.go
      migrations.go

    auth/
      password.go
      jwt.go
      middleware.go
      handlers.go

    orgs/
      handlers.go
      service.go
      models.go

    projects/
      handlers.go
      service.go
      models.go

    apikeys/
      handlers.go
      service.go
      token.go
      models.go

    ingest/
      handlers.go
      service.go
      junit_parser.go
      models.go
      dedupe.go

    flakes/
      detector.go
      stats.go
      handlers.go
      service.go
      models.go

    slack/
      slack.go

    audit/
      audit.go

    web/
      handlers.go
      templates.go

  migrations/
    0001_schema.sql

  web/
    templates/
      layout.html
      login.html
      signup.html
      org_list.html
      org_create.html
      project_list.html
      project_create.html
      project_settings.html
      flakes_list.html
      flake_detail.html
    static/
      app.css
      app.js

  action/
    action.yml
    README.md
    upload.sh

  docs/
    api.md
    github-action.md
    runbook.md

  testdata/
    junit/
      passing.xml
      failing.xml
      flaky_attempt1.xml
      flaky_attempt2.xml
12.2 File Purposes (Normativ)
SSOT.md MUST be a copy of this document.
migrations/0001_schema.sql MUST contain Section 9 SQL verbatim.
action/ MUST contain the composite GitHub Action for upload.
docs/ MUST document setup and usage.
13. AuthN/AuthZ (Agent Auth, Human Auth, RBAC)
13.1 Human AuthN
Signup/login as in Section 8.4.
JWT claims MUST include:
sub = user_id
exp = expiry (default 7 days)
iat
JWT cookie MUST be rotated on login.
13.2 Human AuthZ (RBAC)
Every request that references an org or project MUST validate membership.
Role capabilities MUST be:
Action	OWNER	ADMIN	MEMBER	VIEWER
View dashboards	✅	✅	✅	✅
Create projects	✅	✅	❌	❌
Manage API keys	✅	✅	❌	❌
Manage Slack webhook	✅	✅	❌	❌
Invite members	(MVP) ❌	(MVP) ❌	❌	❌

Note: Member invites are OUT OF SCOPE for MVP; memberships are created only implicitly (org creator). This is intentional.
13.3 Agent AuthN
API key tokens MUST be generated server-side and returned once.
Token format MUST be: fgk_<base64url> where <base64url> is 43 characters (32 bytes raw).
Server MUST store sha256(token) as token_hash.
13.4 Agent AuthZ
Ingest endpoint MUST require ingest:write.
Read endpoints MUST NOT accept API keys in MVP (human-only for reads).
14. Security Model (Threats, Mitigations)
14.1 Threats
Threat	Risk	Mitigation (MUST)
API key leakage	Unauthorized ingest spam	store hashes, rotate keys, rate limit
Multi-tenant data leak	severe	enforce org/project scoping in every query
Password brute force	account takeover	rate limit login + bcrypt
Slack webhook leakage	spam	never echo webhook URL; allow clearing
Large JUnit upload	DoS	enforce size limits + timeouts
SQL injection	data loss	parameterized queries only
Sensitive logs	privacy	redact auth headers, no full payload logging

14.2 Ingestion Hard Limits (MVP MUST)
Max files per ingest: 20
Max size per file: 1 MB
Max total upload size: 5 MB
Failure output stored MUST be truncated to 8 KB per test result.
Failure message stored MUST be truncated to 1 KB.
14.3 Operational Security
App MUST support FG_LOG_LEVEL and MUST not log secrets at any level.
App MUST generate request IDs and include them in logs and error responses.
15. Non-Functional Requirements (Performance, Reliability)
15.1 Performance
Ingest request MUST respond within 3 seconds for up to 2000 test cases (excluding network upload time).
Dashboard list endpoint MUST respond within 500 ms for 10k flaky tests (with indexing).
15.2 Reliability
Ingest MUST be idempotent for the same (project_slug, repo_full_name, github_run_id, github_run_attempt, job_name, job_variant):
repeated ingest MUST NOT duplicate results.
Slack posting failures MUST NOT fail ingestion; they MUST be logged with request_id.
15.3 Observability (Minimal)
App MUST expose /healthz and /readyz.
App MUST log structured JSON logs to stdout.
16. Test-Strategie (Unit, Integration, E2E)
16.1 Unit Tests MUST cover
JUnit parser:
passing.xml
failing.xml
flaky attempt pair
Flake detection:
fail on attempt 1, pass on attempt 2 ⇒ creates flake_event
pass then pass ⇒ no flake_event
API key hashing/verification
JWT creation/validation
16.2 Integration Tests MUST cover
Apply migration to fresh Postgres
Ingest endpoint writes expected rows
Flake stats aggregation correctness
16.3 E2E (Minimal)
Use httptest:
signup → create org → create project → create key → ingest → list flakes
17. Deployment (Docker Compose, Env Vars)
17.1 Environment Variables (Complete List)
All variables MUST be documented and supported:
Env Var	Required	Default	Description
FG_ENV	yes	(none)	dev or prod
FG_HTTP_ADDR	yes	:8080	listen address
FG_BASE_URL	yes	(none)	public base URL, e.g. http://localhost:8080
FG_DB_DSN	yes	(none)	Postgres DSN
FG_JWT_SECRET	yes	(none)	JWT signing secret (>=32 chars)
FG_LOG_LEVEL	no	info	debug/info/warn/error
FG_RATE_LIMIT_RPM	no	120	requests/min per api key (ingest)
FG_MAX_UPLOAD_BYTES	no	5242880	5MB
FG_MAX_UPLOAD_FILES	no	20	max files
FG_MAX_FILE_BYTES	no	1048576	1MB per file
FG_SLACK_TIMEOUT_MS	no	2000	Slack post timeout
FG_SESSION_DAYS	no	7	session duration

17.2 docker-compose.yml (MUST)
yaml
Code kopieren
services:
  db:
    image: postgres:16.4-alpine
    environment:
      POSTGRES_PASSWORD: postgres
      POSTGRES_USER: postgres
      POSTGRES_DB: flakeguard
    ports:
      - "5432:5432"
    volumes:
      - fg_db:/var/lib/postgresql/data

  app:
    build: .
    environment:
      FG_ENV: dev
      FG_HTTP_ADDR: :8080
      FG_BASE_URL: http://localhost:8080
      FG_DB_DSN: postgres://postgres:postgres@db:5432/flakeguard?sslmode=disable
      FG_JWT_SECRET: change-me-in-dev-please-change
      FG_LOG_LEVEL: debug
    ports:
      - "8080:8080"
    depends_on:
      - db

volumes:
  fg_db: {}
17.3 Dockerfile (MUST)
dockerfile
Code kopieren
# build
FROM golang:1.22.2-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/flakeguard ./cmd/flakeguard

# runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/flakeguard /app/flakeguard
COPY web /app/web
COPY migrations /app/migrations
ENV FG_HTTP_ADDR=:8080
EXPOSE 8080
CMD ["/app/flakeguard"]
17.4 Makefile (MUST)
makefile
Code kopieren
.PHONY: dev test fmt lint db-migrate

dev:
	docker compose up --build

test:
	go test ./... -count=1

fmt:
	go fmt ./...

lint:
	@echo "No external linter in MVP. Use go vet."
	go vet ./...

db-migrate:
	@echo "Apply migrations via psql using FG_DB_DSN"
17.5 .env.example (MUST)
env
Code kopieren
FG_ENV=dev
FG_HTTP_ADDR=:8080
FG_BASE_URL=http://localhost:8080
FG_DB_DSN=postgres://postgres:postgres@localhost:5432/flakeguard?sslmode=disable
FG_JWT_SECRET=change-me
FG_LOG_LEVEL=debug
FG_RATE_LIMIT_RPM=120
FG_MAX_UPLOAD_BYTES=5242880
FG_MAX_UPLOAD_FILES=20
FG_MAX_FILE_BYTES=1048576
FG_SLACK_TIMEOUT_MS=2000
FG_SESSION_DAYS=7
18. Implementation Milestones (Week 1-2, Week 3-4, etc.)
Milestone 1 (Week 1): Repo Skeleton + Health + Config + DB Migration
Goal: Bootable service with migrations and health checks. Tasks (MUST)
Create repo structure (Section 12).
Implement config loader reading all env vars (Section 17.1) with validation.
Implement DB connection (pgx) and readyz DB ping.
Add request ID middleware and structured logging.
Add /healthz and /readyz.
Add migration runner that reads migrations/0001_schema.sql and applies it once in dev:
MVP MUST apply schema at startup only when FG_ENV=dev.
In prod, startup MUST NOT auto-apply migrations.
Definition of Done
docker compose up --build starts app, /healthz returns ok, /readyz returns db ok.
DB contains all tables from Section 9.
Claude Code Prompt (Milestone 1)
text
Code kopieren
Implement Milestone 1 exactly as SSOT:

1) Create the full repository structure in Section 12.
2) Create go.mod and add dependencies exactly from Section 11.2 with pinned versions.
3) Implement config loader in internal/config/config.go reading and validating every env var in Section 17.1.
4) Implement DB connector (pgx) and a dev-only migration runner that applies migrations/0001_schema.sql when FG_ENV=dev.
5) Implement /healthz and /readyz endpoints (Section 8.3.1).
6) Add request_id middleware and structured JSON logging.
7) Add Dockerfile, docker-compose.yml, Makefile, .env.example exactly as SSOT.

Run: docker compose up --build and curl /healthz and /readyz.
Do not add features beyond this milestone.
Milestone 2 (Week 2): Human Auth (Signup/Login/Logout) + Minimal UI
Goal: User can signup/login and see org creation page. Tasks (MUST)
Implement auth endpoints from Section 8.4.
Implement bcrypt hashing + verification.
Implement session JWT cookie.
Implement server-rendered pages:
/signup
/login
after login: /orgs list page and /orgs/new
Implement CSRF protection:
MVP MUST implement a simple double-submit token cookie:
cookie fg_csrf
hidden form field _csrf
MUST match.
Definition of Done
User can signup, login, logout.
Pages render and actions work.
Claude Code Prompt (Milestone 2)
text
Code kopieren
Implement Milestone 2 exactly as SSOT:

- Add bcrypt password hashing, JWT sessions in HttpOnly cookie fg_session.
- Implement POST /api/v1/auth/signup, /login, /logout with exact JSON envelopes from SSOT.
- Add server-rendered pages /signup and /login and /orgs and /orgs/new.
- Implement simple CSRF (double-submit cookie fg_csrf + hidden field _csrf) for HTML form posts.
- Add unit tests for password hashing and JWT validation.

Run go test ./... and verify signup/login via curl and browser.
Do not implement org/project APIs yet beyond pages.
Milestone 3 (Week 3): Orgs + Projects + API Keys + Audit Log
Goal: Create org, create project, generate API key. Tasks (MUST)
Implement org endpoints (Section 8.5):
create org
list orgs
list members (only self membership exists in MVP)
Implement project endpoints (Section 8.6):
create project
list projects
Implement API key endpoints (Section 8.8):
create/list/revoke
hash and store tokens
Implement audit log writes for each action.
UI pages:
org list + create
project list + create
project settings page with “Create API Key” button and “Slack webhook set/clear”
Definition of Done
Logged in user can create org, create project, create API key and copy it once.
API key list shows metadata but never shows token again.
Claude Code Prompt (Milestone 3)
text
Code kopieren
Implement Milestone 3 exactly as SSOT:

1) Implement orgs/projects/api-keys endpoints in Section 8 (with RBAC from Section 13.2).
2) Implement API key token generation and hashing per Section 13.3 (fgk_ prefix + sha256 stored as bytea).
3) Add audit log records for all actions required by Section 7.10.
4) Implement server-rendered UI pages for org/project creation and project settings (slack + keys).

Add integration tests that:
- signup+login
- create org
- create project
- create api key
Verify DB rows exist and token is only returned once.
Milestone 4 (Week 4): Ingestion API + JUnit Parser + Persistence
Goal: GitHub Action (or curl) can upload JUnit and store test results. Tasks (MUST)
Implement POST /api/v1/ingest/junit (Section 8.9):
multipart parsing
validate meta schema fields
enforce hard limits (Section 14.2)
dedupe by unique run attempt/job
Implement JUnit XML parsing:
parse <testsuite> and <testcase>
interpret failure/error/skipped
Persist:
ci_run upsert
ci_run_attempt upsert
ci_job upsert
test_case upsert
test_result upsert
Store truncated failure fields (Section 14.2)
Update last_seen_at for test_cases and ci_runs.
Definition of Done
A curl multipart request with testdata JUnit stores rows.
Duplicate submission does not duplicate results.
Response includes counts.
Claude Code Prompt (Milestone 4)
text
Code kopieren
Implement Milestone 4 exactly as SSOT:

- Add POST /api/v1/ingest/junit with API key auth scope ingest:write.
- Parse multipart with meta JSON + multiple junit files.
- Enforce upload limits: max files 20, max file size 1MB, max total 5MB.
- Parse JUnit XML into canonical test_identifier and status (passed/failed/skipped/error).
- Persist into ci_runs, ci_run_attempts, ci_jobs, test_cases, test_results with the constraints from the SQL schema.
- Implement dedupe so repeating the same run_attempt/job does not duplicate results.
- Add unit tests for junit parsing using testdata.
- Add integration test that ingests and verifies rows.

Do not implement flake detection yet.
Milestone 5 (Week 5): Flake Detection + Flake Stats Aggregation
Goal: Detect fail→pass across attempts and show in API. Tasks (MUST)
Implement flake detector:
on ingest, after writing results, check same run_id+job for prior attempts
if evidence exists, insert flake_events
Maintain flake_stats:
update counts (mixed_outcome_runs, total_runs_seen)
recompute flake_score
store last_failure_message from most recent failed result
Implement read APIs:
list flakes (Section 8.10.1)
flake detail (Section 8.10.2)
Definition of Done
Ingest attempt1 failing + attempt2 passing ⇒ flake_events_created=1
Flake list endpoint shows the test with correct score/counts.
Claude Code Prompt (Milestone 5)
text
Code kopieren
Implement Milestone 5 exactly as SSOT:

- Add flake detection logic per Section 7.7: fail in earlier attempt and pass in later attempt for same github_run_id/job/test.
- Insert flake_events with failed_attempt_number and passed_attempt_number.
- Maintain flake_stats with mixed_outcome_runs, total_runs_seen, flake_score, last_failure_message.
- Implement GET /api/v1/projects/{project_id}/flakes and GET /api/v1/projects/{project_id}/flakes/{test_case_id} with exact response shapes.

Add tests for flake detection using testdata flaky_attempt1.xml and flaky_attempt2.xml.
Milestone 6 (Week 6): Dashboard UI (Flake List + Detail)
Goal: Human-visible dashboard pages. Tasks (MUST)
Implement pages:
/orgs/{org_slug}/projects/{project_slug}/flakes
/orgs/{org_slug}/projects/{project_slug}/flakes/{test_case_id}
Add filters in UI (server-side query params):
days, repo, job_name
Add links to run URLs in evidence.
Definition of Done
After ingesting flaky sample, dashboard shows flake and evidence.
Claude Code Prompt (Milestone 6)
text
Code kopieren
Implement Milestone 6 exactly as SSOT:

- Build server-rendered pages for flake list and flake detail.
- Use the existing API/service layer to query flakes and evidence.
- Implement filter query params days/repo/job_name.
- Display flake_score, counts, last_seen, and evidence with run_url links.
- Keep UI minimal: single CSS file and basic layout template.

Do not add any new JS frameworks.
Milestone 7 (Week 7): Slack Notifications + Project Settings
Goal: Slack messages on new flake evidence. Tasks (MUST)
Implement project Slack endpoints (Section 8.7).
Store webhook URL in DB (already in schema).
On creation of a new flake event:
post Slack message with required fields (Section 7.9)
Ensure webhook URL is never returned in full.
Definition of Done
When slack enabled and new flake event occurs, Slack receives a message.
Ingest still succeeds if Slack fails.
Claude Code Prompt (Milestone 7)
text
Code kopieren
Implement Milestone 7 exactly as SSOT:

- Add PUT /api/v1/projects/{project_id}/slack and DELETE /api/v1/projects/{project_id}/slack.
- Never echo full webhook URL in responses.
- Add Slack client that posts JSON payload to webhook with timeout FG_SLACK_TIMEOUT_MS.
- On new flake_events created, post a Slack message including repo, workflow, job, test identifier, and link to dashboard detail page.
- Slack failures must not fail ingestion.

Add tests using an httptest server as fake Slack webhook.
Milestone 8 (Week 8): GitHub Action (Composite) + Docs + MVP Hardening
Goal: Provide copy-paste integration for GitHub Actions. Tasks (MUST)
Implement composite action under /action:
inputs:
flakeguard_url (required)
project_slug (required)
api_key (required)
junit_paths (required, glob)
job_variant (optional)
behavior:
find junit files by glob
upload multipart to /api/v1/ingest/junit
Write docs:
/docs/github-action.md with sample workflow yaml
/docs/api.md summarizing endpoints (no new details)
/docs/runbook.md (how to rotate keys, clear slack)
Add retention policy (MVP fixed):
A daily cron job MUST delete:
junit_files content older than 30 days (set content NULL)
flake_events older than 180 days
Stats MUST remain (flake_stats) but counts only reflect last 30 days window for flake marking in UI (as in Section 7.7).
Definition of Done
GitHub Action example works in a repo and ingestion succeeds.
Docs exist and are accurate.
Cron retention runs in dev with a short interval when FG_ENV=dev (MAY be configured to 1 minute for dev).
Claude Code Prompt (Milestone 8)
text
Code kopieren
Implement Milestone 8 exactly as SSOT:

- Create a composite GitHub Action in /action with action.yml and upload.sh.
- Implement file globbing in bash (use find + pattern) and upload multipart via curl.
- Document usage in /docs/github-action.md with a full example workflow yaml.
- Add retention cron job using robfig/cron: in prod daily at 03:00 UTC; in dev every minute.
- Retention deletes junit_files content older than 30 days (content=NULL, content_truncated stays true/false) and deletes flake_events older than 180 days.

Do not add billing or GitHub App integration.
19. MVP Acceptance Criteria (Checkboxen)
19.1 Core
 docker compose up --build starts app and Postgres successfully
 /healthz returns 200 with {status:"ok"}
 /readyz returns 200 only if DB is reachable
 User can signup/login/logout
 User can create an org and project
 User can create an API key and copy it once
 Ingest endpoint rejects missing/invalid meta
 Ingest endpoint enforces upload limits (files/bytes)
 Ingest stores test results and is idempotent for repeats
 Flake event is created for fail-attempt1 → pass-attempt2
 Flake list API returns correct score and counts
 Dashboard shows flaky tests and evidence
 Slack notification posts on new flake event (when enabled)
 Slack failures do not break ingest
 Retention job runs and removes old raw junit content
19.2 Security
 API key stored only as sha256 hash (no plaintext)
 Password stored as bcrypt hash (cost 12)
 Auth headers never logged
 Webhook URL never fully returned
20. Out of Scope (Explizit)
The following MUST NOT be built in MVP:
Billing, subscriptions, Stripe integration
GitHub App installation flow or OAuth login
Automatic PR creation / quarantine PRs
Multi-user invites and full membership management
SSO (SAML/OIDC), SCIM
Support for CI providers other than GitHub Actions
Non-JUnit formats (TRX, NUnit, etc.)
LLM/AI features (this is not an “AI for X” product)
21. Anhänge (Beispiel-Payloads, Config-Beispiele)
21.1 Example curl ingestion
bash
Code kopieren
curl -X POST "$FG_BASE_URL/api/v1/ingest/junit" \
  -H "Authorization: Bearer $FLAKEGUARD_API_KEY" \
  -F 'meta={
    "project_slug":"gha-ci",
    "repo_full_name":"acme/payment-service",
    "workflow_name":"CI",
    "workflow_ref":".github/workflows/ci.yml",
    "github_run_id":1234567890,
    "github_run_attempt":1,
    "github_run_number":421,
    "run_url":"https://github.com/acme/payment-service/actions/runs/1234567890",
    "sha":"a3c1e7b9f2a0",
    "branch":"main",
    "event":"push",
    "pr_number":null,
    "job_name":"test",
    "job_variant":"ubuntu-latest",
    "started_at":"2025-12-25T09:58:00Z",
    "completed_at":"2025-12-25T10:03:12Z"
  };type=application/json' \
  -F "junit=@testdata/junit/failing.xml" \
  -F "junit=@testdata/junit/passing.xml"
21.2 GitHub Actions Workflow Example (MVP)
yaml
Code kopieren
name: CI
on:
  push:
    branches: [ "main" ]
  pull_request:

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run tests
        run: |
          mkdir -p test-results
          # Your test command must produce JUnit XML into test-results/
          # Example placeholders:
          # go test ./... -json | go-junit-report > test-results/junit.xml

      - name: Upload JUnit to FlakeGuard
        if: always()
        uses: ./.github/actions/flakeguard-upload
        with:
          flakeguard_url: ${{ secrets.FLAKEGUARD_URL }}
          project_slug: gha-ci
          api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
          junit_paths: "test-results/**/*.xml"
          job_variant: "ubuntu-latest"
22. Erster Claude Code Prompt
Paste this as the very first Claude Code CLI prompt after creating an empty repo folder.
text
Code kopieren
You are implementing FlakeGuard exactly per SSOT.md.

Rules:
- Do exactly what SSOT.md says. No extra features.
- Use Go 1.22.2 + PostgreSQL 16.4 + Chi router.
- Pin dependencies exactly as in SSOT Section 11.
- Create the repository structure exactly as in SSOT Section 12.
- Implement milestones in order. Start with Milestone 1 only.
- After Milestone 1, run docker compose up --build and verify /healthz and /readyz.

Now implement Milestone 1:
1) Create folders/files per SSOT Section 12.
2) Create go.mod/go.sum with pinned deps.
3) Implement config loader, DB connection, dev-only migration runner, request-id middleware, logging, health endpoints.
4) Add Dockerfile, docker-compose.yml, Makefile, .env.example.

Do not ask questions. The SSOT is complete. Stop after Milestone 1 is done and tests pass.
23. Änderungslog
1.0.0 (2025-12-25): Initial SSOT for FlakeGuard MVP (GitHub Actions JUnit ingest, rerun-based flake detection, dashboard, Slack).
makefile
Code kopieren


::contentReference[oaicite:0]{index=0}






