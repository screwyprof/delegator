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

// parseConfig wraps env.Parse to return (Config, error) for use with env.Must
func parseConfig() (Config, error) {
	var cfg Config
	err := env.Parse(&cfg)
	return cfg, err
}

// New loads all configuration from environment variables
func New() Config {
	return env.Must(parseConfig())
}
