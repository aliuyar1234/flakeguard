package auth

import (
	"context"
	"net/http"
	"net/url"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type contextKey string

const UserIDContextKey contextKey = "user_id"

// AuthMiddleware validates the session cookie and injects the user ID into context.
// If the session is invalid, it clears the cookie and continues without authentication.
func AuthMiddleware(secret string, isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := GetSessionCookie(r)
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := ValidateToken(token, secret)
			if err != nil {
				log.Debug().Err(err).Msg("Invalid session token")
				ClearSessionCookie(w, isProduction)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDContextKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuth enforces session authentication for JSON API routes.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r.Context()) == uuid.Nil {
			apperrors.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuthPage enforces session authentication for HTML pages.
func RequireAuthPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserID(r.Context()) == uuid.Nil {
			nextURL := r.URL.RequestURI()
			http.Redirect(w, r, "/login?next="+url.QueryEscape(nextURL), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUserID retrieves the user ID from the request context.
func GetUserID(ctx context.Context) uuid.UUID {
	userID, ok := ctx.Value(UserIDContextKey).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return userID
}
