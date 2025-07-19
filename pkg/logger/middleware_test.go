package logger_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/httpkit"
	"github.com/screwyprof/delegator/pkg/logger"
)

// Error is a test error type that implements httpkit.HTTPError
type Error struct {
	err  error
	code int
}

func (e Error) Error() string { return e.err.Error() }
func (e Error) HTTPCode() int { return e.code }
func (e Error) Cause() error  { return e.err }

// logEntry represents a parsed log entry for testing
type logEntry struct {
	Level    string  `json:"level"`
	Msg      string  `json:"msg"`
	Method   string  `json:"method"`
	URI      string  `json:"uri"`
	Status   int     `json:"status"`
	Duration float64 `json:"duration"` // slog logs duration as nanoseconds (number)
	BytesIn  int     `json:"bytes_in"`
	BytesOut int     `json:"bytes_out"`
	Error    string  `json:"error,omitempty"`
}

// parseLogEntry parses a single JSON log line
func parseLogEntry(t *testing.T, logOutput string) logEntry {
	t.Helper()

	// Get the last line (most recent log entry)
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	lastLine := lines[len(lines)-1]

	var entry logEntry
	err := json.Unmarshal([]byte(lastLine), &entry)
	require.NoError(t, err, "Should parse log entry as JSON")

	return entry
}

func TestNewMiddleware(t *testing.T) {
	t.Parallel()

	t.Run("it logs successful requests at info level", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var logBuffer bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))

		successHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "ok"}`))
		})

		middleware := logger.NewMiddleware(log)(successHandler)
		req := httptest.NewRequest(http.MethodGet, "/test/success", nil)
		rec := httptest.NewRecorder()

		// Act
		middleware.ServeHTTP(rec, req)

		// Assert
		assert.Equal(t, http.StatusOK, rec.Code)

		entry := parseLogEntry(t, logBuffer.String())
		assert.Equal(t, "INFO", entry.Level)
		assert.Equal(t, "HTTP", entry.Msg)
		assert.Equal(t, http.MethodGet, entry.Method)
		assert.Equal(t, "/test/success", entry.URI)
		assert.Equal(t, http.StatusOK, entry.Status)
		assert.Greater(t, entry.Duration, 0.0)
		assert.Equal(t, req.ContentLength, int64(entry.BytesIn))
		assert.Equal(t, rec.Body.Len(), entry.BytesOut)
		assert.Empty(t, entry.Error)
	})

	t.Run("it logs client errors at info level with error details", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var logBuffer bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))

		badRequestHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			validationErr := errors.New("invalid year parameter: year must be exactly 4 digits")
			apiErr := Error{err: validationErr, code: http.StatusBadRequest}
			httpkit.JsonError(apiErr)(w, r)
		})

		middleware := logger.NewMiddleware(log)(badRequestHandler)
		req := httptest.NewRequest(http.MethodGet, "/test/invalid", nil)
		rec := httptest.NewRecorder()

		// Act
		middleware.ServeHTTP(rec, req)

		// Assert
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		entry := parseLogEntry(t, logBuffer.String())
		assert.Equal(t, "INFO", entry.Level)
		assert.Equal(t, "HTTP", entry.Msg)
		assert.Equal(t, http.MethodGet, entry.Method)
		assert.Equal(t, "/test/invalid", entry.URI)
		assert.Equal(t, 400, entry.Status)
		assert.Equal(t, "invalid year parameter: year must be exactly 4 digits", entry.Error)
	})

	t.Run("it logs server errors at error level with error details", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var logBuffer bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))

		serverErrorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			internalErr := errors.New("database connection failed: connection refused")
			apiErr := Error{err: internalErr, code: http.StatusInternalServerError}
			httpkit.JsonError(apiErr)(w, r)
		})

		middleware := logger.NewMiddleware(log)(serverErrorHandler)
		req := httptest.NewRequest(http.MethodGet, "/test/error", nil)
		rec := httptest.NewRecorder()

		// Act
		middleware.ServeHTTP(rec, req)

		// Assert
		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		entry := parseLogEntry(t, logBuffer.String())
		assert.Equal(t, "ERROR", entry.Level)
		assert.Equal(t, "HTTP", entry.Msg)
		assert.Equal(t, http.MethodGet, entry.Method)
		assert.Equal(t, "/test/error", entry.URI)
		assert.Equal(t, http.StatusInternalServerError, entry.Status)
		assert.Equal(t, "database connection failed: connection refused", entry.Error)
	})

	t.Run("it captures request duration accurately", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var logBuffer bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))

		slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond) // Small delay
			w.WriteHeader(http.StatusOK)
		})

		middleware := logger.NewMiddleware(log)(slowHandler)
		req := httptest.NewRequest(http.MethodGet, "/test/slow", nil)
		rec := httptest.NewRecorder()

		// Act
		start := time.Now()
		middleware.ServeHTTP(rec, req)
		actualDuration := time.Since(start)

		// Assert
		entry := parseLogEntry(t, logBuffer.String())

		// Duration should be at least 10ms (our artificial delay)
		assert.GreaterOrEqual(t, entry.Duration, 10_000_000.0) // 10ms in nanoseconds

		// Duration should be reasonably close to actual duration (within 50ms tolerance)
		toleranceNs := 50_000_000.0 // 50ms in nanoseconds
		assert.InDelta(t, float64(actualDuration.Nanoseconds()), entry.Duration, toleranceNs)
	})

	t.Run("it tracks request and response bytes", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var logBuffer bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{Level: slog.LevelInfo}))

		echoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Hello, World!"))
		})

		middleware := logger.NewMiddleware(log)(echoHandler)

		// Create request with body
		reqBody := `{"test": "data"}`
		req := httptest.NewRequest(http.MethodPost, "/test/echo", strings.NewReader(reqBody))
		req.Header.Set("Content-Length", strconv.Itoa(len(reqBody)))
		rec := httptest.NewRecorder()

		// Act
		middleware.ServeHTTP(rec, req)

		// Assert
		entry := parseLogEntry(t, logBuffer.String())
		assert.Equal(t, len(reqBody), entry.BytesIn)
		assert.Equal(t, rec.Body.Len(), entry.BytesOut)
	})
}
