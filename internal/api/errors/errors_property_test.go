package errors

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// **Feature: release-changelog-cicd, Property 8: Structured Error Response Format**
// *For any* API error response, the response body SHALL contain code (string),
// message (string), and request_id (string) fields.
// **Validates: Requirements 11.1**
func TestPropertyStructuredErrorResponseFormat(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for error codes
	genErrorCode := gen.OneConstOf(
		CodeValidationError,
		CodeNotFound,
		CodeUnauthorized,
		CodeForbidden,
		CodeInternalError,
		CodeConflict,
	)

	// Generator for non-empty strings
	genNonEmptyString := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})

	// Generator for request IDs (UUID-like format)
	genRequestID := gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")

	properties.Property("Error response contains required fields", prop.ForAll(
		func(code, message, requestID string) bool {
			// Create an API error
			err := New(code, message).WithRequestID(requestID)

			// Write the error to a response recorder
			rr := httptest.NewRecorder()
			WriteError(rr, err)

			// Parse the response body
			var response map[string]any
			if jsonErr := json.NewDecoder(rr.Body).Decode(&response); jsonErr != nil {
				t.Logf("Failed to decode response: %v", jsonErr)
				return false
			}

			// Check that code field exists and is a string
			codeVal, hasCode := response["code"]
			if !hasCode {
				t.Log("Response missing 'code' field")
				return false
			}
			if _, ok := codeVal.(string); !ok {
				t.Log("'code' field is not a string")
				return false
			}

			// Check that message field exists and is a string
			messageVal, hasMessage := response["message"]
			if !hasMessage {
				t.Log("Response missing 'message' field")
				return false
			}
			if _, ok := messageVal.(string); !ok {
				t.Log("'message' field is not a string")
				return false
			}

			// Check that request_id field exists and is a string
			requestIDVal, hasRequestID := response["request_id"]
			if !hasRequestID {
				t.Log("Response missing 'request_id' field")
				return false
			}
			if _, ok := requestIDVal.(string); !ok {
				t.Log("'request_id' field is not a string")
				return false
			}

			// Verify the values match what we set
			if codeVal.(string) != code {
				t.Logf("Code mismatch: got %s, want %s", codeVal, code)
				return false
			}
			if messageVal.(string) != message {
				t.Logf("Message mismatch: got %s, want %s", messageVal, message)
				return false
			}
			if requestIDVal.(string) != requestID {
				t.Logf("RequestID mismatch: got %s, want %s", requestIDVal, requestID)
				return false
			}

			return true
		},
		genErrorCode,
		genNonEmptyString,
		genRequestID,
	))

	properties.Property("HTTP status code matches error code", prop.ForAll(
		func(code string) bool {
			err := New(code, "test message")

			expectedStatus := err.HTTPStatusCode()
			rr := httptest.NewRecorder()
			WriteError(rr, err)

			if rr.Code != expectedStatus {
				t.Logf("Status code mismatch for %s: got %d, want %d", code, rr.Code, expectedStatus)
				return false
			}

			return true
		},
		genErrorCode,
	))

	properties.Property("Content-Type is application/json", prop.ForAll(
		func(code, message string) bool {
			err := New(code, message)

			rr := httptest.NewRecorder()
			WriteError(rr, err)

			contentType := rr.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Logf("Content-Type mismatch: got %s, want application/json", contentType)
				return false
			}

			return true
		},
		genErrorCode,
		genNonEmptyString,
	))

	properties.TestingRun(t)
}

// **Feature: release-changelog-cicd, Property 9: Validation Error Field Details**
// *For any* validation error, the error response details field SHALL contain an array
// of field-specific errors with field name and error message.
// **Validates: Requirements 11.2**
func TestPropertyValidationErrorFieldDetails(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for field names (valid identifier-like strings)
	genFieldName := gen.RegexMatch("[a-z][a-zA-Z0-9_]{0,20}")

	// Generator for non-empty error messages
	genErrorMessage := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})

	// Generator for validation error
	genValidationError := gopter.CombineGens(
		genFieldName,
		genErrorMessage,
	).Map(func(values []interface{}) ValidationError {
		return ValidationError{
			Field:   values[0].(string),
			Message: values[1].(string),
		}
	})

	properties.Property("Validation error contains field-level details", prop.ForAll(
		func(validationErrors []ValidationError) bool {
			// Skip empty validation errors
			if len(validationErrors) == 0 {
				return true
			}

			// Create validation errors
			var errs ValidationErrors
			for _, ve := range validationErrors {
				errs.Add(ve.Field, ve.Message)
			}

			// Convert to API error
			apiErr := errs.ToAPIError()

			// Write the error to a response recorder
			rr := httptest.NewRecorder()
			WriteError(rr, apiErr)

			// Parse the response body
			var response map[string]any
			if jsonErr := json.NewDecoder(rr.Body).Decode(&response); jsonErr != nil {
				t.Logf("Failed to decode response: %v", jsonErr)
				return false
			}

			// Check that code is VALIDATION_ERROR
			if response["code"] != CodeValidationError {
				t.Logf("Expected code %s, got %v", CodeValidationError, response["code"])
				return false
			}

			// Check that details field exists
			details, hasDetails := response["details"]
			if !hasDetails {
				t.Log("Response missing 'details' field")
				return false
			}

			detailsMap, ok := details.(map[string]any)
			if !ok {
				t.Log("'details' field is not an object")
				return false
			}

			// Check that fields array exists in details
			fields, hasFields := detailsMap["fields"]
			if !hasFields {
				t.Log("Details missing 'fields' array")
				return false
			}

			fieldsArray, ok := fields.([]any)
			if !ok {
				t.Log("'fields' is not an array")
				return false
			}

			// Check that each field error has field and message
			if len(fieldsArray) != len(validationErrors) {
				t.Logf("Field count mismatch: got %d, want %d", len(fieldsArray), len(validationErrors))
				return false
			}

			for i, fieldErr := range fieldsArray {
				fieldErrMap, ok := fieldErr.(map[string]any)
				if !ok {
					t.Logf("Field error %d is not an object", i)
					return false
				}

				// Check field name exists and is a string
				fieldName, hasField := fieldErrMap["field"]
				if !hasField {
					t.Logf("Field error %d missing 'field'", i)
					return false
				}
				if _, ok := fieldName.(string); !ok {
					t.Logf("Field error %d 'field' is not a string", i)
					return false
				}

				// Check message exists and is a string
				message, hasMessage := fieldErrMap["message"]
				if !hasMessage {
					t.Logf("Field error %d missing 'message'", i)
					return false
				}
				if _, ok := message.(string); !ok {
					t.Logf("Field error %d 'message' is not a string", i)
					return false
				}

				// Verify values match
				if fieldName.(string) != validationErrors[i].Field {
					t.Logf("Field name mismatch at %d: got %s, want %s", i, fieldName, validationErrors[i].Field)
					return false
				}
				if message.(string) != validationErrors[i].Message {
					t.Logf("Message mismatch at %d: got %s, want %s", i, message, validationErrors[i].Message)
					return false
				}
			}

			return true
		},
		gen.SliceOfN(5, genValidationError),
	))

	properties.Property("Single field validation error has correct structure", prop.ForAll(
		func(field, message string) bool {
			errs := AddFieldError(field, message)
			apiErr := errs.ToAPIError()

			// Write the error to a response recorder
			rr := httptest.NewRecorder()
			WriteError(rr, apiErr)

			// Parse the response body
			var response map[string]any
			if jsonErr := json.NewDecoder(rr.Body).Decode(&response); jsonErr != nil {
				t.Logf("Failed to decode response: %v", jsonErr)
				return false
			}

			// Verify the message matches the single field error message
			if response["message"] != message {
				t.Logf("Message mismatch: got %v, want %s", response["message"], message)
				return false
			}

			return true
		},
		genFieldName,
		genErrorMessage,
	))

	properties.TestingRun(t)
}

// **Feature: release-changelog-cicd, Property 10: Error Log Completeness**
// *For any* logged error, the log entry SHALL contain correlation_id, error_code,
// message, and stack_trace fields.
// **Validates: Requirements 11.4**
func TestPropertyErrorLogCompleteness(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(time.Now().UnixNano())

	properties := gopter.NewProperties(parameters)

	// Generator for correlation IDs (UUID-like format)
	genCorrelationID := gen.RegexMatch("[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}")

	// Generator for error codes
	genErrorCode := gen.OneConstOf(
		CodeValidationError,
		CodeNotFound,
		CodeUnauthorized,
		CodeForbidden,
		CodeInternalError,
		CodeConflict,
	)

	// Generator for non-empty messages
	genMessage := gen.AlphaString().SuchThat(func(s string) bool {
		return len(s) > 0
	})

	properties.Property("Error log entry contains all required fields", prop.ForAll(
		func(correlationID, errorCode, message string) bool {
			// Create an error log entry
			entry := NewErrorLogEntry(correlationID, errorCode, message)

			// Check correlation_id
			if entry.CorrelationID != correlationID {
				t.Logf("CorrelationID mismatch: got %s, want %s", entry.CorrelationID, correlationID)
				return false
			}

			// Check error_code
			if entry.ErrorCode != errorCode {
				t.Logf("ErrorCode mismatch: got %s, want %s", entry.ErrorCode, errorCode)
				return false
			}

			// Check message
			if entry.Message != message {
				t.Logf("Message mismatch: got %s, want %s", entry.Message, message)
				return false
			}

			// Check stack_trace is non-empty
			if entry.StackTrace == "" {
				t.Log("StackTrace is empty")
				return false
			}

			return true
		},
		genCorrelationID,
		genErrorCode,
		genMessage,
	))

	properties.Property("Error log entry from APIError contains all required fields", prop.ForAll(
		func(correlationID, errorCode, message string) bool {
			// Create an API error
			apiErr := New(errorCode, message)

			// Create an error log entry from the API error
			entry := NewErrorLogEntryFromAPIError(apiErr, correlationID)

			// Check correlation_id
			if entry.CorrelationID != correlationID {
				t.Logf("CorrelationID mismatch: got %s, want %s", entry.CorrelationID, correlationID)
				return false
			}

			// Check error_code matches API error code
			if entry.ErrorCode != apiErr.Code {
				t.Logf("ErrorCode mismatch: got %s, want %s", entry.ErrorCode, apiErr.Code)
				return false
			}

			// Check message matches API error message
			if entry.Message != apiErr.Message {
				t.Logf("Message mismatch: got %s, want %s", entry.Message, apiErr.Message)
				return false
			}

			// Check stack_trace is non-empty
			if entry.StackTrace == "" {
				t.Log("StackTrace is empty")
				return false
			}

			return true
		},
		genCorrelationID,
		genErrorCode,
		genMessage,
	))

	properties.Property("ToSlogAttrs returns all required fields", prop.ForAll(
		func(correlationID, errorCode, message string) bool {
			entry := NewErrorLogEntry(correlationID, errorCode, message)
			attrs := entry.ToSlogAttrs()

			// Should have 8 elements (4 key-value pairs)
			if len(attrs) != 8 {
				t.Logf("Expected 8 attrs, got %d", len(attrs))
				return false
			}

			// Check that all required keys are present
			requiredKeys := map[string]bool{
				"correlation_id": false,
				"error_code":     false,
				"message":        false,
				"stack_trace":    false,
			}

			for i := 0; i < len(attrs); i += 2 {
				key, ok := attrs[i].(string)
				if !ok {
					t.Logf("Key at index %d is not a string", i)
					return false
				}
				if _, exists := requiredKeys[key]; exists {
					requiredKeys[key] = true
				}
			}

			for key, found := range requiredKeys {
				if !found {
					t.Logf("Missing required key: %s", key)
					return false
				}
			}

			return true
		},
		genCorrelationID,
		genErrorCode,
		genMessage,
	))

	properties.TestingRun(t)
}
