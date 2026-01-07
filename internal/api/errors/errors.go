// Package errors provides structured error types and response helpers for the API.
package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
)

// Error codes for structured API responses.
const (
	CodeValidationError = "VALIDATION_ERROR"
	CodeNotFound        = "NOT_FOUND"
	CodeUnauthorized    = "UNAUTHORIZED"
	CodeForbidden       = "FORBIDDEN"
	CodeInternalError   = "INTERNAL_ERROR"
	CodeConflict        = "CONFLICT"
)

// APIError represents a structured API error response.
type APIError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// WithDetails returns a copy of the error with additional details.
func (e *APIError) WithDetails(details map[string]any) *APIError {
	return &APIError{
		Code:      e.Code,
		Message:   e.Message,
		Details:   details,
		RequestID: e.RequestID,
	}
}

// WithRequestID returns a copy of the error with the request ID set.
func (e *APIError) WithRequestID(requestID string) *APIError {
	return &APIError{
		Code:      e.Code,
		Message:   e.Message,
		Details:   e.Details,
		RequestID: requestID,
	}
}

// New creates a new APIError with the given code and message.
func New(code, message string) *APIError {
	return &APIError{
		Code:    code,
		Message: message,
	}
}

// NewValidationError creates a validation error.
func NewValidationError(message string) *APIError {
	return New(CodeValidationError, message)
}

// NewNotFoundError creates a not found error.
func NewNotFoundError(message string) *APIError {
	return New(CodeNotFound, message)
}

// NewUnauthorizedError creates an unauthorized error.
func NewUnauthorizedError(message string) *APIError {
	return New(CodeUnauthorized, message)
}

// NewForbiddenError creates a forbidden error.
func NewForbiddenError(message string) *APIError {
	return New(CodeForbidden, message)
}

// NewInternalError creates an internal server error.
func NewInternalError(message string) *APIError {
	return New(CodeInternalError, message)
}

// NewConflictError creates a conflict error.
func NewConflictError(message string) *APIError {
	return New(CodeConflict, message)
}

// HTTPStatusCode returns the appropriate HTTP status code for the error.
func (e *APIError) HTTPStatusCode() int {
	switch e.Code {
	case CodeValidationError:
		return http.StatusBadRequest
	case CodeNotFound:
		return http.StatusNotFound
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeConflict:
		return http.StatusConflict
	case CodeInternalError:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// WriteError writes an APIError as a JSON response.
func WriteError(w http.ResponseWriter, err *APIError) {
	WriteJSON(w, err.HTTPStatusCode(), err)
}

// WriteErrorWithRequestID writes an APIError with the request ID set.
func WriteErrorWithRequestID(w http.ResponseWriter, err *APIError, requestID string) {
	WriteError(w, err.WithRequestID(requestID))
}

// GetStackTrace returns the current stack trace as a string.
func GetStackTrace() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

// ValidationError represents a field-level validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of field-level validation errors.
type ValidationErrors []ValidationError

// Add adds a new validation error for a field.
func (v *ValidationErrors) Add(field, message string) {
	*v = append(*v, ValidationError{Field: field, Message: message})
}

// HasErrors returns true if there are any validation errors.
func (v ValidationErrors) HasErrors() bool {
	return len(v) > 0
}

// ToAPIError converts validation errors to an APIError with field details.
func (v ValidationErrors) ToAPIError() *APIError {
	if len(v) == 0 {
		return NewValidationError("validation failed")
	}

	// Use the first error's message as the main message
	mainMessage := v[0].Message
	if len(v) > 1 {
		mainMessage = fmt.Sprintf("%s (and %d more errors)", mainMessage, len(v)-1)
	}

	return &APIError{
		Code:    CodeValidationError,
		Message: mainMessage,
		Details: map[string]any{
			"fields": v,
		},
	}
}

// NewValidationErrorWithFields creates a validation error with field-level details.
func NewValidationErrorWithFields(fields ValidationErrors) *APIError {
	return fields.ToAPIError()
}

// AddFieldError is a helper to create a validation error for a single field.
func AddFieldError(field, message string) ValidationErrors {
	return ValidationErrors{{Field: field, Message: message}}
}

// ErrorLogEntry represents a structured error log entry.
type ErrorLogEntry struct {
	CorrelationID string `json:"correlation_id"`
	ErrorCode     string `json:"error_code"`
	Message       string `json:"message"`
	StackTrace    string `json:"stack_trace"`
}

// NewErrorLogEntry creates a new error log entry with all required fields.
func NewErrorLogEntry(correlationID, errorCode, message string) *ErrorLogEntry {
	return &ErrorLogEntry{
		CorrelationID: correlationID,
		ErrorCode:     errorCode,
		Message:       message,
		StackTrace:    GetStackTrace(),
	}
}

// NewErrorLogEntryFromAPIError creates an error log entry from an APIError.
func NewErrorLogEntryFromAPIError(err *APIError, correlationID string) *ErrorLogEntry {
	return &ErrorLogEntry{
		CorrelationID: correlationID,
		ErrorCode:     err.Code,
		Message:       err.Message,
		StackTrace:    GetStackTrace(),
	}
}

// ToSlogAttrs returns the error log entry as slog attributes for structured logging.
func (e *ErrorLogEntry) ToSlogAttrs() []any {
	return []any{
		"correlation_id", e.CorrelationID,
		"error_code", e.ErrorCode,
		"message", e.Message,
		"stack_trace", e.StackTrace,
	}
}
