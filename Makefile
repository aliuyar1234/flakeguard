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
