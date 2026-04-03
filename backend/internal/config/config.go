package config

import (
	"os"
	"strconv"
)

// Config holds application-wide settings, loaded from environment variables
// with sensible defaults.
type Config struct {
	SessionMaxAge     int // seconds; default 604800 (7 days)
	MaxKeepersPerTeam int // default 3
	DefaultSeason     int // default 2026
	DefaultBudget     int // default 200
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		SessionMaxAge:     envInt("SESSION_MAX_AGE", 604800),
		MaxKeepersPerTeam: envInt("MAX_KEEPERS", 3),
		DefaultSeason:     envInt("DEFAULT_SEASON", 2026),
		DefaultBudget:     envInt("DEFAULT_BUDGET", 200),
	}
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
