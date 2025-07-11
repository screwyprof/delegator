package tzkt_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

func TestTzktClientGetDelegations(t *testing.T) {
	t.Parallel()

	t.Run("it parses successful response", func(t *testing.T) {
		t.Parallel()

		// Arrange - Test data as structs (only fields we actually parse)
		testDelegations := []tzkt.Delegation{
			createTestDelegation(109, "2018-06-30T19:30:27Z", "tz1Wit2PqodvPeuRRhdQXmkrtU8e8bRYZecd", 25079312620),
			createTestDelegation(167, "2018-06-30T20:29:42Z", "tz1U2ufqFdVkN2RdYormwHtgm3ityYY1uqft", 10199999690),
		}

		server := httptest.NewServer(successHandler(t, testDelegations))
		defer server.Close()

		// Create client pointing to mock server
		client := tzkt.NewClientWithHTTP(server.Client(), server.URL)

		// Act
		delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
			Limit: 2,
		})

		// Assert - Check raw API data parsed correctly
		require.NoError(t, err)
		require.Len(t, delegations, 2, "Expected to parse 2 delegations from mock response")

		// Verify first delegation parsed correctly (raw API format)
		assert.Equal(t, "2018-06-30T19:30:27Z", delegations[0].Timestamp)
		assert.Equal(t, int64(25079312620), delegations[0].Amount)
		assert.Equal(t, "tz1Wit2PqodvPeuRRhdQXmkrtU8e8bRYZecd", delegations[0].Sender.Address)
		assert.Equal(t, 109, delegations[0].Level)
	})

	t.Run("it handles malformed URL", func(t *testing.T) {
		t.Parallel()

		// Arrange - Create client with invalid URL that will cause http.NewRequestWithContext to fail
		client := tzkt.NewClientWithHTTP(&http.Client{}, "http://a b.com/") // Invalid URL with space

		// Act
		delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assert.ErrorIs(t, err, tzkt.ErrMalformedRequest)
		assert.Nil(t, delegations)
	})

	t.Run("it handles HTTP request failure", func(t *testing.T) {
		t.Parallel()

		// Arrange - Create client with URL that will cause HTTP request to fail
		client := tzkt.NewClientWithHTTP(&http.Client{}, "http://invalid-nonexistent-domain.local")

		// Act
		delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assert.ErrorIs(t, err, tzkt.ErrHTTPRequestFailed)
		assert.Nil(t, delegations)
	})

	t.Run("it handles unexpected status code", func(t *testing.T) {
		t.Parallel()

		// Arrange - Mock server that returns 500 Internal Server Error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := tzkt.NewClientWithHTTP(server.Client(), server.URL)

		// Act
		delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assert.ErrorIs(t, err, tzkt.ErrUnexpectedStatus)
		assert.Nil(t, delegations)
	})

	t.Run("it handles malformed response body", func(t *testing.T) {
		t.Parallel()

		// Arrange - Mock server that returns invalid JSON
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			_, err := w.Write([]byte(`{"invalid json": broken`))
			require.NoError(t, err, "Failed to write malformed response")
		}))
		defer server.Close()

		client := tzkt.NewClientWithHTTP(server.Client(), server.URL)

		// Act
		delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
			Limit: 10,
		})

		// Assert
		assert.ErrorIs(t, err, tzkt.ErrMalformedResponseBody)
		assert.Nil(t, delegations)
	})
}

// createTestDelegation creates a test delegation with the given parameters
func createTestDelegation(level int, timestamp, address string, amount int64) tzkt.Delegation {
	return tzkt.Delegation{
		Level:     level,
		Timestamp: timestamp,
		Sender: struct {
			Address string `json:"address"`
		}{
			Address: address,
		},
		Amount: amount,
	}
}

// successHandler creates an HTTP handler that returns the given delegations as JSON
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
