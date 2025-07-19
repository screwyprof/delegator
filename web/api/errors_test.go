package api_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/web/api"
)

func TestAPIErrorHandling(t *testing.T) {
	t.Parallel()

	t.Run("it exposes all error details safely for BadRequest", func(t *testing.T) {
		t.Parallel()

		// Arrange - any validation error (all 4xx are safe by design)
		validationErr := errors.New("invalid year parameter: year must be exactly 4 digits (YYYY format)")

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
		internalErr := errors.New("database connection failed: password authentication failed for user 'admin'")

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
		unknownErr := errors.New("some random error")

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
		validationErr := errors.New("invalid per_page parameter: per_page must be between 1 and 100")
		apiErr := api.BadRequest(validationErr)

		// Act
		jsonBytes, err := json.Marshal(apiErr)

		// Assert
		require.NoError(t, err)

		var response map[string]any
		err = json.Unmarshal(jsonBytes, &response)
		require.NoError(t, err)

		assert.Equal(t, float64(http.StatusBadRequest), response["code"])
		assert.Equal(t, "invalid per_page parameter: per_page must be between 1 and 100", response["message"])
	})

	t.Run("it prevents double-wrapping of API errors", func(t *testing.T) {
		t.Parallel()

		// Arrange
		originalErr := errors.New("some validation error")
		apiErr1 := api.BadRequest(originalErr)

		// Act - try to wrap an already wrapped error
		apiErr2 := api.Wrap(apiErr1)

		// Assert - should return the same error, not double-wrap
		assert.Same(t, apiErr1, apiErr2)
	})

	t.Run("it supports error unwrapping correctly", func(t *testing.T) {
		t.Parallel()

		// Arrange
		originalErr := errors.New("original error")
		apiErr := api.BadRequest(originalErr)

		// Act & Assert - errors.Is should work through the wrapper
		assert.True(t, errors.Is(apiErr, originalErr))
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
