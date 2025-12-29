# Product Backlog (Public)

This is a public, high-signal checklist of work that would make FlakeGuard feel “production-ready” and GitHub‑publishable (docs/tests/ops polish), without relying on private specs.

## P0 — Must‑Have Before “proud to publish”

### Open source hygiene
- [x] Pick a license and add `LICENSE` (MIT/Apache-2.0/etc).
- [x] Add `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, and `SECURITY.md`.
- [x] Add `.github/ISSUE_TEMPLATE/*` + `.github/pull_request_template.md`.
- [x] Add a “Support” section (how to report bugs, security issues, and feature requests).

### CI / quality gates
- [x] Add GitHub Actions CI: `go test ./...`, `go vet ./...`, `gofmt -l`, and `docker build`.
- [x] Add linting (`golangci-lint`) and enforce it in CI.
- [x] Add ShellCheck for `action/*.sh`.
- [x] Add a minimal “smoke” integration job that boots Postgres and runs integration tests.

### Public docs completeness
- [x] Update `docs/api.md` to include the new org settings endpoints (invites/members/audit).
- [x] Add `docs/development.md` (local dev, tests, migrations, env vars).
- [x] Add `docs/deployment.md` (prod guidance: TLS, migrations, backups, upgrades).
- [x] Document the GitHub Action versioning strategy (`uses: owner/repo@vX`).

### Critical test coverage gaps
- [ ] Add integration tests for: org invites flow (create → accept), member role changes, member removal, and “last OWNER” guardrails.
- [ ] Add tests for audit entries for invites/membership changes.

### Repo/module identity
- [x] Decide on the canonical Go module path:
  - Option A: move repo to `github.com/flakeguard/flakeguard`
  - Option B: change `go.mod` + all imports to match the current GitHub repo path (`github.com/aliuyar1234/flakeguard`)
- [x] Ensure README examples match the chosen repo/module path.

## P1 — Strong product polish (high value)

### Security & hardening
- [ ] Add standard security headers middleware (CSP, X-Content-Type-Options, Referrer-Policy, etc.).
- [ ] Add account email verification or an explicit “no email verification” stance in docs.
- [ ] Add password reset flow (or an admin CLI flow) so users don’t get locked out.
- [ ] Add API key expiration + rotation UX (optional but strongly recommended).
- [ ] Add org audit log pagination + filtering (action/type/actor).

### UX improvements
- [ ] Add pagination and search to flakes list (by test name/class, branch, timeframe).
- [ ] Add “copy” buttons for API keys/invite links (client-side convenience).
- [ ] Improve onboarding: first-run wizard (create org → project → API key → GH action snippet).
- [ ] Add consistent navigation: org switcher + per-org settings link everywhere.

### Observability / operations
- [ ] Add Prometheus metrics endpoint (requests, ingestions, slack deliveries, DB pool stats).
- [ ] Add structured logging fields consistently (org_id, project_id, request_id).
- [ ] Add a safe “diagnostics” page/endpoint for admins (redacted config, build info).

### Data lifecycle / retention
- [ ] Make retention days configurable via env vars (currently hard-coded).
- [ ] Add distributed lock for retention job to avoid multi-instance double-runs.
- [ ] Add “export” primitives (CSV/JSON export for flakes and audit log).

## P2 — Competitive differentiators (nice-to-have)

### Integrations
- [ ] Slack: batch notifications per CI run + rich Block Kit formatting + deep links to flakes.
- [ ] GitHub: optionally comment on PRs / create GitHub Check annotations for flaky tests.
- [ ] CLI uploader (in addition to the GitHub Action) for local and other CI providers.

### Product depth
- [ ] Per-variant and per-branch flake scoring (separate stats by `job_variant`/branch).
- [ ] “Mute/ignore” rules (per test, glob patterns, or tags) with audit trail.
- [ ] Multi-project dashboards (org-wide trends, top regressions).

### Performance/scaling
- [ ] Background ingestion processing (queue) for large JUnit uploads.
- [ ] Add query-level performance monitoring and tune indexes for list endpoints.
