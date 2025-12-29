# FlakeGuard

FlakeGuard is a test flake detection and monitoring system for GitHub Actions workflows.

## Quick Start

### GitHub Actions

Add this step to your workflow after running tests:

```yaml
- name: Upload test results to FlakeGuard
  uses: aliuyar1234/flakeguard/action@v1
  if: always() # Upload even if tests fail
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/**/*.xml'
```

Documentation:

- `docs/github-action.md`
- `docs/api.md`
- `docs/runbook.md`
- `examples/github-workflow.yml`

### Self-Hosting

Prerequisites:

- Docker + Docker Compose
- Go 1.22.x (optional, for local development)

Run with Docker Compose:

```bash
docker compose up --build
```

Health endpoints:

```bash
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

## Configuration

FlakeGuard uses environment variables (see `.env.example`).

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `FG_ENV` | Yes | - | `dev` or `prod` |
| `FG_HTTP_ADDR` | No | `:8080` | HTTP bind address |
| `FG_BASE_URL` | Yes | - | Base URL used for links (e.g., Slack) |
| `FG_DB_DSN` | Yes | - | PostgreSQL DSN |
| `FG_JWT_SECRET` | Yes | - | Session/JWT secret (min 32 chars) |
| `FG_LOG_LEVEL` | No | `info` | `debug`, `info`, `warn`, `error` |
| `FG_RATE_LIMIT_RPM` | No | `120` | Requests per minute per API key |
| `FG_MAX_UPLOAD_BYTES` | No | `5242880` | Max total upload bytes |
| `FG_MAX_UPLOAD_FILES` | No | `20` | Max number of uploaded files |
| `FG_MAX_FILE_BYTES` | No | `1048576` | Max size per uploaded file |
| `FG_SLACK_TIMEOUT_MS` | No | `2000` | Slack webhook timeout (ms) |
| `FG_SESSION_DAYS` | No | `7` | Session validity in days |

## Endpoints (MVP)

- `GET /healthz` (liveness)
- `GET /readyz` (readiness; checks DB connectivity)
- `POST /api/v1/ingest/junit` (agent upload; requires Bearer API key)
- Dashboard APIs under `/api/v1/auth`, `/api/v1/orgs`, `/api/v1/projects` (requires session cookie)

## Project Structure

```
flakeguard/
  cmd/flakeguard/          # Application entry point
  internal/                # Application packages
  migrations/              # SQL migrations
  web/                     # Server-rendered dashboard (templates + static)     
  action/                  # GitHub Action (composite) for JUnit upload
  docs/                    # Documentation
  examples/                # Example workflows
```

## Support

- Bug reports / feature requests: GitHub Issues
- Security issues: see `SECURITY.md`
