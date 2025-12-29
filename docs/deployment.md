# Deployment

## Prerequisites

- PostgreSQL 16+ (recommended)
- Reverse proxy / load balancer terminating TLS (recommended)

## Configuration

FlakeGuard is configured via environment variables (see `.env.example`).

Production recommendations:

- Set `FG_ENV=prod`
- Set `FG_BASE_URL` to your public HTTPS URL (used for links)
- Use a strong `FG_JWT_SECRET` (32+ chars; treat as a secret)
- Run Postgres backups (at least daily) and test restore

## Migrations

- `FG_ENV=dev` runs migrations automatically on startup.
- `FG_ENV=prod` requires applying migrations before starting the new version.

Migrations are in `migrations/*.sql` and can be applied in order. FlakeGuard also embeds migrations and can run them in-process when needed (see `internal/db`).

## Health checks

- Liveness: `GET /healthz` (always `200` if process is up)
- Readiness: `GET /readyz` (returns `200` only if Postgres is reachable)

## Backups

- Take regular `pg_dump` backups.
- Before upgrades, take a fresh backup and apply migrations first.

## Upgrades

Recommended order:

1. Backup Postgres
2. Apply DB migrations
3. Deploy the new app version
4. Verify `GET /readyz` and basic dashboard flows

