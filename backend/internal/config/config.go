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

	// MockYahoo (YAHOO_MOCK=1) serves synthetic Yahoo Fantasy data instead of
	// calling the real API, and enables a password-less dev login. Development
	// only — it makes every league route reachable without authenticating
	// against Yahoo, so it must never be set in a deployed environment.
	MockYahoo bool
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() Config {
	return Config{
		SessionMaxAge:     envInt("SESSION_MAX_AGE", 604800),
		MaxKeepersPerTeam: envInt("MAX_KEEPERS", 3),
		DefaultSeason:     envInt("DEFAULT_SEASON", 2026),
		DefaultBudget:     envInt("DEFAULT_BUDGET", 200),
		MockYahoo:         envInt("YAHOO_MOCK", 0) == 1,
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
