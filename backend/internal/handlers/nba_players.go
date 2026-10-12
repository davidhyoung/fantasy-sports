package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type nbaPlayerDetailResp struct {
	ESPNID       string                `json:"espn_id"`
	Name         string                `json:"name"`
	Position     string                `json:"position"`
	Team         string                `json:"team"`
	HeadshotURL  string                `json:"headshot_url"`
	BirthDate    string                `json:"birth_date"`
	Height       int                   `json:"height"` // inches
	Weight       int                   `json:"weight"` // pounds
	College      string                `json:"college"`
	YearsExp     int                   `json:"years_exp"`
	JerseyNumber int                   `json:"jersey_number"`
	Stats        []nbaCategoryEntry    `json:"stats"`
	Seasons      []nbaPlayerSeasonResp `json:"seasons"`
}

type nbaPlayerSeasonResp struct {
	Season int                `json:"season"`
	Team   string             `json:"team"`
	Games  int                `json:"games"`
	Stats  []nbaCategoryEntry `json:"stats"`
}

// nbaPlayerSeasons returns one row per real NBA season this player has any
// stats for — a career table, independent of the detail view's period
// picker above (which answers "how's this player doing lately"; this
// answers "what has their whole career looked like"). Per-game averages for
// counting categories (same convention as period "season_avg"); fg_pct/
// ft_pct are always a ratio of aggregated makes/attempts, never averaged.
// `team` is whichever team their most recent game that season was with —
// good enough for a compact table; a mid-season trade isn't split into two
// rows the way some career-stat sites do.
func (h *Handler) nbaPlayerSeasons(ctx context.Context, espnID string) ([]nbaPlayerSeasonResp, error) {
	rows, err := h.db.Query(ctx, `
		SELECT season, (array_agg(team ORDER BY game_date DESC))[1], COUNT(*),
		       COALESCE(SUM(points),0), COALESCE(SUM(rebounds),0), COALESCE(SUM(assists),0),
		       COALESCE(SUM(steals),0), COALESCE(SUM(blocks),0), COALESCE(SUM(turnovers),0),
		       COALESCE(SUM(fg3_made),0),
		       COALESCE(SUM(fg_made),0), COALESCE(SUM(fg_att),0),
		       COALESCE(SUM(ft_made),0), COALESCE(SUM(ft_att),0)
		FROM nba_player_stats
		WHERE espn_id = $1
		GROUP BY season
		ORDER BY season DESC
	`, espnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []nbaPlayerSeasonResp
	for rows.Next() {
		var s nbaPlayerSeasonResp
		var team string
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&s.Season, &team, &s.Games, &pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		s.Team = team
		divisor := 1.0
		if s.Games > 0 {
			divisor = float64(s.Games)
		}
		totals := map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
		s.Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
		for _, c := range nbaCategories {
			s.Stats = append(s.Stats, nbaCategoryEntry{Category: c, Value: totals[c]})
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetNBAPlayer returns a single NBA player's metadata plus aggregated
// category totals for the requested period (same vocabulary as the
// Players/Roster tabs' timeframe picker — see parseNBAStatPeriod) across
// every canonical category, unfiltered by any league's relevant-category
// scoring, since this is a standalone detail view rather than a
// league-scoped table. `league_id`, when given, resolves which real season
// "season"/"last_season" mean (that league's own season); omitted, it falls
// back to the server's configured default season.
//
// GET /api/nba/players/{espnId}?period=&league_id=
func (h *Handler) GetNBAPlayer(w http.ResponseWriter, r *http.Request) {
	espnID := chi.URLParam(r, "espnId")
	if espnID == "" {
		respondError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	var p nbaPlayerDetailResp
	err := h.db.QueryRow(r.Context(), `
		SELECT espn_id, name, COALESCE(position, ''), COALESCE(team, ''), COALESCE(headshot_url, ''),
		       COALESCE(to_char(birth_date, 'YYYY-MM-DD'), ''), COALESCE(height, 0), COALESCE(weight, 0),
		       COALESCE(college, ''), COALESCE(years_exp, 0), COALESCE(jersey_number, 0)
		FROM nba_players WHERE espn_id = $1
	`, espnID).Scan(&p.ESPNID, &p.Name, &p.Position, &p.Team, &p.HeadshotURL,
		&p.BirthDate, &p.Height, &p.Weight, &p.College, &p.YearsExp, &p.JerseyNumber)
	if err != nil {
		respondError(w, http.StatusNotFound, "player not found")
		return
	}

	season := h.config.DefaultSeason
	if leagueIDStr := r.URL.Query().Get("league_id"); leagueIDStr != "" {
		if leagueID, err := strconv.ParseInt(leagueIDStr, 10, 64); err == nil {
			season = h.leagueSeasonInt(r, leagueID)
		}
	}
	period := parseNBAStatPeriod(r)

	totals, err := h.nbaRawCategoryTotals(r.Context(), season, period, []string{espnID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	playerTotals := totals[espnID]
	p.Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
	for _, c := range nbaCategories {
		p.Stats = append(p.Stats, nbaCategoryEntry{Category: c, Value: playerTotals[c]})
	}

	seasons, err := h.nbaPlayerSeasons(r.Context(), espnID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Seasons = seasons

	respondJSON(w, http.StatusOK, p)
}
