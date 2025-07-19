package testcfg

import (
	"github.com/caarlos0/env/v11"
)

// Config holds test-specific configuration for web API acceptance tests
type Config struct {
	LogLevel         string `env:"WEB_TEST_LOG_LEVEL" envDefault:"info"`
	LogHumanFriendly bool   `env:"WEB_TEST_LOG_HUMAN_FRIENDLY" envDefault:"true"`
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
