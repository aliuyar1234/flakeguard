# Contributing

Thanks for considering contributing to FlakeGuard!

## Development setup

Prerequisites:

- Go 1.22.x
- Docker + Docker Compose (for Postgres and integration tests)

Quick start:

```bash
docker compose up --build
```

Run tests:

```bash
go test ./...
```

Format code:

```bash
gofmt -w .
```

## Pull requests

- Keep changes focused and small when possible.
- Add/adjust tests for behavior changes.
- Update docs when adding endpoints or changing UX.
- Ensure `go test ./...` passes.

## Reporting issues

Please use GitHub Issues and include:

- FlakeGuard version/commit
- Environment (OS, Postgres version, Docker, Go)
- Steps to reproduce
- Logs (redact secrets)

