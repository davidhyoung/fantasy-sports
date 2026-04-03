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
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Sport     string    `json:"sport"`
	Season    string    `json:"season"`
	YahooKey  string    `json:"yahoo_key,omitempty"`
	LogoURL   string    `json:"logo_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
