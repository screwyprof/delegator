// Package config loads the migrator's configuration from MIGRATOR_* environment variables.
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds configuration for the migrator service.
type Config struct {
	// Database configuration
	DatabaseURL string `env:"MIGRATOR_DATABASE_URL" envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"` //nolint:lll // a struct tag can't be wrapped

	// Migration configuration
	MigrationsDir string `env:"MIGRATOR_MIGRATIONS_DIR" envDefault:"migrator/migrations"`

	// Initial checkpoint configuration (optional)
	InitialCheckpoint uint64 `env:"MIGRATOR_INITIAL_CHECKPOINT" envDefault:"0"`

	// Logging configuration
	LogLevel         string `env:"LOG_LEVEL"          envDefault:"info"`
	LogHumanFriendly bool   `env:"LOG_HUMAN_FRIENDLY" envDefault:"false"`

	// OperationTimeout bounds connecting to the database and the checkpoint step. Migrations are
	// bounded only by SIGINT/SIGTERM: their duration depends on the size of the table.
	OperationTimeout time.Duration `env:"MIGRATOR_OPERATION_TIMEOUT" envDefault:"30s"`
}

// New loads all configuration from environment variables.
func New() Config {
	return env.Must(env.ParseAs[Config]())
}
