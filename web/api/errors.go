package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for error classification.
var (
	ErrBadRequest          = errors.New(http.StatusText(http.StatusBadRequest))
	ErrInternalServerError = errors.New(http.StatusText(http.StatusInternalServerError))
)

// Error represents a structured API error response.
type Error struct {
	cause    error  // The original error (for logging/debugging)
	message  string // Safe user-facing message
	httpCode int    // HTTP status code (also used as API error code)
}

// HTTPCode returns the HTTP status code for this error.
func (e *Error) HTTPCode() int {
	return e.httpCode
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.message
}

// Unwrap returns the underlying cause for error unwrapping.
func (e *Error) Unwrap() error {
	return e.cause
}

// Is implements error checking for sentinel errors.
func (e *Error) Is(target error) bool {
	return errors.Is(e.cause, target)
}

// Cause returns the original error for logging purposes.
func (e *Error) Cause() error {
	return e.cause
}

// MarshalJSON implements [json.Marshaler] interface.
func (e *Error) MarshalJSON() ([]byte, error) {
	data, err := json.Marshal(map[string]any{
		"code":    e.httpCode,
		"message": e.message,
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling API error: %w", err)
	}

	return data, nil
}

// BadRequest is a 400 whose message is cause's, since client errors are safe to show.
func BadRequest(cause error) *Error {
	return &Error{
		cause:    cause,
		message:  cause.Error(), // 4xx errors are safe to expose
		httpCode: http.StatusBadRequest,
	}
}

// InternalServerError is a 500 with a generic message; cause is kept for logs, never sent.
func InternalServerError(cause error) *Error {
	return &Error{
		cause:    cause,
		message:  http.StatusText(http.StatusInternalServerError), // Never expose internal error details
		httpCode: http.StatusInternalServerError,
	}
}

// ServiceUnavailable is a 503 for a dependency that doesn't answer; like a 500, the cause is kept
// for logs and never sent.
func ServiceUnavailable(cause error) *Error {
	return &Error{
		cause:    cause,
		message:  http.StatusText(http.StatusServiceUnavailable),
		httpCode: http.StatusServiceUnavailable,
	}
}

// Wrap transforms any error into a safe API error
// If the error is already an API error, it returns it unchanged.
func Wrap(err error) *Error {
	if err == nil {
		return nil
	}

	// Don't double-wrap API errors
	if apiErr, ok := errors.AsType[*Error](err); ok {
		return apiErr
	}

	// For now, classify all unknown errors as internal server errors
	// In the future, this could be expanded to check for specific error types
	return InternalServerError(err)
}
