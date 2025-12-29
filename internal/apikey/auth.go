package apikey

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aliuyar1234/flakeguard/internal/apikeys"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrMissingAPIKey is returned when no API key is provided
	ErrMissingAPIKey = errors.New("missing API key in Authorization header")

	// ErrInvalidAPIKey is returned when the API key is invalid
	ErrInvalidAPIKey = errors.New("invalid API key")

	// ErrRevokedAPIKey is returned when the API key has been revoked
	ErrRevokedAPIKey = errors.New("API key has been revoked")

	// ErrExpiredAPIKey is returned when the API key has expired
	ErrExpiredAPIKey = errors.New("API key has expired")

	// ErrInsufficientScope is returned when the API key doesn't have required scope
	ErrInsufficientScope = errors.New("API key does not have required scope")
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// contextKeyAPIKey is the context key for storing the authenticated API key
	contextKeyAPIKey contextKey = "apikey"

	// contextKeyProjectID is the context key for storing the project ID
	contextKeyProjectID contextKey = "project_id"
)

// ExtractAPIKey extracts the API key token from the Authorization header
// Expected format: "Authorization: Bearer <token>"
func ExtractAPIKey(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", ErrMissingAPIKey
	}

	// Check for Bearer scheme
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("invalid Authorization header format, expected 'Bearer <token>'")
	}

	token := parts[1]
	if token == "" {
		return "", ErrMissingAPIKey
	}

	return token, nil
}

// HashToken hashes an API key token using SHA-256
func HashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

// ValidateAPIKey validates an API key token and returns the API key
func ValidateAPIKey(ctx context.Context, pool *pgxpool.Pool, token string) (*apikeys.ApiKey, error) {
	// Hash the token
	tokenHash := HashToken(token)

	// Look up API key by hash
	service := apikeys.NewService(pool)
	key, err := service.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, apikeys.ErrAPIKeyNotFound) {
			return nil, ErrInvalidAPIKey
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Check if revoked
	if key.RevokedAt.Valid {
		return nil, ErrRevokedAPIKey
	}

	// Check if expired
	if key.ExpiresAt.Valid && !key.ExpiresAt.Time.After(time.Now()) {
		return nil, ErrExpiredAPIKey
	}

	return key, nil
}

// ValidateScope checks if the API key has the required scope
func ValidateScope(key *apikeys.ApiKey, requiredScope apikeys.ApiKeyScope) error {
	for _, scope := range key.Scopes {
		if scope == string(requiredScope) {
			return nil
		}
	}
	return ErrInsufficientScope
}

// UpdateLastUsed updates the last_used_at timestamp for an API key
func UpdateLastUsed(ctx context.Context, pool *pgxpool.Pool, apiKeyID uuid.UUID) error {
	service := apikeys.NewService(pool)
	return service.UpdateLastUsed(ctx, apiKeyID)
}

// WithAPIKey adds the API key to the request context
func WithAPIKey(ctx context.Context, key *apikeys.ApiKey) context.Context {
	return context.WithValue(ctx, contextKeyAPIKey, key)
}

// GetAPIKey retrieves the API key from the request context
func GetAPIKey(ctx context.Context) *apikeys.ApiKey {
	key, ok := ctx.Value(contextKeyAPIKey).(*apikeys.ApiKey)
	if !ok {
		return nil
	}
	return key
}

// WithProjectID adds the project ID to the request context
func WithProjectID(ctx context.Context, projectID uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyProjectID, projectID)
}

// GetProjectID retrieves the project ID from the request context
func GetProjectID(ctx context.Context) uuid.UUID {
	projectID, ok := ctx.Value(contextKeyProjectID).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return projectID
}
