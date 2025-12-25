.PHONY: build test run migrate clean help

# Build variables
BINARY_NAME=flakeguard
BUILD_DIR=bin
GO_FILES=$(shell find . -name '*.go' -type f)

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/flakeguard

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Run the application (requires .env file)
run:
	@echo "Running $(BINARY_NAME)..."
	go run ./cmd/flakeguard

# Run database migrations
migrate:
	@echo "Running migrations..."
	@echo "Note: Migrations auto-run when FG_ENV=dev"
	@echo "Use 'make run' or 'docker compose up' to apply migrations"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	go clean

# Show help
help:
	@echo "FlakeGuard Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build    - Build the application binary"
	@echo "  test     - Run all tests with race detection"
	@echo "  run      - Run the application locally"
	@echo "  migrate  - Show migration instructions"
	@echo "  clean    - Remove build artifacts"
	@echo "  help     - Show this help message"
