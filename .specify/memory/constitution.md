<!--
  Sync Impact Report
  ===================
  Version change: (new) → 1.0.0

  Added sections:
  - Core Principles (10 principles from SSOT)
  - Security Requirements
  - Development Workflow
  - Governance

  Modified principles: N/A (initial version)
  Removed sections: N/A (initial version)

  Templates requiring updates:
  - .specify/templates/plan-template.md: ✅ Compatible (Constitution Check section exists)
  - .specify/templates/spec-template.md: ✅ Compatible (Requirements section aligns)
  - .specify/templates/tasks-template.md: ✅ Compatible (Phase structure matches milestones)

  Follow-up TODOs: None
-->

# FlakeGuard Constitution

## Core Principles

### I. SSOT Supremacy (NON-NEGOTIABLE)

The SSOT document (`FlakeGuard Spec.md`) is the **only** authoritative truth for what MUST be built.

- If anything conflicts (README, code comments, tickets, Slack), **SSOT wins**
- Anything not explicitly specified is **OUT OF SCOPE** and MUST NOT be implemented
- No "reasonable assumptions" are allowed - if detail is missing, implementation MUST stop and SSOT MUST be updated first

**Rationale**: Prevents scope creep, ensures alignment, eliminates ambiguity.

### II. Deterministic & Testable Behavior

All behavior MUST be deterministic and testable.

- Every feature MUST have unit tests for core logic
- Every persistence layer MUST have integration tests
- Acceptance criteria in SSOT Section 19 MUST be covered
- Tests MUST pass before merging

**Rationale**: Flaky tests are the enemy we're fighting - our own system MUST NOT be flaky.

### III. Multi-Tenant First

The system MUST be multi-tenant from day 1 (Organizations), even if single-user initially.

- Every query MUST enforce org/project scoping
- No data leakage between tenants
- API keys scoped to projects, not global

**Rationale**: Retrofitting multi-tenancy is expensive; bake it in from the start.

### IV. 12-Factor Configuration

All config MUST be provided via environment variables. No hidden config files in production.

- All env vars documented in SSOT Section 17.1
- `.env.example` MUST stay in sync with supported variables
- Secrets (JWT, DB credentials) MUST NOT be hardcoded

**Rationale**: Enables deployment flexibility, prevents secrets in version control.

### V. Security by Default

Security is not optional. These rules are NON-NEGOTIABLE:

- API keys stored as SHA256 hashes only (never plaintext)
- Passwords stored using bcrypt with cost 12
- All inbound requests authenticated and rate-limited
- Sensitive data MUST NOT appear in logs (no raw API keys, no JUnit payload dumps)
- Webhook URLs never echoed in API responses

**Rationale**: One leak destroys trust. Defense in depth.

### VI. Idempotent Operations

All mutations MUST be idempotent where possible.

- Migrations MUST run cleanly on fresh PostgreSQL and re-run without error
- Ingestion MUST be idempotent for same (run_id, attempt, job) - no duplicate results
- Slack posting failures MUST NOT fail ingestion

**Rationale**: Retries and reruns are normal in CI - handle them gracefully.

### VII. Feature-Flagged Integrations

All external integrations MUST be feature-flagged and safe-by-default.

- Slack webhook optional (if not set, no notifications)
- No destructive external actions without explicit enablement
- External failures MUST be logged but not propagate to user-facing errors

**Rationale**: Third-party outages should not break core functionality.

### VIII. API Stability

All API responses MUST be stable and backward-compatible within same major version (`v1`).

- Stable error envelope: `{error: {code, message, request_id}}`
- Stable success envelope: `{request_id, data}`
- IDs MUST be UUID v4
- Timestamps MUST be RFC3339 with timezone
- No breaking changes without major version bump

**Rationale**: Consumers depend on contract stability.

### IX. Simplicity & YAGNI

Start simple. Only build what SSOT specifies.

- Single Go binary for MVP (API + UI + background jobs)
- Server-rendered HTML (no SPA framework)
- No external linters beyond `go vet` in MVP
- Dependencies pinned exactly as SSOT Section 11.2

**Rationale**: Complexity is the enemy of delivery. Add only when proven necessary.

### X. Milestone-Driven Development

Implementation MUST follow SSOT Section 18 milestones in order.

- Each milestone is commit-ready
- Tests MUST run after each milestone
- Failures MUST be fixed before continuing
- No scope expansion beyond current milestone

**Rationale**: Predictable progress, working software at each step.

## Security Requirements

### Authentication & Authorization

| Aspect | Requirement |
|--------|-------------|
| Human Auth | JWT in HttpOnly cookie `fg_session`, HS256 signed |
| Agent Auth | Bearer token `fgk_<base64url>`, stored as sha256 hash |
| RBAC Roles | OWNER, ADMIN, MEMBER, VIEWER |
| Session Duration | Configurable via `FG_SESSION_DAYS` (default 7) |

### Hard Limits (DoS Prevention)

| Limit | Value |
|-------|-------|
| Max files per ingest | 20 |
| Max size per file | 1 MB |
| Max total upload | 5 MB |
| Failure output stored | 8 KB max |
| Failure message stored | 1 KB max |
| Rate limit | `FG_RATE_LIMIT_RPM` per API key |

### Audit Events (MUST log)

- User signup, login failures (rate-limited)
- Org/project creation
- API key creation/revocation
- Slack webhook changes

## Development Workflow

### Definition of Done

A feature is DONE only if:

1. Implemented exactly as SSOT specifies
2. Has unit tests for core logic, integration tests for persistence
3. Accessible through API (and UI where required)
4. Documented minimally in `/docs/`
5. Covered by acceptance criteria in SSOT Section 19

### Code Quality Gates

- `go fmt ./...` - formatting
- `go vet ./...` - static analysis
- `go test ./...` - all tests pass
- No TODO comments for deferred work (update SSOT instead)

### Commit Discipline

- Small, commit-ready changes per milestone
- Tests pass before commit
- No WIP commits to main branch

## Governance

### Amendment Process

1. Propose change to SSOT first (this constitution derives from SSOT)
2. Document rationale and impact
3. Update constitution to reflect SSOT changes
4. Propagate to dependent templates
5. Increment version per semantic versioning

### Version Policy

- **MAJOR**: Backward incompatible principle removal or redefinition
- **MINOR**: New principle added or materially expanded guidance
- **PATCH**: Clarifications, wording fixes, non-semantic refinements

### Compliance Review

- All PRs MUST verify compliance with Core Principles
- Constitution Check in plan-template.md MUST pass before implementation
- Complexity MUST be justified against Principle IX (Simplicity)

**Version**: 1.0.0 | **Ratified**: 2025-12-25 | **Last Amended**: 2025-12-25
