package testcfg

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds test-specific configuration for web API acceptance tests
type Config struct {
	SeedCheckpoint   int64         `env:"WEB_TEST_SEED_CHECKPOINT" envDefault:"1939557726552064"`
	SeedChunkSize    uint64        `env:"WEB_TEST_SEED_CHUNK_SIZE" envDefault:"1000"`
	SeedTimeout      time.Duration `env:"WEB_TEST_SEED_TIMEOUT" envDefault:"5s"`
	LogLevel         string        `env:"WEB_TEST_LOG_LEVEL" envDefault:"info"`
	LogHumanFriendly bool          `env:"WEB_TEST_LOG_HUMAN_FRIENDLY" envDefault:"true"`
}

// New loads test configuration from environment variables
func New() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}
