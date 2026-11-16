package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/screwyprof/delegator/pkg/httpkit"
	"github.com/screwyprof/delegator/web/api"
)

// HealthRoute is the ServeMux pattern the health check is registered under.
const HealthRoute = http.MethodGet + " /healthz"

// healthTimeout bounds the database ping, so a hung database reads as unhealthy rather than
// hanging the probe.
const healthTimeout = 2 * time.Second

// Pinger reports whether a dependency answers; a *pgxpool.Pool is one.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health serves GET /healthz: 200 while the database answers, 503 while it doesn't.
type Health struct {
	db Pinger
}

// NewHealth creates the health check for db.
func NewHealth(db Pinger) *Health {
	return &Health{db: db}
}

// AddRoutes registers the health check on m at HealthRoute.
func (h *Health) AddRoutes(m *http.ServeMux) {
	m.Handle(HealthRoute, httpkit.HandlerFunc(h.Check))
}

// Check pings the database. The cause of a failure is logged by the middleware, never sent.
func (h *Health) Check(_ http.ResponseWriter, r *http.Request) http.HandlerFunc {
	ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		return httpkit.JSONError(api.ServiceUnavailable(err))
	}

	return httpkit.JSON(map[string]string{"status": "ok"})
}
