package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// rosterEntryResp is one player on a native league's roster, enriched with
// player metadata and contract terms.
type rosterEntryResp struct {
	GsisID       string    `json:"gsis_id"`
	Name         string    `json:"name"`
	Position     string    `json:"position"`
	Team         string    `json:"team"`
	HeadshotURL  string    `json:"headshot_url,omitempty"`
	TeamID       int64     `json:"team_id"`
	Slot         string    `json:"slot"`
	AcquiredVia  string    `json:"acquired_via"`
	AcquiredAt   time.Time `json:"acquired_at"`
	Salary       int       `json:"salary"`
	SignedSeason int       `json:"signed_season"`
	YearsTotal   *int      `json:"years_total"`
	YearsUsed    int       `json:"years_used"`
}

// GetLeagueRosters returns every rostered player in a native league. The
// client groups by team_id — the shape is a flat list either way once you're
// keying off it.
//
// GET /api/leagues/{id}/rosters
func (h *Handler) GetLeagueRosters(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT
			lr.gsis_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
			lr.team_id, lr.slot, lr.acquired_via, lr.acquired_at,
			COALESCE(lc.salary, 0), COALESCE(lc.signed_season, 0), lc.years_total, COALESCE(lc.years_used, 1)
		FROM league_rosters lr
		JOIN nfl_players p ON p.gsis_id = lr.gsis_id
		LEFT JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
		WHERE lr.league_id = $1
		ORDER BY lr.team_id, lr.slot, p.name
	`, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	roster := []rosterEntryResp{}
	for rows.Next() {
		var e rosterEntryResp
		if err := rows.Scan(
			&e.GsisID, &e.Name, &e.Position, &e.Team, &e.HeadshotURL,
			&e.TeamID, &e.Slot, &e.AcquiredVia, &e.AcquiredAt,
			&e.Salary, &e.SignedSeason, &e.YearsTotal, &e.YearsUsed,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		roster = append(roster, e)
	}
	respondJSON(w, http.StatusOK, roster)
}

// assignRosterReq is the body for adding a player to a native league's roster.
type assignRosterReq struct {
	GsisID       string `json:"gsis_id"`
	TeamID       int64  `json:"team_id"`
	Slot         string `json:"slot"`
	AcquiredVia  string `json:"acquired_via"`
	Salary       int    `json:"salary"`
	SignedSeason int    `json:"signed_season"`
	YearsTotal   *int   `json:"years_total"`
}

var validSlots = map[string]bool{
	"QB": true, "RB": true, "WR": true, "TE": true, "K": true, "DEF": true,
	"FLEX": true, "SFLEX": true, "BN": true, "TAXI": true, "IR": true,
}

var validAcquiredVia = map[string]bool{
	"draft": true, "auction": true, "trade": true, "waiver": true, "fa": true, "keeper": true,
}

// AssignLeagueRoster puts a player on a native league's roster and signs them
// to a contract in one transaction — every roster row gets a contract row
// (even a $0 undrafted pickup), since cap space is derived as
// budget - SUM(salary) and that math needs every rostered player accounted for.
//
// POST /api/leagues/{id}/rosters
func (h *Handler) AssignLeagueRoster(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req assignRosterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.GsisID == "" || req.TeamID == 0 {
		respondError(w, http.StatusBadRequest, "gsis_id and team_id are required")
		return
	}
	if req.Slot == "" {
		req.Slot = "BN"
	}
	if !validSlots[req.Slot] {
		respondError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	if req.AcquiredVia == "" || !validAcquiredVia[req.AcquiredVia] {
		respondError(w, http.StatusBadRequest, "invalid acquired_via")
		return
	}
	if req.Salary < 0 {
		respondError(w, http.StatusBadRequest, "salary cannot be negative")
		return
	}
	if req.SignedSeason == 0 {
		req.SignedSeason = h.leagueSeasonInt(r, leagueID)
	}

	var teamInLeague bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", req.TeamID, leagueID,
	).Scan(&teamInLeague); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !teamInLeague {
		respondError(w, http.StatusBadRequest, "team is not in this league")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO league_rosters (league_id, team_id, gsis_id, slot, acquired_via)
		VALUES ($1, $2, $3, $4, $5)
	`, leagueID, req.TeamID, req.GsisID, req.Slot, req.AcquiredVia); err != nil {
		respondError(w, http.StatusConflict, "player is already rostered in this league (or not a known player)")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO league_contracts (league_id, gsis_id, salary, signed_season, years_total, years_used)
		VALUES ($1, $2, $3, $4, $5, 1)
	`, leagueID, req.GsisID, req.Salary, req.SignedSeason, req.YearsTotal); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "rostered"})
}

// UpdateLeagueRoster moves a rostered player — to another team (a trade), a
// different slot, and/or new contract terms. Only fields present in the body
// are changed.
//
// PUT /api/leagues/{id}/rosters/{gsisId}
func (h *Handler) UpdateLeagueRoster(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	gsisID := chi.URLParam(r, "gsisId")
	if gsisID == "" {
		respondError(w, http.StatusBadRequest, "invalid player id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req struct {
		TeamID          *int64  `json:"team_id"`
		Slot            *string `json:"slot"`
		Salary          *int    `json:"salary"`
		YearsTotal      *int    `json:"years_total"`
		ClearYearsTotal bool    `json:"clear_years_total"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Slot != nil && !validSlots[*req.Slot] {
		respondError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	if req.TeamID != nil {
		var teamInLeague bool
		if err := h.db.QueryRow(r.Context(),
			"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", *req.TeamID, leagueID,
		).Scan(&teamInLeague); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !teamInLeague {
			respondError(w, http.StatusBadRequest, "team is not in this league")
			return
		}
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	if req.TeamID != nil || req.Slot != nil {
		tag, err := tx.Exec(r.Context(), `
			UPDATE league_rosters
			SET team_id = COALESCE($3, team_id), slot = COALESCE($4, slot)
			WHERE league_id = $1 AND gsis_id = $2
		`, leagueID, gsisID, req.TeamID, req.Slot)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "player is not rostered in this league")
			return
		}
	}

	if req.Salary != nil || req.YearsTotal != nil || req.ClearYearsTotal {
		if _, err := tx.Exec(r.Context(), `
			UPDATE league_contracts
			SET salary = COALESCE($3, salary),
			    years_total = CASE WHEN $4 THEN NULL ELSE COALESCE($5, years_total) END
			WHERE league_id = $1 AND gsis_id = $2
		`, leagueID, gsisID, req.Salary, req.ClearYearsTotal, req.YearsTotal); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DropLeagueRoster releases a player back to free agency. The contract row
// cascades with it (FK ON DELETE CASCADE).
//
// DELETE /api/leagues/{id}/rosters/{gsisId}
func (h *Handler) DropLeagueRoster(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	gsisID := chi.URLParam(r, "gsisId")
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	tag, err := h.db.Exec(r.Context(),
		"DELETE FROM league_rosters WHERE league_id = $1 AND gsis_id = $2", leagueID, gsisID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "player is not rostered in this league")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
}

// freeAgentResp is one player not currently rostered in a native league.
type freeAgentResp struct {
	GsisID      string   `json:"gsis_id"`
	Name        string   `json:"name"`
	Position    string   `json:"position"`
	Team        string   `json:"team"`
	HeadshotURL string   `json:"headshot_url,omitempty"`
	ProjFptsPPR *float64 `json:"proj_fpts_ppr"`
}

// GetLeagueFreeAgents returns players in a native league with no roster row —
// nfl_players minus league_rosters, ordered by projection for the league's
// target season (players with no projection sort last, not first).
//
// GET /api/leagues/{id}/free-agents?position=&season=&limit=&offset=
func (h *Handler) GetLeagueFreeAgents(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	q := r.URL.Query()
	season := h.leagueSeasonInt(r, leagueID)
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	position := q.Get("position")
	limit := 50
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT p.gsis_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       pr.proj_fpts_ppr
		FROM nfl_players p
		LEFT JOIN league_rosters lr ON lr.league_id = $1 AND lr.gsis_id = p.gsis_id
		LEFT JOIN nfl_projections pr ON pr.gsis_id = p.gsis_id AND pr.target_season = $2
		WHERE lr.gsis_id IS NULL
		  AND ($3 = '' OR p.position = $3)
		ORDER BY pr.proj_fpts_ppr DESC NULLS LAST, p.name
		LIMIT $4 OFFSET $5
	`, leagueID, season, position, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	agents := []freeAgentResp{}
	for rows.Next() {
		var a freeAgentResp
		if err := rows.Scan(&a.GsisID, &a.Name, &a.Position, &a.Team, &a.HeadshotURL, &a.ProjFptsPPR); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agents = append(agents, a)
	}
	respondJSON(w, http.StatusOK, agents)
}

// leagueSeasonInt resolves a league's season (stored as TEXT, matching
// Yahoo's shape) to an int, falling back to the configured default season
// when it doesn't parse — a native league's season is user-entered free text.
func (h *Handler) leagueSeasonInt(r *http.Request, leagueID int64) int {
	var season string
	if err := h.db.QueryRow(r.Context(), "SELECT season FROM leagues WHERE id = $1", leagueID).Scan(&season); err == nil {
		if v, err := strconv.Atoi(season); err == nil {
			return v
		}
	}
	return h.config.DefaultSeason
}
