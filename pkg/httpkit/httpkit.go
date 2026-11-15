// Package httpkit is a small toolkit for handlers that return an [http.HandlerFunc]: JSON
// responses, HTTP-aware errors, and request-scoped error tracking that middleware can read.
package httpkit

import (
	"context"
	"encoding/json"
	"net/http"
)

// HTTPError interface for HTTP-aware errors with detailed causes.
type HTTPError interface {
	HTTPCode() int
	Cause() error
	error
}

// Header constants.
const (
	contentTypeHeader  = "Content-Type"
	contentTypeOptions = "X-Content-Type-Options"

	jsonContentType           = "application/json; charset=utf-8"
	nosniffContentTypeOptions = "nosniff"
)

// addHeaderIfNotSet sets key to value unless the handler already set it. Each call gets its own
// slice, so no two responses share a header value's backing array.
func addHeaderIfNotSet(w http.ResponseWriter, key, value string) {
	header := w.Header()
	if val := header[key]; len(val) == 0 {
		header[key] = []string{value}
	}
}

// Context helpers for request-scoped error tracking.
type ctxKeyError struct{}

type errorHolder struct {
	err error
}

// WithErrorTracking creates context with error tracking capability, or returns existing context if already present.
func WithErrorTracking(ctx context.Context) context.Context {
	if _, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		return ctx // Already has error tracking
	}

	holder := &errorHolder{}

	return context.WithValue(ctx, ctxKeyError{}, holder)
}

// SetError sets error in the context.
func SetError(ctx context.Context, err error) {
	if holder, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		holder.err = err
	}
}

// Error gets error from context.
func Error(ctx context.Context) error {
	if holder, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		return holder.err
	}

	return nil
}

// HandlerFunc is a handler that returns the response to write as an [http.HandlerFunc], so a
// handler can end with `return httpkit.JSON(v)` or `return httpkit.JSONError(err)`.
type HandlerFunc func(http.ResponseWriter, *http.Request) http.HandlerFunc

// ServeHTTP gives the request a context that can record an error for middleware, runs h, and
// writes the handler it returns, if any.
func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := WithErrorTracking(r.Context())
	r = r.WithContext(ctx)

	if handler := h(w, r); handler != nil {
		handler(w, r)
	}
}

// JSON creates a handler that returns JSON response.
func JSON(data any) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		addHeaderIfNotSet(w, contentTypeHeader, jsonContentType)
		addHeaderIfNotSet(w, contentTypeOptions, nosniffContentTypeOptions)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(data)
	}
}

// JSONError creates a handler that sets an error in context and writes the error response.
func JSONError(err HTTPError) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Set error in context for middleware (if available)
		SetError(r.Context(), err)

		// Add headers
		addHeaderIfNotSet(w, contentTypeHeader, jsonContentType)
		addHeaderIfNotSet(w, contentTypeOptions, nosniffContentTypeOptions)

		// Write the status code and response
		w.WriteHeader(err.HTTPCode())
		_ = json.NewEncoder(w).Encode(err)
	}
}
