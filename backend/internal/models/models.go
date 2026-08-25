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
	LeagueID  int64              `json:"league_id"`
	NumTeams  int                `json:"num_teams"`
	Budget    int                `json:"budget"`
	Slots     map[string]int     `json:"slots"`
	Scoring   map[string]float64 `json:"scoring"`
	TaxiSlots int                `json:"taxi_slots"`
	IRSlots   int                `json:"ir_slots"`
	// TaxiCapPct/IRCapPct are how much of a stashed player's salary counts
	// against the hard cap (0-1) — a taxi/IR player isn't on the active
	// competitive roster, so their salary is discounted, not exempted.
	TaxiCapPct float64 `json:"taxi_cap_pct"`
	IRCapPct   float64 `json:"ir_cap_pct"`
	// DraftRounds is how many rounds a generated draft class has — a short
	// rookie draft for dynasty, a full re-draft's worth for redraft rollover.
	DraftRounds int `json:"draft_rounds"`
	// RegularSeasonWeeks is how many weeks a generated schedule covers.
	RegularSeasonWeeks int `json:"regular_season_weeks"`
	// RookieScale configures how a drafted player's contract is derived from
	// his draft slot — see internal/handlers/rookie_scale.go.
	RookieScale RookieScale `json:"rookie_scale"`
}

// RookieScale prices a rookie contract off the league's own real
// auction-value board rather than a typed-in number: TopPct/BottomPct pick
// the percentile range of that board a draft class spans (pick 1.01 prices
// at TopPct, the class's last pick at BottomPct), and YearsByRound sets
// contract length per round (keyed by round number as a string, matching
// Slots/Scoring's string-keyed JSONB convention). A zero/missing field means
// "use the default," not "explicitly zero" — unlike TaxiCapPct/IRCapPct,
// there's no real league that would deliberately want e.g. TopPct=0, so no
// pointer indirection is needed to tell "unset" from "zero."
type RookieScale struct {
	TopPct       float64        `json:"top_pct"`
	BottomPct    float64        `json:"bottom_pct"`
	YearsByRound map[string]int `json:"years_by_round"`
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
