//go:build acceptance

package tzkt_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

const (
	// Configuration constants for acceptance testing
	limit       = 5
	offset      = 100000 // Use older data for more stable test results
	httpTimeout = 30 * time.Second
	tzktBaseURL = "https://api.tzkt.io"
)

func TestTzktClientRealAPI(t *testing.T) {
	t.Parallel()

	// Arrange
	client := tzkt.NewClient(&http.Client{
		Timeout: httpTimeout,
	}, tzktBaseURL)

	// Act - Call the real Tzkt API with offset for more stable data
	delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
		Limit:  limit,
		Offset: offset,
	})

	// Assert - Verify we get valid delegation data structure
	require.NoError(t, err)
	assert.Len(t, delegations, limit, "Expected exactly %d delegations with limit=%d", limit, limit)

	// Verify each delegation has required fields with valid data
	for i, delegation := range delegations {
		// Basic format validation
		assert.Greater(t, delegation.Level, int64(0), "Delegation %d should have valid block level", i)
		assert.False(t, delegation.Timestamp.IsZero(), "Delegation %d should have valid timestamp", i)
		assert.NotEmpty(t, delegation.Sender.Address, "Delegation %d should have sender address", i)
		assert.GreaterOrEqual(t, delegation.Amount, int64(0), "Delegation %d should have non-negative amount", i)
		assert.Contains(t, delegation.Sender.Address, "tz", "Delegation %d sender should be Tezos address", i)

		// Verify timestamp is parseable to RFC3339 (proves it came from valid JSON)
		timestampStr := delegation.Timestamp.Format(time.RFC3339)
		assert.Contains(t, timestampStr, "T", "Delegation %d timestamp should be RFC3339 format", i)
	}
}
