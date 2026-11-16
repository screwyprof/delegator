package pgxdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pool settings given in the connection string win over the package's defaults; the defaults
// fill in only what the string leaves out.
func TestPoolConfig(t *testing.T) {
	t.Parallel()

	const base = "postgres://u:p@localhost:5432/db?sslmode=disable"

	t.Run("it applies the defaults when the connection string sets none", func(t *testing.T) {
		t.Parallel()

		cfg, err := poolConfig(base)
		require.NoError(t, err)

		assert.Equal(t, int32(defaultMinConns), cfg.MinConns)
		assert.Equal(t, int32(defaultMaxConns), cfg.MaxConns)
		assert.Equal(t, defaultMaxConnLifetime, cfg.MaxConnLifetime)
		assert.Equal(t, defaultMaxConnIdleTime, cfg.MaxConnIdleTime)
		assert.Equal(t, defaultHealthCheckPeriod, cfg.HealthCheckPeriod)
		assert.Equal(t, defaultConnectTimeout, cfg.ConnConfig.ConnectTimeout)
	})

	t.Run("it keeps settings the connection string gives", func(t *testing.T) {
		t.Parallel()

		cfg, err := poolConfig(base + "&pool_min_conns=1&pool_max_conns=25&pool_max_conn_lifetime=1h" +
			"&pool_max_conn_idle_time=1m&pool_health_check_period=30s&connect_timeout=3")
		require.NoError(t, err)

		assert.Equal(t, int32(1), cfg.MinConns)
		assert.Equal(t, int32(25), cfg.MaxConns)
		assert.Equal(t, time.Hour, cfg.MaxConnLifetime)
		assert.Equal(t, time.Minute, cfg.MaxConnIdleTime)
		assert.Equal(t, 30*time.Second, cfg.HealthCheckPeriod)
		assert.Equal(t, 3*time.Second, cfg.ConnConfig.ConnectTimeout)
	})

	t.Run("it reads keyword/value connection strings too", func(t *testing.T) {
		t.Parallel()

		cfg, err := poolConfig("host=localhost user=u password=p dbname=db pool_max_conns=7")
		require.NoError(t, err)

		assert.Equal(t, int32(7), cfg.MaxConns)
		assert.Equal(t, int32(defaultMinConns), cfg.MinConns)
	})

	t.Run("it reads keyword/value settings written with spaces around =", func(t *testing.T) {
		t.Parallel()

		cfg, err := poolConfig("host=localhost user=u pool_max_conns = 7 connect_timeout =3")
		require.NoError(t, err)

		assert.Equal(t, int32(7), cfg.MaxConns)
		assert.Equal(t, 3*time.Second, cfg.ConnConfig.ConnectTimeout)
	})

	t.Run("it rejects an invalid connection string", func(t *testing.T) {
		t.Parallel()

		_, err := poolConfig("postgres://u:p@localhost:notaport/db")
		require.ErrorIs(t, err, ErrInvalidConnectionString)
	})
}
