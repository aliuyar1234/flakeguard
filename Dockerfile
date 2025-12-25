# Multi-stage build for FlakeGuard
# Stage 1: Build
FROM golang:1.22.2-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o flakeguard ./cmd/flakeguard

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder (migrations are embedded)
COPY --from=builder /build/flakeguard .

# Copy web assets
COPY --from=builder /build/web ./web

# Create non-root user
RUN addgroup -g 1000 flakeguard && \
    adduser -D -u 1000 -G flakeguard flakeguard && \
    chown -R flakeguard:flakeguard /app

USER flakeguard

EXPOSE 8080

CMD ["./flakeguard"]
