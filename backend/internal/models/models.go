package models

import "time"

// contextKey is an unexported type for context keys in this package,
// preventing collisions with keys from other packages.
type contextKey string

// UserContextKey is the key used to store the logged-in user in a request context.
const UserContextKey contextKey = "user"

// User represents an authenticated user whose identity comes from Yahoo OAuth.
// Tokens are intentionally omitted from JSON so they are never sent to the frontend.
type User struct {
	ID          int64     `json:"id"`
	YahooGUID   string    `json:"yahoo_guid"`
	DisplayName string    `json:"display_name"`
	Email       string    `json:"email,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type League struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Sport    string `json:"sport"`
	Season   string `json:"season"`
	YahooKey string `json:"yahoo_key,omitempty"`
	LogoURL  string `json:"logo_url,omitempty"`
	// Source is "yahoo" (synced from a live Yahoo league) or "native" (no
	// external league at all — this app is the system of record).
	Source string `json:"source"`
	// Format selects the season-rollover strategy: "redraft" | "keeper" | "dynasty".
	Format    string    `json:"format"`
	CreatedAt time.Time `json:"created_at"`
}

// LeagueSettings holds the canonical roster/scoring configuration for a native
// league, in the same vocabulary the draft-values ?slots=&scoring= overrides
// use (see internal/services/leaguesettings).
type LeagueSettings struct {
	LeagueID    int64              `json:"league_id"`
	NumTeams    int                `json:"num_teams"`
	Budget      int                `json:"budget"`
	Slots       map[string]int     `json:"slots"`
	Scoring     map[string]float64 `json:"scoring"`
	TaxiSlots   int                `json:"taxi_slots"`
	IRSlots     int                `json:"ir_slots"`
	// DraftRounds is how many rounds a generated draft class has — a short
	// rookie draft for dynasty, a full re-draft's worth for redraft rollover.
	DraftRounds int `json:"draft_rounds"`
}

type Team struct {
	ID             int64  `json:"id"`
	LeagueID       int64  `json:"league_id"`
	Name           string `json:"name"`
	YahooKey       string `json:"yahoo_key,omitempty"`
	UserID         int64  `json:"user_id,omitempty"`
	LogoURL        string `json:"logo_url,omitempty"`
	IsCommissioner bool   `json:"is_commissioner,omitempty"`
}

type Player struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Sport      string `json:"sport"`
	Position   string `json:"position"`
	ExternalID string `json:"external_id,omitempty"`
}

type RosterEntry struct {
	TeamID   int64  `json:"team_id"`
	PlayerID int64  `json:"player_id"`
	Slot     string `json:"slot"`
}
