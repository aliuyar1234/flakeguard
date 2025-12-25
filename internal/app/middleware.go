package app

import (
	"github.com/flakeguard/flakeguard/internal/apperrors"
	"net/http"
	"sync"
	"time"

	"github.com/flakeguard/flakeguard/internal/auth"
	"github.com/rs/zerolog/log"
)

// LoggingMiddleware logs HTTP requests with structured fields
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log request details
		duration := time.Since(start)
		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.statusCode).
			Dur("duration", duration).
			Str("request_id", GetRequestID(r.Context())).
			Str("remote_addr", r.RemoteAddr).
			Msg("HTTP request")
	})
}

// RecoveryMiddleware recovers from panics and returns a 500 error
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("error", err).
					Str("request_id", GetRequestID(r.Context())).
					Str("path", r.URL.Path).
					Msg("Panic recovered")

				apperrors.WriteInternalError(w, r, "Internal server error")
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// statusResponseWriter wraps http.ResponseWriter to capture status code
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code
func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// ContentTypeJSON sets Content-Type to application/json
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// NoCacheMiddleware adds headers to prevent caching
func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

// CSRFMiddleware validates CSRF tokens for POST/PUT/DELETE requests
func CSRFMiddleware(isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate CSRF for state-changing methods
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
				if err := auth.ValidateCSRF(r); err != nil {
					log.Warn().
						Err(err).
						Str("method", r.Method).
						Str("path", r.URL.Path).
						Str("remote_addr", r.RemoteAddr).
						Msg("CSRF validation failed")
					http.Error(w, `{"error":{"code":"forbidden","message":"Invalid CSRF token"}}`, http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimiter tracks request rates per IP address
type rateLimiter struct {
	mu       sync.Mutex
	requests map[string]*ipRateLimit
}

// ipRateLimit tracks requests for a single IP
type ipRateLimit struct {
	count      int
	resetAt    time.Time
	lastAccess time.Time
}

// newRateLimiter creates a new rate limiter
func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		requests: make(map[string]*ipRateLimit),
	}
	// Start cleanup goroutine
	go rl.cleanup()
	return rl
}

// cleanup removes stale IP entries every minute
func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, limit := range rl.requests {
			// Remove entries older than 2 minutes
			if now.Sub(limit.lastAccess) > 2*time.Minute {
				delete(rl.requests, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks if the request from this IP should be allowed
func (rl *rateLimiter) allow(ip string, maxRequests int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	limit, exists := rl.requests[ip]

	if !exists || now.After(limit.resetAt) {
		// First request or window expired - allow and reset
		rl.requests[ip] = &ipRateLimit{
			count:      1,
			resetAt:    now.Add(window),
			lastAccess: now,
		}
		return true
	}

	// Update last access time
	limit.lastAccess = now

	// Check if under limit
	if limit.count < maxRequests {
		limit.count++
		return true
	}

	// Over limit
	return false
}

// RateLimitMiddleware limits requests per IP address
// Default: 10 requests per minute
func RateLimitMiddleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	limiter := newRateLimiter()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr

			if !limiter.allow(ip, maxRequests, window) {
				log.Warn().
					Str("ip", ip).
					Str("path", r.URL.Path).
					Msg("Rate limit exceeded")
				http.Error(w, `{"error":{"code":"too_many_requests","message":"Rate limit exceeded. Please try again later."}}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
