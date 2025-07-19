package testcfg

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds test-specific configuration for tzkt client acceptance tests
type Config struct {
	Limit       uint64        `env:"TZKT_TEST_LIMIT" envDefault:"5"`
	Offset      uint64        `env:"TZKT_TEST_OFFSET" envDefault:"100000"`
	HTTPTimeout time.Duration `env:"TZKT_TEST_HTTP_TIMEOUT" envDefault:"30s"`
	BaseURL     string        `env:"TZKT_TEST_BASE_URL" envDefault:"https://api.tzkt.io"`
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
