//go:build acceptance

package tzkt_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
	"github.com/screwyprof/delegator/pkg/tzkt/testcfg"
)

func TestTzktClientRealAPI(t *testing.T) {
	t.Parallel()

	// Load test configuration from environment
	testCfg := testcfg.New()

	// Arrange
	client := tzkt.NewClient(&http.Client{
		Timeout: testCfg.HTTPTimeout,
	}, testCfg.BaseURL)

	// Act - Call the real Tzkt API with offset for more stable data
	delegations, err := client.GetDelegations(t.Context(), tzkt.DelegationsRequest{
		Limit:  testCfg.Limit,
		Offset: testCfg.Offset,
	})

	// Assert - Verify we get valid delegation data structure
	require.NoError(t, err)
	assert.Len(t, delegations, int(testCfg.Limit), "Expected exactly %d delegations with limit=%d", testCfg.Limit, testCfg.Limit)

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
		_, err := time.Parse(time.RFC3339, timestampStr)
		assert.NoError(t, err, "Delegation %d timestamp should be valid RFC3339: %s", i, timestampStr)

		t.Logf("Delegation %d: ID=%d, Level=%d, Amount=%d, Sender=%s, Timestamp=%s",
			i, delegation.ID, delegation.Level, delegation.Amount, delegation.Sender.Address, timestampStr)
	}
}
