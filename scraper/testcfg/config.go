package testcfg

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds test-specific configuration for scraper acceptance tests
// NOTE: All values are test-optimized (smaller, faster) compared to production
type Config struct {
	// Test-optimized scraper business logic parameters
	ChunkSize         uint64        `env:"SCRAPER_TEST_CHUNK_SIZE" envDefault:"1000"`     // vs 10000 in production
	PollInterval      time.Duration `env:"SCRAPER_TEST_POLL_INTERVAL" envDefault:"100ms"` // vs 10s in production
	HttpClientTimeout time.Duration `env:"SCRAPER_TEST_HTTP_CLIENT_TIMEOUT" envDefault:"30s"`
	TzktAPIURL        string        `env:"SCRAPER_TEST_TZKT_API_URL" envDefault:"https://api.tzkt.io"`
	DatabaseURL       string        `env:"SCRAPER_TEST_DATABASE_URL" envDefault:"postgres://delegator:delegator@postgres:5432/delegator?sslmode=disable"`

	// Test execution timeouts
	ShutdownTimeout time.Duration `env:"SCRAPER_TEST_SHUTDOWN_TIMEOUT" envDefault:"2s"`

	// Test database setup (for migrator/migratortest)
	Checkpoint  int64         `env:"SCRAPER_TEST_CHECKPOINT" envDefault:"1939557726552064"`
	SeedTimeout time.Duration `env:"SCRAPER_TEST_SEED_TIMEOUT" envDefault:"5s"`
}

// parseConfig wraps env.Parse to return (Config, error) for use with env.Must
func parseConfig() (Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	return cfg, err
}

// New loads test configuration from environment variables
func New() Config {
	return env.Must(parseConfig())
}
