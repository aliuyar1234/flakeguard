# Extra Implementations (Beyond `specs/**/tasks.md`)

This file tracks product work that was implemented outside the spec milestone `tasks.md` checklists (which are kept private/ignored in git).

## 2025-12-29 — Org invites + member management + audit viewer

**Primary commit:** `e4a189b`

### Organization invitations (shareable links)
- Added DB support for org invites: `migrations/0003_org_invites.sql`.
  - New table: `org_invites` (stores `token_hash`, never plaintext token).
  - Added `updated_at` to `org_memberships` + trigger.
- Added invite token utilities: `internal/orgs/invite_token.go` (`fgi_` prefix, SHA-256 hash stored).
- Added invite API:
  - `POST /api/v1/orgs/{org_id}/invites` (OWNER/ADMIN; Admin cannot create Admin invites)
  - `GET /api/v1/orgs/{org_id}/invites` (OWNER/ADMIN; active, unexpired only)
  - `DELETE /api/v1/orgs/{org_id}/invites/{invite_id}` (OWNER/ADMIN)
  - `POST /api/v1/orgs/invites/accept` (logged-in user; email must match invite email)
- Invite behavior:
  - Invites expire after **7 days**.
  - Creating a new invite for the same org+email automatically revokes any existing active invite for that email.
  - No email sending yet; the UI returns an **accept URL** you can copy/share.

### Membership management (role changes + removal)
- Added member management API:
  - `PUT /api/v1/orgs/{org_id}/members/{user_id}` (role update)
  - `DELETE /api/v1/orgs/{org_id}/members/{user_id}` (remove member / leave org)
- Guardrails:
  - Cannot remove/demote the **last OWNER**.
  - ADMIN cannot change/remove OWNER/ADMIN (and cannot promote to ADMIN/OWNER).

### Org settings UI (members + invites + audit)
- New settings page: `GET /orgs/{org_id}/settings`
  - Template: `web/templates/org_settings.html`
  - Handler: `internal/web/org_settings_handlers.go`
- New invite accept page: `GET /invites/accept?token=...`
  - Template: `web/templates/invite_accept.html`
  - Handler: `internal/web/org_settings_handlers.go`
- Updated UI token reveal logic to support invites:
  - `web/static/app.js` now supports `body.data.invite.accept_url` as `data-token-target`.

### Better auth redirect UX (`next=...`)
- `internal/auth/middleware.go`: `RequireAuthPage` now redirects unauthenticated users to `/login?next=<original>`.
- `internal/web/handlers.go` + templates:
  - Login/signup pages preserve `next` and redirect back to the intended page after auth.
  - `web/templates/login.html`, `web/templates/signup.html` use `{{.Redirect}}` and preserve `next` in links.

### Audit log viewing + new events
- Added audit reader: `internal/audit/reader.go`
- Added org audit API:
  - `GET /api/v1/orgs/{org_id}/audit?limit=50` (OWNER/ADMIN)
- Added new audit events in `internal/audit/audit.go`:
  - `org.invite_created`, `org.invite_revoked`, `org.invite_accepted`
  - `org.member_role_updated`, `org.member_removed`
- Org Settings shows the most recent audit events (currently last 25) for quick visibility.

