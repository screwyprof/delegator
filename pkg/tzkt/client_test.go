//go:build integration

package tzkt_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTzktClientParsesSuccessfulResponse(t *testing.T) {
	t.Parallel()

	// Arrange - Mock server with real Tzkt API response format
	mockResponse := `[
		{
			"type": "delegation",
			"id": 1098907648,
			"level": 109,
			"timestamp": "2018-06-30T19:30:27Z",
			"sender": {
				"address": "tz1Wit2PqodvPeuRRhdQXmkrtU8e8bRYZecd"
			},
			"amount": 25079312620,
			"status": "applied"
		},
		{
			"type": "delegation", 
			"id": 1649410048,
			"level": 167,
			"timestamp": "2018-06-30T20:29:42Z",
			"sender": {
				"address": "tz1U2ufqFdVkN2RdYormwHtgm3ityYY1uqft"
			},
			"amount": 10199999690,
			"status": "applied"
		}
	]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
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
}
