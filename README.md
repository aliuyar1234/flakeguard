# FlakeGuard

FlakeGuard is a test flake detection and monitoring system for GitHub Actions workflows.

## Quick Start

### For GitHub Actions Users

Add FlakeGuard to your GitHub Actions workflow to automatically detect and track flaky tests:

```yaml
- name: Upload test results to FlakeGuard
  uses: ./action  # Or your-org/flakeguard-action@v1 when published
  if: always()  # Upload even if tests fail
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/**/*.xml'
```

**Documentation:**
- [GitHub Action Usage](docs/github-action.md) - Complete integration guide
- [API Reference](docs/api.md) - REST API documentation
- [Example Workflow](examples/github-workflow.yml) - Full working example
- [Runbook](docs/runbook.md) - Operational guide

### For Self-Hosting FlakeGuard

### Prerequisites

- Docker and Docker Compose
- Go 1.22.2 (for local development without Docker)

### Running with Docker Compose

1. Start the service and database:

```bash
docker compose up --build
```

2. The service will be available at `http://localhost:8080`

3. Check health endpoints:

```bash
# Liveness check
curl http://localhost:8080/healthz

# Readiness check (includes database connectivity)
curl http://localhost:8080/readyz
```

### Local Development

1. Copy the example environment file:

```bash
cp .env.example .env
```

2. Start a PostgreSQL database:

```bash
docker compose up postgres
```

3. Run the application:

```bash
make run
```

Or build and run:

```bash
make build
./bin/flakeguard
```

## Configuration

FlakeGuard follows the 12-factor app methodology and is configured via environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `FG_DATABASE_DSN` | Yes | - | PostgreSQL connection string |
| `FG_JWT_SECRET` | Yes | - | JWT signing secret (min 32 bytes) |
| `FG_HTTP_ADDR` | No | `:8080` | HTTP server bind address |
| `FG_LOG_LEVEL` | No | `info` | Log level (debug, info, warn, error) |
| `FG_ENV` | No | `prod` | Environment (dev, prod). Auto-runs migrations when `dev` |
| `FG_SESSION_DAYS` | No | `7` | Session validity in days |
| `FG_RATE_LIMIT_RPM` | No | `120` | Rate limit (requests per minute per API key) |
| `FG_MAX_UPLOAD_BYTES` | No | `5242880` | Max total upload size (5MB) |
| `FG_MAX_FILE_BYTES` | No | `1048576` | Max single file size (1MB) |
| `FG_SLACK_TIMEOUT_MS` | No | `2000` | Slack webhook timeout in milliseconds |
| `FG_BASE_URL` | No | - | Base URL for dashboard (used in Slack notifications) |
| `FG_RETENTION_JUNIT_DAYS` | No | `30` | Days to retain JUnit file content |
| `FG_RETENTION_EVENTS_DAYS` | No | `180` | Days to retain flake events |

### Example Configuration

```bash
FG_DATABASE_DSN=postgres://user:pass@localhost:5432/flakeguard?sslmode=disable
FG_JWT_SECRET=your-secret-minimum-32-bytes-required
FG_HTTP_ADDR=:8080
FG_LOG_LEVEL=info
FG_ENV=dev
```

## Project Structure

```
flakeguard/
├── cmd/flakeguard/       # Application entry point
├── internal/
│   ├── app/              # HTTP handlers and application logic
│   ├── config/           # Configuration management
│   ├── db/               # Database connection and migrations
│   ├── flake/            # Flake detection and statistics
│   ├── ingest/           # JUnit ingestion and parsing
│   ├── retention/        # Data retention and cleanup
│   ├── slack/            # Slack notifications
│   └── ...               # Other internal packages
├── migrations/           # SQL migration files
├── web/
│   ├── templates/        # HTML templates
│   └── static/           # CSS, JS, images
├── action/               # GitHub Action for JUnit upload
├── docs/                 # Documentation
├── examples/             # Example workflows
└── testdata/            # Test fixtures
```

## Development

### Running Tests

```bash
make test
```

### Building

```bash
make build
```

The binary will be created at `./bin/flakeguard`.

### Database Migrations

In development mode (`FG_ENV=dev`), migrations run automatically on startup.

In production mode (`FG_ENV=prod`), migrations must be run manually before deploying.

## Features

- **Automated Flake Detection**: Analyzes JUnit test results to identify flaky tests
- **GitHub Actions Integration**: Drop-in action for seamless CI/CD integration
- **Slack Notifications**: Get alerted when new flakes are detected
- **Retention Policies**: Automatic cleanup of old data to manage storage
- **API Access**: REST API for programmatic access to flake data
- **Multi-Project Support**: Organize tests across multiple projects
- **Job Variants**: Track flakes across different test configurations (OS, language versions, etc.)

## API Endpoints

### Public Endpoints

- `GET /healthz` - Liveness check (always returns 200 if service is running)
- `GET /readyz` - Readiness check (returns 200 only if database is connected)

### Authenticated API Endpoints

- `POST /api/v1/ingest/junit` - Upload JUnit test results (requires API key)
- `GET /api/v1/projects/{id}/flakes` - Retrieve flaky tests for a project (requires API key)

Full API documentation: [docs/api.md](docs/api.md)

## License

Copyright 2025 FlakeGuard
