package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5/middleware"
	apierrors "github.com/narvanalabs/control-plane/internal/api/errors"
)

// Recovery returns a middleware that recovers from panics and logs the error.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					requestID := middleware.GetReqID(r.Context())

					// Create structured error log entry
					logEntry := apierrors.NewErrorLogEntry(
						requestID,
						apierrors.CodeInternalError,
						"panic recovered",
					)

					logger.Error("panic recovered",
						"error", rec,
						"correlation_id", logEntry.CorrelationID,
						"error_code", logEntry.ErrorCode,
						"stack_trace", string(debug.Stack()),
						"request_id", requestID,
						"method", r.Method,
						"path", r.URL.Path,
					)

					// Write structured error response
					err := apierrors.NewInternalError("An unexpected error occurred").WithRequestID(requestID)
					apierrors.WriteError(w, err)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
