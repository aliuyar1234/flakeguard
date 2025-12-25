package auth

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDContextKey is the context key for storing user ID
	UserIDContextKey contextKey = "user_id"
)

// AuthMiddleware validates the session cookie and injects the user ID into context
// If the session is invalid, it clears the cookie and continues without authentication
func AuthMiddleware(secret string, sessionDays int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session cookie
			token := GetSessionCookie(r)
			if token == "" {
				// No session cookie - continue without authentication
				next.ServeHTTP(w, r)
				return
			}

			// Validate token
			claims, err := ValidateToken(token, secret)
			if err != nil {
				// Invalid token - clear cookie and continue
				log.Debug().Err(err).Msg("Invalid session token")
				ClearSessionCookie(w)
				next.ServeHTTP(w, r)
				return
			}

			// Add user ID to context
			ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth is middleware that requires authentication
// Returns 401 if the user is not authenticated
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID retrieves the user ID from the request context
// Returns uuid.Nil if no user is authenticated
func GetUserID(ctx context.Context) uuid.UUID {
	userID, ok := ctx.Value(UserIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return userID
}
