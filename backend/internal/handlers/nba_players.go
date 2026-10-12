package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type nbaPlayerDetailResp struct {
	ESPNID       string             `json:"espn_id"`
	Name         string             `json:"name"`
	Position     string             `json:"position"`
	Team         string             `json:"team"`
	HeadshotURL  string             `json:"headshot_url"`
	BirthDate    string             `json:"birth_date"`
	Height       int                `json:"height"` // inches
	Weight       int                `json:"weight"` // pounds
	College      string             `json:"college"`
	YearsExp     int                `json:"years_exp"`
	JerseyNumber int                `json:"jersey_number"`
	Stats        []nbaCategoryEntry `json:"stats"`
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

	respondJSON(w, http.StatusOK, p)
}
