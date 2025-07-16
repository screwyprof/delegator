package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration loaded from environment variables
type Config struct {
	ChunkSize         uint64
	PollInterval      time.Duration
	DatabaseURL       string
	InitialCheckpoint uint64
	HttpClientTimeout time.Duration
	TzktAPIURL        string
	LogLevel          string
	LogHumanFriendly  bool
}

// New loads all configuration from environment variables
func New() Config {
	return Config{
		ChunkSize:         getEnvUint64("SCRAPER_CHUNK_SIZE", 10000),
		PollInterval:      getEnvDuration("SCRAPER_POLL_INTERVAL", 10*time.Second),
		DatabaseURL:       getEnv("SCRAPER_DATABASE_URL", "postgres://delegator:delegator@localhost:5432/delegator?sslmode=disable"),
		InitialCheckpoint: getEnvUint64("SCRAPER_INITIAL_CHECKPOINT", 0),
		HttpClientTimeout: getEnvDuration("SCRAPER_HTTP_CLIENT_TIMEOUT", 30*time.Second),
		TzktAPIURL:        getEnv("SCRAPER_TZKT_API_URL", "https://api.tzkt.io"),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		LogHumanFriendly:  getEnvBool("LOG_HUMAN_FRIENDLY", true),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvUint64(key string, defaultValue uint64) uint64 {
	if value := os.Getenv(key); value != "" {
		if uint64Value, err := strconv.ParseUint(value, 10, 64); err == nil {
			return uint64Value
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultValue
}
