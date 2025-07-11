//go:build acceptance

package tzkt_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/tzkt"
)

func TestTzktClientGetDelegations(t *testing.T) {
	t.Parallel()

	// Arrange
	client := tzkt.NewClient()

	// Act - Call the real Tzkt API
	delegations, err := client.GetDelegations(context.Background(), tzkt.DelegationsRequest{
		Limit: 10,
	})

	// Assert - Now we expect actual delegations from the API
	require.NoError(t, err)
	assert.NotNil(t, delegations)
	assert.NotEmpty(t, delegations, "Expected to receive delegations from Tzkt API, but got empty slice. Check if API is responding or if our HTTP implementation is working.")
}
