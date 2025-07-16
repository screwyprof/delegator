package tzkt_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

func TestTzktClientGetDelegations(t *testing.T) {
	t.Parallel()

	t.Run("it parses successful response", func(t *testing.T) {
		t.Parallel()

		// Arrange
		expectedDelegations := []tzkt.Delegation{
			createTestDelegation(1098907648, int64(109), "2018-06-30T19:30:27Z", "tz1Wit2PqodvPeuRRhdQXmkrtU8e8bRYZecd", 25079312620),
			createTestDelegation(1649410048, int64(167), "2018-06-30T20:29:42Z", "tz1U2ufqFdVkN2RdYormwHtgm3ityYY1uqft", 10199999690),
		}

		server := httptest.NewServer(successHandler(t, expectedDelegations))
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 2,
		})

		// Assert
		assertDelegationsReceived(t, err, delegations, 2)
		assertParsedDelegationsMatch(t, expectedDelegations, delegations)
	})

	t.Run("it handles malformed URL", func(t *testing.T) {
		t.Parallel()

		// Arrange - Create client with invalid URL that will cause http.NewRequestWithContext to fail
		client := tzkt.NewClient(&http.Client{}, "http://a b.com/") // Invalid URL with space

		// Act
		delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertAPIError(t, err, tzkt.ErrMalformedRequest, delegations)
	})

	t.Run("it handles HTTP request failure", func(t *testing.T) {
		t.Parallel()

		// Arrange - Create client with URL that will cause HTTP request to fail
		client := tzkt.NewClient(&http.Client{}, "http://invalid-nonexistent-domain.local")

		// Act
		delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertAPIError(t, err, tzkt.ErrHTTPRequestFailed, delegations)
	})

	t.Run("it handles unexpected status code", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := newServerWithStatusCode(t, http.StatusInternalServerError)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertAPIError(t, err, tzkt.ErrUnexpectedStatus, delegations)
	})

	t.Run("it handles malformed response body", func(t *testing.T) {
		t.Parallel()

		// Arrange
		server := newServerWithInvalidJSON(t)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertAPIError(t, err, tzkt.ErrMalformedResponseBody, delegations)
	})

	t.Run("it uses provided limit when specified", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 25,
		})

		// Assert
		assertURLContainsParam(t, err, requestURL, "limit=25")
	})

	t.Run("it uses default limit when not specified", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 0,
		})

		// Assert
		assertURLContainsParam(t, err, requestURL, "limit=100")
	})

	t.Run("it excludes offset parameter when zero", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit:  10,
			Offset: 0,
		})

		// Assert
		assertURLContainsParam(t, err, requestURL, "limit=10")
		assertURLExcludesParam(t, err, requestURL, "offset")
	})

	t.Run("it includes offset parameter when specified", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit:  10,
			Offset: 50,
		})

		// Assert
		assertURLContainsParam(t, err, requestURL, "limit=10")
		assertURLContainsParam(t, err, requestURL, "offset=50")
	})

	t.Run("it includes select parameter for necessary fields", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertSelectParameterContainsAllRequiredFields(t, err, requestURL)
	})

	t.Run("it excludes id.gt parameter when nil", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertIDFilterNotPresent(t, err, requestURL)
	})

	t.Run("it includes id.gt parameter when specified", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)
		idFilter := int64(12345)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit:         10,
			IDGreaterThan: &idFilter,
		})

		// Assert
		assertIDFilterPresent(t, err, requestURL, idFilter)
	})

	t.Run("it excludes timestamp.ge parameter when nil", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assertTimestampFilterNotPresent(t, err, requestURL)
	})

	t.Run("it includes timestamp.ge parameter when specified", func(t *testing.T) {
		t.Parallel()

		// Arrange
		var requestURL string
		server := newURLTrackingServer(t, &requestURL)
		defer server.Close()

		client := newClientWithServer(server)
		timestampFilter := time.Date(2024, 12, 1, 10, 0, 0, 0, time.UTC)

		// Act
		_, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
			Limit:       10,
			TimestampGE: &timestampFilter,
		})

		// Assert
		assertTimestampFilterPresent(t, err, requestURL, timestampFilter)
	})
}

func createTestDelegation(id int64, level int64, timestamp, address string, amount int64) tzkt.Delegation {
	parsedTime, _ := time.Parse(time.RFC3339, timestamp)
	return tzkt.Delegation{
		ID:        id,
		Level:     level,
		Timestamp: parsedTime,
		Sender: struct {
			Address string `json:"address"`
		}{
			Address: address,
		},
		Amount: amount,
	}
}

func successHandler(t *testing.T, delegations []tzkt.Delegation) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		response, err := json.Marshal(delegations)
		require.NoError(t, err, "Failed to marshal test data")

		_, err = w.Write(response)
		require.NoError(t, err, "Failed to write response")
	}
}

func newServerWithStatusCode(t *testing.T, statusCode int) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
	}))
}

func newServerWithInvalidJSON(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"invalid json": broken`))
		require.NoError(t, err, "Failed to write malformed response")
	}))
}

func newURLTrackingServer(t *testing.T, urlCapture *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*urlCapture = r.URL.String()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`[]`))
		require.NoError(t, err, "Failed to write response")
	}))
}

func newClientWithServer(server *httptest.Server) *tzkt.Client {
	return tzkt.NewClient(server.Client(), server.URL)
}

func assertAPIError(t *testing.T, err error, expectedError error, delegations []tzkt.Delegation) {
	t.Helper()
	assert.ErrorIs(t, err, expectedError)
	assert.Nil(t, delegations)
}

func assertDelegationsReceived(t *testing.T, err error, delegations []tzkt.Delegation, expectedCount int) {
	t.Helper()
	require.NoError(t, err)
	require.Len(t, delegations, expectedCount, "Expected to receive %d delegations", expectedCount)
}

func assertURLContainsParam(t *testing.T, err error, requestURL, expectedParam string) {
	t.Helper()
	require.NoError(t, err)
	assert.Contains(t, requestURL, expectedParam, "Expected URL to contain parameter: %s", expectedParam)
}

func assertURLExcludesParam(t *testing.T, err error, requestURL, excludedParam string) {
	t.Helper()
	require.NoError(t, err)
	assert.NotContains(t, requestURL, excludedParam, "Expected URL to exclude parameter: %s", excludedParam)
}

func assertParsedDelegationsMatch(t *testing.T, expected, actual []tzkt.Delegation) {
	t.Helper()
	assert.Equal(t, expected, actual, "Parsed delegations should match expected values")
}

func assertSelectParameterContainsAllRequiredFields(t *testing.T, err error, requestURL string) {
	t.Helper()
	require.NoError(t, err)

	requiredFields := []string{"id", "timestamp", "amount", "sender", "level"}

	assert.Contains(t, requestURL, "select=", "Expected URL to contain select parameter")

	for _, field := range requiredFields {
		assert.Contains(t, requestURL, field,
			"Expected select parameter to include required field: %s", field)
	}
}

func assertIDFilterNotPresent(t *testing.T, err error, requestURL string) {
	t.Helper()
	require.NoError(t, err)
	assert.NotContains(t, requestURL, "id.gt", "Expected no ID-based continuation filtering when not specified")
}

func assertIDFilterPresent(t *testing.T, err error, requestURL string, expectedID int64) {
	t.Helper()
	require.NoError(t, err)
	expectedParam := fmt.Sprintf("id.gt=%d", expectedID)
	assert.Contains(t, requestURL, expectedParam, "Expected continuation from ID %d", expectedID)
}

func assertTimestampFilterNotPresent(t *testing.T, err error, requestURL string) {
	t.Helper()
	require.NoError(t, err)
	assert.NotContains(t, requestURL, "timestamp.ge", "Expected no time-based backfill filtering when not specified")
}

func assertTimestampFilterPresent(t *testing.T, err error, requestURL string, expectedTime time.Time) {
	t.Helper()
	require.NoError(t, err)
	// Note: URL encoding will encode : as %3A, so we check for the core timestamp components
	assert.Contains(t, requestURL, "timestamp.ge=", "Expected backfill filtering from timestamp")
	assert.Contains(t, requestURL, "2024-12-01T10", "Expected backfill from time %v", expectedTime)
}
