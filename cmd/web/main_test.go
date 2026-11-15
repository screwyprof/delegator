package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
