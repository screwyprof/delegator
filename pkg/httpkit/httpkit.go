package httpkit

import (
	"context"
	"encoding/json"
	"net/http"
)

// HTTPError interface for HTTP-aware errors with detailed causes
type HTTPError interface {
	HTTPCode() int
	Cause() error
	error
}

// Header constants
const (
	contentTypeHeader  = "Content-Type"
	contentTypeOptions = "X-Content-Type-Options"
)

var (
	jsonContentType           = []string{"application/json; charset=utf-8"}
	nosniffContentTypeOptions = []string{"nosniff"}
)

func addHeaderIfNotSet(w http.ResponseWriter, key string, value []string) {
	header := w.Header()
	if val := header[key]; len(val) == 0 {
		header[key] = value
	}
}

// Context helpers for request-scoped error tracking
type ctxKeyError struct{}

type errorHolder struct {
	err error
}

// WithErrorTracking creates context with error tracking capability, or returns existing context if already present
func WithErrorTracking(ctx context.Context) context.Context {
	if _, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		return ctx // Already has error tracking
	}
	holder := &errorHolder{}
	return context.WithValue(ctx, ctxKeyError{}, holder)
}

// SetError sets error in the context
func SetError(ctx context.Context, err error) {
	if holder, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		holder.err = err
	}
}

// Error gets error from context
func Error(ctx context.Context) error {
	if holder, ok := ctx.Value(ctxKeyError{}).(*errorHolder); ok {
		return holder.err
	}
	return nil
}

// HTTP handler utilities
type HandlerFunc func(http.ResponseWriter, *http.Request) http.HandlerFunc

func (h HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := WithErrorTracking(r.Context())
	r = r.WithContext(ctx)

	if handler := h(w, r); handler != nil {
		handler(w, r)
	}
}

// JSON creates a handler that returns JSON response
func JSON(data any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		addHeaderIfNotSet(w, contentTypeHeader, jsonContentType)
		addHeaderIfNotSet(w, contentTypeOptions, nosniffContentTypeOptions)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(data)
	}
}

// JsonError creates a handler that sets an error in context and writes the error response
func JsonError(err HTTPError) http.HandlerFunc {
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
