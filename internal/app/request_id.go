package app

import (
	"context"
	"net/http"

	"github.com/aliuyar1234/flakeguard/internal/apperrors"
)

// RequestIDMiddleware adds a unique request ID to each request.
// Wrapper around internal/apperrors to satisfy SSOT repository structure.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return apperrors.RequestIDMiddleware(next)
}

// GetRequestID retrieves the request ID from the context.
func GetRequestID(ctx context.Context) string {
	return apperrors.GetRequestID(ctx)
}
