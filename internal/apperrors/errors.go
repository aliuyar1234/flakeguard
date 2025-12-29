package apperrors

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents the error envelope per SSOT Section 8.1
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error information
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

// SuccessResponse represents the success envelope per SSOT Section 8.1
type SuccessResponse struct {
	RequestID string      `json:"request_id"`
	Data      interface{} `json:"data"`
}

// WriteError writes an error response in the standard envelope format
func WriteError(w http.ResponseWriter, r *http.Request, statusCode int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := ErrorResponse{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			RequestID: GetRequestID(r.Context()),
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

// WriteSuccess writes a success response in the standard envelope format
func WriteSuccess(w http.ResponseWriter, r *http.Request, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := SuccessResponse{
		RequestID: GetRequestID(r.Context()),
		Data:      data,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}

// WriteServiceUnavailable is a helper for 503 responses
func WriteServiceUnavailable(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusServiceUnavailable, "service_unavailable", message)
}

// WriteInternalError is a helper for 500 responses
func WriteInternalError(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusInternalServerError, "internal_error", message)
}

// WriteBadRequest is a helper for 400 responses
func WriteBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusBadRequest, "bad_request", message)
}

// WriteUnauthorized is a helper for 401 responses
func WriteUnauthorized(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusUnauthorized, "unauthorized", message)
}

// WriteForbidden is a helper for 403 responses
func WriteForbidden(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusForbidden, "forbidden", message)
}

// WriteNotFound is a helper for 404 responses
func WriteNotFound(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusNotFound, "not_found", message)
}

// WriteConflict is a helper for 409 responses
func WriteConflict(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusConflict, "conflict", message)
}

// WriteTooManyRequests is a helper for 429 responses
func WriteTooManyRequests(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusTooManyRequests, "too_many_requests", message)
}

// WritePayloadTooLarge is a helper for 413 responses
func WritePayloadTooLarge(w http.ResponseWriter, r *http.Request, message string) {
	WriteError(w, r, http.StatusRequestEntityTooLarge, "payload_too_large", message)
}
