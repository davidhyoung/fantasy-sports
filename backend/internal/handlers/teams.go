package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
	playersvc "github.com/davidyoung/fantasy-sports/backend/internal/services/players"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

// statEntry is a resolved stat label+value sent to the frontend.
// SortOrder mirrors Yahoo's sort_order: "1" = higher is better, "0" = lower is better (e.g. TO).
type statEntry struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	SortOrder string `json:"sort_order"`
}

// rosterPlayerResp is the enriched roster player shape returned by GetTeamRoster.
// Stats are mapped, filtered, and formatted before being included here.
type rosterPlayerResp struct {
	PlayerKey        string                 `json:"player_key"`
	PlayerID         string                 `json:"player_id"`
	GsisID           string                 `json:"gsis_id,omitempty"` // resolved from nfl_players.yahoo_id
	Name             yahoo.PlayerName       `json:"name"`
	TeamAbbr         string                 `json:"team_abbr"`
	DisplayPosition  string                 `json:"display_position"`
	SelectedPosition yahoo.SelectedPosition `json:"selected_position"`
	ImageURL         string                 `json:"image_url,omitempty"`
	Stats            []statEntry            `json:"stats,omitempty"`
}

func (h *Handler) ListLeagueTeams(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, league_id, name, COALESCE(yahoo_key, ''), COALESCE(user_id, 0), COALESCE(logo_url, ''), is_commissioner
		 FROM teams WHERE league_id = $1 ORDER BY id`,
		leagueID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	teams := []models.Team{}
	for rows.Next() {
		var t models.Team
		if err := rows.Scan(&t.ID, &t.LeagueID, &t.Name, &t.YahooKey, &t.UserID, &t.LogoURL, &t.IsCommissioner); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		teams = append(teams, t)
	}

	respondJSON(w, http.StatusOK, teams)
}

func (h *Handler) GetTeam(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var t models.Team
	err = h.db.QueryRow(r.Context(),
		`SELECT id, league_id, name, COALESCE(yahoo_key, ''), COALESCE(user_id, 0), COALESCE(logo_url, '')
		 FROM teams WHERE id = $1`,
		id,
	).Scan(&t.ID, &t.LeagueID, &t.Name, &t.YahooKey, &t.UserID, &t.LogoURL)
	if err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}

	respondJSON(w, http.StatusOK, t)
}

// GetTeamRoster fetches the live roster for a team from the Yahoo Fantasy API,
// enriched with per-player weekly stats when available.
//
// Query params:
//   - week=N  — specific week number (omit or use "current" for the current week)
func (h *Handler) GetTeamRoster(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// Build the Yahoo stat type string from query params.
	// ?stat_type=lastweek|season|today — or fall back to ?week=N for a specific week.
	// Default (no params): current week.
	statType := "week"
	if st := r.URL.Query().Get("stat_type"); st != "" {
		switch st {
		case "lastweek", "season", "lastmonth":
			statType = st
		case "today":
			statType = "date;date=" + time.Now().Format("2006-01-02")
		}
	} else if wStr := r.URL.Query().Get("week"); wStr != "" {
		if _, convErr := strconv.Atoi(wStr); convErr == nil {
			statType = "week;week=" + wStr
		}
	}

	// Join leagues to get the team's yahoo_key and the league's yahoo_key.
	var yahooKey, leagueYahooKey string
	err = h.db.QueryRow(r.Context(),
		`SELECT COALESCE(t.yahoo_key, ''), COALESCE(l.yahoo_key, '')
		 FROM teams t
		 JOIN leagues l ON l.id = t.league_id
		 WHERE t.id = $1`,
		id,
	).Scan(&yahooKey, &leagueYahooKey)
	if err != nil {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	if yahooKey == "" {
		respondError(w, http.StatusUnprocessableEntity, "team has no yahoo_key")
		return
	}

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[teams] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	// Fetch roster stats and league scoring categories concurrently.
	type rosterResult struct {
		players []yahoo.RosterPlayer
		err     error
	}
	type statsResult struct {
		names map[string]yahoo.LeagueStat
		err   error
	}
	rosterCh := make(chan rosterResult, 1)
	statsCh := make(chan statsResult, 1)

	go func() {
		p, err := yc.GetRosterWithStats(r.Context(), yahooKey, statType)
		rosterCh <- rosterResult{p, err}
	}()
	go func() {
		names, err := yc.GetLeagueScoringStats(r.Context(), leagueYahooKey)
		statsCh <- statsResult{names, err}
	}()

	rr := <-rosterCh
	if rr.err != nil {
		log.Printf("[teams] GetRosterWithStats %s type=%s: %v", yahooKey, statType, rr.err)
		respondError(w, http.StatusBadGateway, "failed to fetch roster from Yahoo: "+rr.err.Error())
		return
	}
	sr := <-statsCh
	if sr.err != nil {
		log.Printf("[teams] GetLeagueScoringStats leagueKey=%q: %v", leagueYahooKey, sr.err)
		// Non-fatal: continue without stat labels rather than failing the whole request.
	}

	players := rr.players
	statCats := sr.names // nil if the call failed or league has no settings

	// Build yahoo_id → gsis_id map for this roster (batch query).
	rosterKeys := make([]string, len(players))
	for i, p := range players {
		rosterKeys[i] = p.PlayerKey
	}
	yahooIDToGsis := playersvc.ResolveBatchYahooToGsis(r.Context(), h.db, rosterKeys)

	resp := make([]rosterPlayerResp, 0, len(players))
	for _, p := range players {
		entry := rosterPlayerResp{
			PlayerKey:        p.PlayerKey,
			PlayerID:         p.PlayerID,
			GsisID:           yahooIDToGsis[playersvc.YahooKeyToNumericID(p.PlayerKey)],
			Name:             p.Name,
			TeamAbbr:         p.EditorialTeamAbbr,
			DisplayPosition:  p.DisplayPosition,
			SelectedPosition: p.SelectedPosition,
			ImageURL:         p.HeadshotURL(),
		}
		if p.PlayerStats != nil {
			for _, s := range p.PlayerStats.Stats {
				// Skip truly empty values; keep "-" (no games played yet) so columns still render.
				if s.Value == "" {
					continue
				}
				// Trim trailing ".00" from whole-number floats ("312.00" → "312").
				val := s.Value
				if val != "-" {
					val = strings.TrimRight(strings.TrimRight(val, "0"), ".")
				}
				if statCats != nil {
					cat, ok := statCats[s.StatID]
					if !ok {
						continue // not a scoring category for this league
					}
					label := cat.DisplayName
					if label == "" {
						label = cat.Name
					}
					entry.Stats = append(entry.Stats, statEntry{Label: label, Value: val, SortOrder: cat.SortOrder})
				} else {
					// Fallback: use raw stat ID as label when scoring categories unavailable.
					entry.Stats = append(entry.Stats, statEntry{Label: s.StatID, Value: val, SortOrder: "1"})
				}
			}
		}
		resp = append(resp, entry)
	}

	respondJSON(w, http.StatusOK, resp)
}
