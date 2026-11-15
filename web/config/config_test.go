package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/screwyprof/delegator/web/config"
)

// Not parallel: t.Setenv mutates process-wide environment.
func TestPublicURL(t *testing.T) {
	t.Run("it is optional", func(t *testing.T) {
		t.Setenv("WEB_PUBLIC_URL", "")

		assert.Empty(t, config.New().PublicURL.Host)
	})

	t.Run("it accepts an absolute http(s) URL, with or without a path prefix", func(t *testing.T) {
		for _, value := range []string{"https://api.example.com", "http://localhost:8080", "https://example.com/tezos/"} {
			t.Setenv("WEB_PUBLIC_URL", value)

			assert.NotPanics(t, func() { config.New() }, value)
		}
	})

	t.Run("it fails fast on a URL links can't be built from", func(t *testing.T) {
		for _, value := range []string{
			"api.example.com",             // no scheme: parses as a bare path, host would be lost
			"//api.example.com",           // no scheme
			"ftp://api.example.com",       // not http(s)
			"https://",                    // no host
			"https://api.example.com?x=1", // query would be clobbered by link params
		} {
			t.Setenv("WEB_PUBLIC_URL", value)

			assert.Panics(t, func() { config.New() }, value)
		}
	})
}
