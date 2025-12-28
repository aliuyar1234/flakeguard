package app

import (
	"net/http"
	"time"

	"github.com/flakeguard/flakeguard/internal/apperrors"
	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/go-chi/httprate"
	"github.com/rs/zerolog/log"
)

// LoggingMiddleware logs HTTP requests with structured fields.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration", time.Since(start)).
			Str("request_id", apperrors.GetRequestID(r.Context())).
			Str("remote_addr", r.RemoteAddr).
			Msg("HTTP request")
	})
}

// RecoveryMiddleware recovers from panics and returns a 500 error.
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("error", err).
					Str("request_id", apperrors.GetRequestID(r.Context())).
					Str("path", r.URL.Path).
					Msg("Panic recovered")

				apperrors.WriteInternalError(w, r, "Internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// ContentTypeJSON sets Content-Type to application/json.
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// NoCacheMiddleware adds headers to prevent caching.
func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware validates CSRF tokens for POST/PUT/DELETE requests.
func CSRFMiddleware(isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
				if err := auth.ValidateCSRF(r); err != nil {
					log.Warn().
						Err(err).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("remote_addr", r.RemoteAddr).
						Msg("CSRF validation failed")

					apperrors.WriteForbidden(w, r, "Invalid CSRF token")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LoginRateLimitMiddleware limits login attempts per IP address to 10/minute.
func LoginRateLimitMiddleware() func(http.Handler) http.Handler {
	return httprate.Limit(
		10,
		time.Minute,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "60")
			apperrors.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "Too many login attempts. Try again later.")
		}),
	)
}
