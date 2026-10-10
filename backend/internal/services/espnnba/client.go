// Package espnnba fetches NBA roster and real-game box-score data from
// ESPN's unofficial site.api.espn.com endpoints — the same hidden API family
// already proven reliable in this app for NFL (see internal/services/espnlive),
// now the sole data source for native NBA leagues since no nflverse-equivalent
// bulk NBA source with a maintained ID crosswalk exists (see
// .claude/plans and migrations/000033_nba_players.up.sql). Unauthenticated,
// undocumented, best-effort — same caveats as espnlive.
package espnnba

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	scoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/scoreboard"
	summaryURL    = "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/summary"
	rosterURL     = "https://site.api.espn.com/apis/site/v2/sports/basketball/nba/teams"
	userAgent     = "Mozilla/5.0 (compatible; fantasy-sports-nba/1.0)"
	// NumTeams is how many team ids (1..NumTeams) the roster endpoint covers.
	NumTeams = 30
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

func get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("espn: %s returned %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ScoreboardResponse is the subset of ESPN's scoreboard JSON we care about.
type ScoreboardResponse struct {
	Events []ScoreboardEvent `json:"events"`
}

type ScoreboardEvent struct {
	ID           string `json:"id"`
	Competitions []struct {
		Status struct {
			Type struct {
				State string `json:"state"` // "pre" | "in" | "post"
			} `json:"type"`
		} `json:"status"`
	} `json:"competitions"`
}

// State returns the event's live status ("pre"/"in"/"post"), or "" if the
// response didn't include a competition (shouldn't happen in practice).
func (e ScoreboardEvent) State() string {
	if len(e.Competitions) == 0 {
		return ""
	}
	return e.Competitions[0].Status.Type.State
}

// FetchScoreboard returns NBA games on the given date (YYYYMMDD, ESPN's own
// date-query format).
func FetchScoreboard(ctx context.Context, yyyymmdd string) ([]ScoreboardEvent, error) {
	var out ScoreboardResponse
	url := fmt.Sprintf("%s?dates=%s", scoreboardURL, yyyymmdd)
	if err := get(ctx, url, &out); err != nil {
		return nil, fmt.Errorf("fetch scoreboard %s: %w", yyyymmdd, err)
	}
	return out.Events, nil
}

// SummaryResponse is the subset of ESPN's game-summary JSON we care about.
type SummaryResponse struct {
	Boxscore struct {
		Players []TeamBoxscore `json:"players"`
	} `json:"boxscore"`
}

type TeamBoxscore struct {
	Team struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"team"`
	// NBA's box score is one flat stat group per team (unlike NFL's
	// passing/rushing/receiving/... split) — Statistics has exactly one
	// entry in practice, but kept as a slice to match the real response shape.
	Statistics []StatGroup `json:"statistics"`
}

type StatGroup struct {
	Keys     []string          `json:"keys"` // e.g. "fieldGoalsMade-fieldGoalsAttempted"
	Athletes []AthleteStatLine `json:"athletes"`
}

type AthleteStatLine struct {
	Athlete struct {
		ID          string `json:"id"` // ESPN athlete id — nba_players.espn_id
		DisplayName string `json:"displayName"`
	} `json:"athlete"`
	Stats []string `json:"stats"` // positionally aligned with the group's Keys
}

// FetchBoxscore returns the per-team stat line for one completed game.
func FetchBoxscore(ctx context.Context, espnEventID string) ([]TeamBoxscore, error) {
	var out SummaryResponse
	url := fmt.Sprintf("%s?event=%s", summaryURL, espnEventID)
	if err := get(ctx, url, &out); err != nil {
		return nil, fmt.Errorf("fetch boxscore %s: %w", espnEventID, err)
	}
	return out.Boxscore.Players, nil
}

// RosterResponse is the subset of ESPN's team-roster JSON we care about.
type RosterResponse struct {
	Team struct {
		Abbreviation string `json:"abbreviation"`
	} `json:"team"`
	Athletes []RosterAthlete `json:"athletes"`
}

type RosterAthlete struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Height      float64 `json:"height"` // inches
	Weight      float64 `json:"weight"` // pounds
	DateOfBirth string  `json:"dateOfBirth"`
	Jersey      string  `json:"jersey"`
	Headshot    struct {
		Href string `json:"href"`
	} `json:"headshot"`
	Position struct {
		Abbreviation string `json:"abbreviation"` // "G" | "F" | "C" — verified against real data, no finer split exists
	} `json:"position"`
	College struct {
		Name string `json:"name"`
	} `json:"college"`
	Experience struct {
		Years int `json:"years"`
	} `json:"experience"`
}

// FetchRoster returns one team's full roster (teamID 1..NumTeams).
func FetchRoster(ctx context.Context, teamID int) (RosterResponse, error) {
	var out RosterResponse
	url := fmt.Sprintf("%s/%d/roster", rosterURL, teamID)
	if err := get(ctx, url, &out); err != nil {
		return out, fmt.Errorf("fetch roster %d: %w", teamID, err)
	}
	return out, nil
}
