package config

import (
	"errors"
	"fmt"
	"net/url"

	env "github.com/caarlos0/env/v11"
)

// ErrInvalidConfig reports a configuration value that parsed but can't be used
var ErrInvalidConfig = errors.New("invalid configuration")

// Config holds all configuration loaded from environment variables
type Config struct {
	HTTPPort         string `env:"WEB_HTTP_PORT"      envDefault:"8080"`
	HTTPHost         string `env:"WEB_HTTP_HOST"      envDefault:"localhost"`
	DatabaseURL      string `env:"WEB_DATABASE_URL"   envDefault:"postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"`
	LogLevel         string `env:"LOG_LEVEL"          envDefault:"info"`
	LogHumanFriendly bool   `env:"LOG_HUMAN_FRIENDLY" envDefault:"false"`

	// PublicURL is the base URL clients reach the API at (e.g. https://api.example.com), used
	// for absolute pagination links. Set it behind a reverse proxy; unset, links use the
	// request's own host.
	PublicURL url.URL `env:"WEB_PUBLIC_URL"`
}

// New loads all configuration from environment variables. Like env.Must, it panics on invalid
// configuration so a misconfigured service fails at startup instead of misbehaving later.
func New() Config {
	cfg := env.Must(env.ParseAs[Config]())

	if err := validatePublicURL(cfg.PublicURL); err != nil {
		panic(err)
	}

	return cfg
}

// validatePublicURL requires an unset value or an absolute http(s) URL with a host and no
// query or fragment. Without this, a value like "api.example.com" parses as a bare path, gets
// ignored, and links silently fall back to the (often internal) request host.
func validatePublicURL(u url.URL) error {
	if u == (url.URL{}) {
		return nil
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf(
			"%w: WEB_PUBLIC_URL must be an absolute http(s) URL like https://api.example.com, got %q",
			ErrInvalidConfig,
			u.String(),
		)
	}
	return nil
}
