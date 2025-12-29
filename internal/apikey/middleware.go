package apikey

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aliuyar1234/flakeguard/internal/apikeys"
	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/go-chi/httprate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"
)

// RequireAPIKey is middleware that validates API key authentication
// It checks for a valid API key in the Authorization header and validates the required scope
func RequireAPIKey(pool *pgxpool.Pool, requiredScope apikeys.ApiKeyScope) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract API key from Authorization header
			token, err := ExtractAPIKey(r)
			if err != nil {
				if errors.Is(err, ErrMissingAPIKey) {
					apperrors.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Missing Authorization header")
					return
				}
				apperrors.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Invalid Authorization header")
				return
			}

			// Validate API key
			key, err := ValidateAPIKey(ctx, pool, token)
			if err != nil {
				if err == ErrInvalidAPIKey || err == ErrRevokedAPIKey {
					apperrors.WriteError(w, r, http.StatusUnauthorized, "invalid_api_key", "Invalid API key")
					return
				}
				log.Error().Err(err).Msg("Failed to validate API key")
				apperrors.WriteInternalError(w, r, "Authentication failed")
				return
			}

			// Validate scope
			if err := ValidateScope(key, requiredScope); err != nil {
				apperrors.WriteError(w, r, http.StatusForbidden, "forbidden", fmt.Sprintf("API key missing required scope: %s", requiredScope))
				return
			}

			// Update last_used_at timestamp (fire and forget)
			go func() {
				if err := UpdateLastUsed(ctx, pool, key.ID); err != nil {
					log.Error().Err(err).Str("api_key_id", key.ID.String()).Msg("Failed to update last_used_at")
				}
			}()

			// Add API key and project ID to context
			ctx = WithAPIKey(ctx, key)
			ctx = WithProjectID(ctx, key.ProjectID)

			// Continue to next handler
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RateLimitByAPIKey creates a rate limiter that limits requests per API key
// The limit is specified in requests per minute
func RateLimitByAPIKey(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.Limit(
		requestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			// Use API key ID as the rate limit key
			key := GetAPIKey(r.Context())
			if key == nil {
				// If no API key in context, fall back to IP (shouldn't happen after RequireAPIKey)
				return httprate.KeyByIP(r)
			}
			return fmt.Sprintf("apikey:%s", key.ID.String()), nil
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			// Log rate limit event
			key := GetAPIKey(r.Context())
			if key != nil {
				log.Warn().
					Str("api_key_id", key.ID.String()).
					Str("api_key_name", key.Name).
					Str("path", r.URL.Path).
					Msg("Rate limit exceeded")
			}

			// Set Retry-After header (60 seconds = 1 minute)
			w.Header().Set("Retry-After", "60")
			apperrors.WriteError(w, r, http.StatusTooManyRequests, "rate_limit_exceeded", "Rate limit exceeded. Please retry after 60 seconds.")
		}),
	)
}
