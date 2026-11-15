package api_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/api"
)

// Errors the API wraps, as a handler would receive them from lower layers.
var (
	errInvalidYearFormat = errors.New("invalid year parameter: year must be exactly 4 digits (YYYY format)")
	errDatabaseAuth      = errors.New("database connection failed: password authentication failed for user 'admin'")
	errUnclassified      = errors.New("some random error")
	errPerPageOutOfRange = errors.New("invalid per_page parameter: per_page must be between 1 and 100")
	errSomeValidation    = errors.New("some validation error")
	errOriginal          = errors.New("original error")
)

// API errors show clients only what is safe: a 400 carries its cause'"s
// API errors show clients only what is safe: a 400 carries its cause's message, a 500 a generic
// one, while the full cause stays available for logs.
func TestAPIErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("it exposes all error details safely for BadRequest", func(t *testing.T) {
		t.Parallel()

		// Arrange - any validation error (all 4xx are safe by design)
		validationErr := errInvalidYearFormat

		// Act
		apiErr := api.BadRequest(validationErr)

		// Assert
		assert.Equal(t, http.StatusBadRequest, apiErr.HTTPCode())
		assert.Equal(t, "invalid year parameter: year must be exactly 4 digits (YYYY format)", apiErr.Error())
		assert.Equal(t, validationErr, apiErr.Cause())
	})

	t.Run("it hides sensitive details for InternalServerError", func(t *testing.T) {
		t.Parallel()

		// Arrange - internal database error (should NOT be exposed)
		internalErr := errDatabaseAuth

		// Act
		apiErr := api.InternalServerError(internalErr)

		// Assert
		assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPCode())
		assert.Equal(t, "Internal Server Error", apiErr.Error()) // Generic message, no sensitive info
		assert.Equal(t, internalErr, apiErr.Cause())             // Original error still available for logging
	})

	t.Run("it classifies unknown errors as InternalServerError", func(t *testing.T) {
		t.Parallel()

		// Arrange
		unknownErr := errUnclassified

		// Act
		apiErr := api.Wrap(unknownErr)

		// Assert
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusInternalServerError, apiErr.HTTPCode())
		assert.Equal(t, "Internal Server Error", apiErr.Error())
		assert.Equal(t, unknownErr, apiErr.Cause())
	})

	t.Run("it creates correct JSON structure when marshaling", func(t *testing.T) {
		t.Parallel()

		// Arrange
		validationErr := errPerPageOutOfRange
		apiErr := api.BadRequest(validationErr)

		// Act
		jsonBytes, err := json.Marshal(apiErr)

		// Assert
		require.NoError(t, err)

		// Strict decode: the body must be exactly {code, message}, nothing else.
		var response struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}

		decoder := json.NewDecoder(bytes.NewReader(jsonBytes))
		decoder.DisallowUnknownFields()
		require.NoError(t, decoder.Decode(&response))

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, "invalid per_page parameter: per_page must be between 1 and 100", response.Message)
	})

	t.Run("it prevents double-wrapping of API errors", func(t *testing.T) {
		t.Parallel()

		// Arrange
		originalErr := errSomeValidation
		apiErr1 := api.BadRequest(originalErr)

		// Act - try to wrap an already wrapped error
		apiErr2 := api.Wrap(apiErr1)

		// Assert - should return the same error, not double-wrap
		assert.Same(t, apiErr1, apiErr2)
	})

	t.Run("it supports error unwrapping correctly", func(t *testing.T) {
		t.Parallel()

		// Arrange
		originalErr := errOriginal
		apiErr := api.BadRequest(originalErr)

		// Act & Assert - errors.Is should work through the wrapper
		require.ErrorIs(t, apiErr, originalErr)
		assert.Equal(t, originalErr, errors.Unwrap(apiErr))
	})

	t.Run("it returns nil when wrapping a nil error", func(t *testing.T) {
		t.Parallel()

		// Act
		result := api.Wrap(nil)

		// Assert
		assert.Nil(t, result)
	})
}
