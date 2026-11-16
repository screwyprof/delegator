package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/screwyprof/delegator/web/handler"
)

var errDatabaseDown = errors.New("dial tcp 10.0.0.5:5432: connect: connection refused")

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

// The health endpoint reports whether the service can reach its database, without exposing why
// it can't.
func TestHealth(t *testing.T) {
	t.Parallel()

	serve := func(t *testing.T, p handler.Pinger) *httptest.ResponseRecorder {
		t.Helper()

		mux := http.NewServeMux()
		handler.NewHealth(p).AddRoutes(mux)

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

		return rec
	}

	t.Run("it reports healthy when the database answers", func(t *testing.T) {
		t.Parallel()

		rec := serve(t, fakePinger{})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
	})

	t.Run("it reports unavailable when the database doesn't answer", func(t *testing.T) {
		t.Parallel()

		rec := serve(t, fakePinger{err: errDatabaseDown})

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.NotContains(t, rec.Body.String(), "10.0.0.5", "internal details must not leak")
	})
}
