package logger

import (
	"log/slog"
	"os"
)

const BritishTimeFormat = "02.01.2006 15:04:05"

// Config represents logger configuration from environment/config
// LogLevel is a string like "debug", "info", "error";
// LogHumanFriendly toggles between text (true) and JSON (false).
type Config struct {
	LogLevel         string
	LogHumanFriendly bool
}

// ParseLevel converts a string to slog.Level, defaulting to Info on error.
func ParseLevel(level string) slog.Level {
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(level))
	if err != nil {
		return slog.LevelInfo
	}
	return lvl
}

// NewFromConfig creates a slog.Logger based on Config.
func NewFromConfig(cfg Config) *slog.Logger {
	lvl := ParseLevel(cfg.LogLevel)
	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: false,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				// Format time as British timestamp
				return slog.String(slog.TimeKey, a.Value.Time().Format(BritishTimeFormat))
			}
			return a
		},
	}

	var handler slog.Handler
	if cfg.LogHumanFriendly {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	return slog.New(handler)
}
