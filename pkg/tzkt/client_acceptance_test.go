//go:build acceptance

package tzkt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTzktClientGetDelegations(t *testing.T) {
	t.Skip("TODO: Implement HTTP call to Tzkt API - part of next TDD cycle")
	t.Parallel()

	// Arrange
	client := NewClient()

	// Act - Call the real Tzkt API
	delegations, err := client.GetDelegations(context.Background(), GetDelegationsRequest{
		Limit: 10,
	})

	// Assert - Now we expect actual delegations from the API
	require.NoError(t, err)
	assert.NotNil(t, delegations)
	assert.NotEmpty(t, delegations, "Expected to receive delegations from Tzkt API, but got empty slice. Check if API is responding or if our HTTP implementation is working.")
}
