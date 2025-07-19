package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Sentinel errors for error classification
var (
	ErrBadRequest          = errors.New(http.StatusText(http.StatusBadRequest))
	ErrInternalServerError = errors.New(http.StatusText(http.StatusInternalServerError))
)

// Error represents a structured API error response
type Error struct {
	cause    error  // The original error (for logging/debugging)
	message  string // Safe user-facing message
	httpCode int    // HTTP status code (also used as API error code)
}

// HTTPCode returns the HTTP status code for this error
func (e *Error) HTTPCode() int {
	return e.httpCode
}

// Error implements the error interface
func (e *Error) Error() string {
	return e.message
}

// Unwrap returns the underlying cause for error unwrapping
func (e *Error) Unwrap() error {
	return e.cause
}

// Is implements error checking for sentinel errors
func (e *Error) Is(target error) bool {
	return errors.Is(e.cause, target)
}

// Cause returns the original error for logging purposes
func (e *Error) Cause() error {
	return e.cause
}

// MarshalJSON implements json.Marshaler interface
func (e *Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"code":    e.httpCode,
		"message": e.message,
	})
}

// Constructor functions for different error types

func BadRequest(cause error) *Error {
	return &Error{
		cause:    cause,
		message:  cause.Error(), // 4xx errors are safe to expose
		httpCode: http.StatusBadRequest,
	}
}

func InternalServerError(cause error) *Error {
	return &Error{
		cause:    cause,
		message:  http.StatusText(http.StatusInternalServerError), // Never expose internal error details
		httpCode: http.StatusInternalServerError,
	}
}

// Wrap transforms any error into a safe API error
// If the error is already an API error, it returns it unchanged
func Wrap(err error) *Error {
	if err == nil {
		return nil
	}

	// Don't double-wrap API errors
	var apiErr *Error
	if errors.As(err, &apiErr) {
		return apiErr
	}

	// For now, classify all unknown errors as internal server errors
	// In the future, this could be expanded to check for specific error types
	return InternalServerError(err)
}
