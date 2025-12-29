# Development

## Prerequisites

- Go 1.22+
- Docker (for Postgres and integration tests)

## Local run (Docker Compose)

```bash
docker compose up --build
```

- App: `http://localhost:8080`
- Health: `GET /healthz`, `GET /readyz`

## Local run (host Go + external Postgres)

1. Start Postgres (locally or via Docker).
2. Export required env vars (see `.env.example`).
3. Run:

```bash
go run ./cmd/flakeguard
```

Notes:

- `FG_ENV=dev` runs DB migrations automatically on startup.
- `FG_ENV=prod` does not auto-run migrations.

## Tests

Unit + package tests:

```bash
go test ./... -count=1
```

Integration tests:

- Require Docker (they spin up a `postgres:16` container).

```bash
go test ./internal/integration -count=1
```

## Formatting

```bash
gofmt -w .
```

