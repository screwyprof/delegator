package config

import (
	"github.com/caarlos0/env/v11"
)

// Config holds all configuration loaded from environment variables
type Config struct {
	HTTPPort         string `env:"WEB_HTTP_PORT" envDefault:"8080"`
	HTTPHost         string `env:"WEB_HTTP_HOST" envDefault:"localhost"`
	DatabaseURL      string `env:"WEB_DATABASE_URL" envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"`
	LogLevel         string `env:"LOG_LEVEL" envDefault:"info"`
	LogHumanFriendly bool   `env:"LOG_HUMAN_FRIENDLY" envDefault:"false"`
}

// New loads all configuration from environment variables
func New() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}
