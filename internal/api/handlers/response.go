package handlers

import (
	"encoding/json"
	"net/http"
)

// APIError represents a standard API error response.
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
