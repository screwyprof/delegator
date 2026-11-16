// Package testcfg holds settings used only by the tzkt client's tests, read from TZKT_TEST_*.
package testcfg

import (
	"time"

	env "github.com/caarlos0/env/v11"
)

// Config holds test-specific configuration for tzkt client acceptance tests.
type Config struct {
	Limit uint64 `env:"TZKT_TEST_LIMIT" envDefault:"5"`
	// AfterID anchors the live-API test on old delegations, which never change, so its data is
	// stable from run to run.
	AfterID     int64         `env:"TZKT_TEST_AFTER_ID"     envDefault:"1939557726552064"`
	HTTPTimeout time.Duration `env:"TZKT_TEST_HTTP_TIMEOUT" envDefault:"30s"`
	BaseURL     string        `env:"TZKT_TEST_BASE_URL"     envDefault:"https://api.tzkt.io"`
}

// New loads test configuration from environment variables.
func New() Config {
	return env.Must(env.ParseAs[Config]())
}
