package testcfg

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds the scraper acceptance test settings with no production equivalent.
// Everything else the test needs (chunk size, poll interval, HTTP timeout, TzKT URL)
// comes straight from scraper/config.New() — the test doesn't need different values for
// those, so there's nothing to redefine.
type Config struct {
	// ShutdownTimeout is how long the test waits for a clean shutdown.
	ShutdownTimeout time.Duration `env:"SCRAPER_TEST_SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// Checkpoint is the shared "demo checkpoint" — a TzKT delegation ID, not a database
	// concern — kept here (Tezos/scraper domain data) rather than in migrator/migratortest
	// (a generic pgtestdb helper with no reason to know about Tezos). It mirrors env.demo's
	// MIGRATOR_INITIAL_CHECKPOINT, kept independently so both scraper's own acceptance test
	// and web/internal/seedtestdb stay fast without requiring env.demo to be sourced.
	Checkpoint int64 `env:"TEST_CHECKPOINT" envDefault:"1939557726552064"`
}

// New loads test configuration from environment variables
func New() Config {
	return env.Must(env.ParseAs[Config]())
}
