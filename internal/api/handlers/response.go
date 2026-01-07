package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	apierrors "github.com/narvanalabs/control-plane/internal/api/errors"
)

// APIError represents a standard API error response.
// Deprecated: Use apierrors.APIError from internal/api/errors package instead.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return e.Message
}

// Common error codes.
// Deprecated: Use constants from internal/api/errors package instead.
const (
	ErrCodeInvalidRequest = "invalid_request"
	ErrCodeNotFound       = "not_found"
	ErrCodeConflict       = "conflict"
	ErrCodeUnauthorized   = "unauthorized"
	ErrCodeForbidden      = "forbidden"
	ErrCodeInternalError  = "internal_error"
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// WriteError writes an error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, &APIError{
		Code:    code,
		Message: message,
	})
}

// WriteErrorWithDetails writes an error response with additional details.
func WriteErrorWithDetails(w http.ResponseWriter, status int, code, message string, details any) {
	WriteJSON(w, status, &APIError{
		Code:    code,
		Message: message,
		Details: details,
	})
}

// WriteBadRequest writes a 400 Bad Request response.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeInvalidRequest, message)
}

// WriteNotFound writes a 404 Not Found response.
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteConflict writes a 409 Conflict response.
func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, ErrCodeConflict, message)
}

// WriteUnauthorized writes a 401 Unauthorized response.
func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// WriteForbidden writes a 403 Forbidden response.
func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// WriteInternalError writes a 500 Internal Server Error response.
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, message)
}

// WriteStructuredError writes a structured API error response with request ID.
// This is the preferred method for writing error responses as it includes
// the request ID for correlation.
func WriteStructuredError(w http.ResponseWriter, r *http.Request, err *apierrors.APIError) {
	requestID := middleware.GetReqID(r.Context())
	apierrors.WriteErrorWithRequestID(w, err, requestID)
}

// WriteStructuredValidationError writes a validation error with field-level details.
func WriteStructuredValidationError(w http.ResponseWriter, r *http.Request, fields apierrors.ValidationErrors) {
	requestID := middleware.GetReqID(r.Context())
	err := fields.ToAPIError().WithRequestID(requestID)
	apierrors.WriteError(w, err)
}

// WriteStructuredBadRequest writes a structured 400 Bad Request response.
func WriteStructuredBadRequest(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewValidationError(message))
}

// WriteStructuredNotFound writes a structured 404 Not Found response.
func WriteStructuredNotFound(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewNotFoundError(message))
}

// WriteStructuredUnauthorized writes a structured 401 Unauthorized response.
func WriteStructuredUnauthorized(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewUnauthorizedError(message))
}

// WriteStructuredForbidden writes a structured 403 Forbidden response.
func WriteStructuredForbidden(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewForbiddenError(message))
}

// WriteStructuredConflict writes a structured 409 Conflict response.
func WriteStructuredConflict(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewConflictError(message))
}

// WriteStructuredInternalError writes a structured 500 Internal Server Error response.
func WriteStructuredInternalError(w http.ResponseWriter, r *http.Request, message string) {
	WriteStructuredError(w, r, apierrors.NewInternalError(message))
}
