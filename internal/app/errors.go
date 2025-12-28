package app

import (
	"net/http"

	"github.com/flakeguard/flakeguard/internal/apperrors"
)

// ErrorResponse represents the error envelope per SSOT Section 8.1.
type ErrorResponse = apperrors.ErrorResponse

// SuccessResponse represents the success envelope per SSOT Section 8.1.
type SuccessResponse = apperrors.SuccessResponse

func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	apperrors.WriteError(w, r, statusCode, code, message)
}

func WriteSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	apperrors.WriteSuccess(w, r, statusCode, data)
}

func WriteServiceUnavailable(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteServiceUnavailable(w, r, message)
}

func WriteInternalError(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteInternalError(w, r, message)
}

func WriteBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteBadRequest(w, r, message)
}

func WriteUnauthorized(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteUnauthorized(w, r, message)
}

func WriteForbidden(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteForbidden(w, r, message)
}

func WriteNotFound(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteNotFound(w, r, message)
}

func WriteConflict(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteConflict(w, r, message)
}

func WriteTooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WriteTooManyRequests(w, r, message)
}

func WritePayloadTooLarge(w http.ResponseWriter, r *http.Request, message string) {
	apperrors.WritePayloadTooLarge(w, r, message)
}
