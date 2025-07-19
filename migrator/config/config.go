package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds configuration for the migrator service
type Config struct {
	// Database configuration
	DatabaseURL string `env:"MIGRATOR_DATABASE_URL" envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"`

	// Migration configuration
	MigrationsDir string `env:"MIGRATOR_MIGRATIONS_DIR" envDefault:"migrator/migrations"`

	// Initial checkpoint configuration (optional)
	InitialCheckpoint uint64 `env:"MIGRATOR_INITIAL_CHECKPOINT" envDefault:"0"`

	// Logging configuration
	LogLevel         string `env:"LOG_LEVEL" envDefault:"info"`
	LogHumanFriendly bool   `env:"LOG_HUMAN_FRIENDLY" envDefault:"false"`

	// Migration operation timeout
	OperationTimeout time.Duration `env:"MIGRATOR_OPERATION_TIMEOUT" envDefault:"30s"`
}

// New loads all configuration from environment variables
func New() Config {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		panic(err)
	}
	return cfg
}
