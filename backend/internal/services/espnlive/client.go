// Package espnlive polls ESPN's unofficial site.api.espn.com endpoints during
// NFL games and turns their box-score stat lines into the same canonical
// stat vocabulary (internal/services/scoring) the rest of the app already
// speaks, so native leagues can show a best-effort live score preview.
//
// This is deliberately a preview-only, best-effort layer: the endpoints are
// undocumented, unauthenticated, and can change or disappear without notice.
// Nothing here ever writes to nfl_player_stats or league_matchups — see
// nfl_live_player_stats (migrations/000032_live_stats.up.sql) and
// liveweekstats.go for the isolation boundary.
package espnlive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	scoreboardURL = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/scoreboard"
	summaryURL    = "https://site.api.espn.com/apis/site/v2/sports/football/nfl/summary"
	userAgent     = "Mozilla/5.0 (compatible; fantasy-sports-live/1.0)"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

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

// scoreboardResponse is the subset of ESPN's scoreboard JSON we care about.
type scoreboardResponse struct {
	Events []scoreboardEvent `json:"events"`
}

type scoreboardEvent struct {
	ID     string `json:"id"`
	Season struct {
		Year int `json:"year"`
	} `json:"season"`
	Week struct {
		Number int `json:"number"`
	} `json:"week"`
	Competitions []struct {
		Status struct {
			Type struct {
				State string `json:"state"` // "pre" | "in" | "post"
			} `json:"type"`
		} `json:"status"`
	} `json:"competitions"`
}

// state returns the event's live status ("pre"/"in"/"post"), or "" if the
// response didn't include a competition (shouldn't happen in practice).
func (e scoreboardEvent) state() string {
	if len(e.Competitions) == 0 {
		return ""
	}
	return e.Competitions[0].Status.Type.State
}

// fetchScoreboard returns this week's NFL games and their live status.
func fetchScoreboard(ctx context.Context) ([]scoreboardEvent, error) {
	var out scoreboardResponse
	if err := get(ctx, scoreboardURL, &out); err != nil {
		return nil, fmt.Errorf("fetch scoreboard: %w", err)
	}
	return out.Events, nil
}

// summaryResponse is the subset of ESPN's game-summary JSON we care about.
type summaryResponse struct {
	Boxscore struct {
		Players []teamBoxscore `json:"players"`
	} `json:"boxscore"`
}

type teamBoxscore struct {
	Statistics []statCategory `json:"statistics"`
}

type statCategory struct {
	Name     string            `json:"name"` // "passing", "rushing", "receiving", "kicking", ...
	Keys     []string          `json:"keys"`
	Athletes []athleteStatLine `json:"athletes"`
}

type athleteStatLine struct {
	Athlete struct {
		ID string `json:"id"` // ESPN athlete id — matches nfl_players.espn_id verbatim
	} `json:"athlete"`
	Stats []string `json:"stats"` // positionally aligned with the category's Keys
}

// fetchBoxscore returns the per-team stat categories for one game.
func fetchBoxscore(ctx context.Context, espnEventID string) ([]teamBoxscore, error) {
	var out summaryResponse
	url := fmt.Sprintf("%s?event=%s", summaryURL, espnEventID)
	if err := get(ctx, url, &out); err != nil {
		return nil, fmt.Errorf("fetch boxscore %s: %w", espnEventID, err)
	}
	return out.Boxscore.Players, nil
}
