# FlakeGuard Runbook (MVP)

Operational notes for running and maintaining FlakeGuard.

## API Key Management

### Create a Key

- Go to the dashboard: Organizations → Projects → Project Settings → API Keys.
- Create a key and copy the token (it is shown only once).
- Store it in CI secrets (e.g., `secrets.FLAKEGUARD_API_KEY`).

### Rotate a Key

1. Create a new key.
2. Update CI secrets to use the new token.
3. Verify ingestion works (check CI logs + FlakeGuard `/readyz`).
4. Revoke the old key.

### Revoke a Key

- In Project Settings, revoke the key. Requests using that token will fail.

## Slack Webhook Configuration

- Configure Slack in Project Settings → Slack Integration.
- The API never returns the full webhook URL; it only reports whether it is set.
- Removing Slack integration disables notifications for that project.

## Retention Policy (Fixed)

Retention runs automatically:

- Clears raw `junit_files.content` older than **30 days** (sets `content = NULL`, keeps metadata).
- Deletes `flake_events` older than **180 days**.
- Does **not** delete `flake_stats` (aggregated stats remain).

Schedule:

- `FG_ENV=prod`: daily at **03:00 UTC**
- `FG_ENV=dev`: every minute

## Database Maintenance

- Take regular Postgres backups (`pg_dump`) before upgrades.
- Run migrations before deploying new versions in production (`FG_ENV=prod`).

## Troubleshooting

- Health:
  - `GET /healthz` should return 200 if the service is running.
  - `GET /readyz` should return 200 only if Postgres is reachable.
- Ingestion failures:
  - `400 invalid_meta`: the uploaded `meta` JSON is missing required fields or uses invalid RFC3339 timestamps.
  - `401`: API key missing/invalid/revoked or wrong scope.
  - `413`: upload too large (adjust `FG_MAX_UPLOAD_BYTES`, `FG_MAX_UPLOAD_FILES`, `FG_MAX_FILE_BYTES`).
- Slack:
  - Slack webhook failures are logged but must not fail ingestion.
