package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server bounds every phase of a connection, so a slow or idle client can't hold one open.
func TestNewServer(t *testing.T) {
	t.Parallel()

	// Act
	server := newServer("localhost:0", http.NewServeMux())

	// Assert: without a header deadline a client can hold a connection open forever by
	// trickling request headers (Slowloris); ReadTimeout and WriteTimeout bound the rest.
	assert.Positive(t, server.ReadHeaderTimeout, "ReadHeaderTimeout must be set")
	assert.Positive(t, server.ReadTimeout, "ReadTimeout must be set")
	assert.Positive(t, server.WriteTimeout, "WriteTimeout must be set")
	assert.Positive(t, server.IdleTimeout, "IdleTimeout must be set")
}

// `web healthcheck` exits zero only for a 200 from /healthz, so a container healthcheck in the
// scratch image (no curl) can tell a working service from one that can't reach its database.
func TestCheckHealth(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		status  int
		healthy bool
	}{
		"healthy":     {status: http.StatusOK, healthy: true},
		"unavailable": {status: http.StatusServiceUnavailable, healthy: false},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/healthz", r.URL.Path)
			w.WriteHeader(tc.status)
		}))

		err := checkHealth(t.Context(), server.URL)
		server.Close()

		if tc.healthy {
			require.NoError(t, err, name)
		} else {
			require.Error(t, err, name)
		}
	}

	assert.Error(t, checkHealth(t.Context(), "http://127.0.0.1:1"), "unreachable")
}
