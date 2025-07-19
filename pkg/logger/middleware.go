package logger

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/screwyprof/delegator/pkg/httpkit"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	bytesOut   int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.bytesOut += size
	return size, err
}

// NewMiddleware creates HTTP request logging middleware
func NewMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Ensure error tracking context exists (in case httpkit.HandlerFunc wasn't used)
			ctx := httpkit.WithErrorTracking(r.Context())
			r = r.WithContext(ctx)

			// Get request size - use max() to handle -1 case (unknown length)
			bytesIn := max(0, int(r.ContentLength))

			// Wrap the response writer to capture status code and response size
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default to 200 if WriteHeader is never called
			}

			// Serve the request
			next.ServeHTTP(rw, r)

			// Calculate duration
			duration := time.Since(start)

			// Determine log level based on status code
			var level slog.Level
			switch {
			case rw.statusCode >= http.StatusInternalServerError:
				level = slog.LevelError
			default:
				level = slog.LevelInfo
			}

			// Build base log attributes
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("uri", r.RequestURI),
				slog.Int("status", rw.statusCode),
				slog.Duration("duration", duration),
				slog.Int("bytes_in", bytesIn),
				slog.Int("bytes_out", rw.bytesOut),
			}

			// Add error details if available
			if err := httpkit.Error(r.Context()); err != nil {
				// Extract appropriate error message for logging
				attrs = append(attrs, slog.String("error", errorMessage(err)))
			}

			// Log with constant message - let structured fields tell the story
			logger.LogAttrs(r.Context(), level, "HTTP", attrs...)
		})
	}
}

// errorMessage extracts the appropriate error message for logging
func errorMessage(err error) string {
	if httpErr, ok := err.(httpkit.HTTPError); ok {
		return httpErr.Cause().Error() // detailed error for logs
	}
	return err.Error() // fallback for regular errors
}
