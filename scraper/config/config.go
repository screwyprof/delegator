package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all configuration loaded from environment variables
type Config struct {
	ChunkSize         uint64        `env:"SCRAPER_CHUNK_SIZE" envDefault:"10000"`
	PollInterval      time.Duration `env:"SCRAPER_POLL_INTERVAL" envDefault:"10s"`
	DatabaseURL       string        `env:"SCRAPER_DATABASE_URL" envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"`
	HttpClientTimeout time.Duration `env:"SCRAPER_HTTP_CLIENT_TIMEOUT" envDefault:"30s"`
	TzktAPIURL        string        `env:"SCRAPER_TZKT_API_URL" envDefault:"https://api.tzkt.io"`
	LogLevel          string        `env:"LOG_LEVEL" envDefault:"info"`
	LogHumanFriendly  bool          `env:"LOG_HUMAN_FRIENDLY" envDefault:"false"`
}

// New loads all configuration from environment variables
func New() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}
