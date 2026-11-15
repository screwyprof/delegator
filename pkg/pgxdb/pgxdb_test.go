package pgxdb_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/screwyprof/delegator/pkg/pgxdb"
)

// NewConnection's errors carry pgx's own cause, so callers can tell failures apart.
func TestNewConnection(t *testing.T) {
	t.Parallel()

	t.Run("it keeps the driver's error for an invalid connection string", func(t *testing.T) {
		t.Parallel()

		// Act
		_, err := pgxdb.NewConnection(t.Context(), "postgres://user:pass@localhost:notaport/db")

		// Assert
		require.ErrorIs(t, err, pgxdb.ErrInvalidConnectionString)

		var parseErr *pgconn.ParseConfigError
		assert.ErrorAs(t, err, &parseErr, "callers should be able to inspect pgx's own parse error")
	})

	t.Run("it reports a cancelled context as cancellation, not just a failed connection", func(t *testing.T) {
		t.Parallel()

		// Arrange
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		// Act
		_, err := pgxdb.NewConnection(ctx, "postgres://user:pass@127.0.0.1:1/db?sslmode=disable")

		// Assert
		require.ErrorIs(t, err, pgxdb.ErrDatabaseConnection)
		assert.ErrorIs(t, err, context.Canceled, "a shutdown must be distinguishable from a database failure")
	})
}
