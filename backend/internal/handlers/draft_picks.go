package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// draftPickResp is one tradable future draft pick, enriched with team names
// and (once used) the player it was spent on.
type draftPickResp struct {
	ID               int64   `json:"id"`
	Season           int     `json:"season"`
	Round            int     `json:"round"`
	OriginalTeamID   int64   `json:"original_team_id"`
	OriginalTeamName string  `json:"original_team_name"`
	CurrentTeamID    int64   `json:"current_team_id"`
	CurrentTeamName  string  `json:"current_team_name"`
	OverallPick      *int    `json:"overall_pick"`
	UsedOnGsisID     *string `json:"used_on_gsis_id"`
	UsedOnName       *string `json:"used_on_name"`
}

// ListLeagueDraftPicks returns a native league's tradable future picks,
// optionally filtered by season or current owner.
//
// GET /api/leagues/{id}/picks?season=&team_id=
func (h *Handler) ListLeagueDraftPicks(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	q := r.URL.Query()
	var season int
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	var teamID int64
	if t := q.Get("team_id"); t != "" {
		if v, err := strconv.ParseInt(t, 10, 64); err == nil {
			teamID = v
		}
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT dp.id, dp.season, dp.round,
		       dp.original_team_id, ot.name,
		       dp.current_team_id, ct.name,
		       dp.overall_pick, dp.used_on_gsis_id, up.name
		FROM league_draft_picks dp
		JOIN teams ot ON ot.id = dp.original_team_id
		JOIN teams ct ON ct.id = dp.current_team_id
		LEFT JOIN nfl_players up ON up.gsis_id = dp.used_on_gsis_id
		WHERE dp.league_id = $1
		  AND ($2 = 0 OR dp.season = $2)
		  AND ($3 = 0 OR dp.current_team_id = $3)
		ORDER BY dp.season, dp.round, ct.name
	`, leagueID, season, teamID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	picks := []draftPickResp{}
	for rows.Next() {
		var p draftPickResp
		if err := rows.Scan(
			&p.ID, &p.Season, &p.Round,
			&p.OriginalTeamID, &p.OriginalTeamName,
			&p.CurrentTeamID, &p.CurrentTeamName,
			&p.OverallPick, &p.UsedOnGsisID, &p.UsedOnName,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		picks = append(picks, p)
	}
	respondJSON(w, http.StatusOK, picks)
}

type generatePicksReq struct {
	Season int `json:"season"` // defaults to the league's current season
	Rounds int `json:"rounds"` // defaults to league_settings.draft_rounds
}

// GenerateLeagueDraftPicks creates one round-robin draft class (rounds ×
// teams) for a season. Idempotent: a class already generated for that season
// 409s rather than silently duplicating or partially inserting.
//
// POST /api/leagues/{id}/picks/generate
func (h *Handler) GenerateLeagueDraftPicks(w http.ResponseWriter, r *http.Request) {
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

	var req generatePicksReq
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — tolerate empty body, defaults apply
	if req.Season == 0 {
		req.Season = h.leagueSeasonInt(r, leagueID)
	}
	if req.Rounds == 0 {
		h.db.QueryRow(r.Context(), "SELECT draft_rounds FROM league_settings WHERE league_id = $1", leagueID).Scan(&req.Rounds) //nolint:errcheck
		if req.Rounds == 0 {
			req.Rounds = 4
		}
	}
	if req.Rounds < 1 || req.Rounds > 30 {
		respondError(w, http.StatusBadRequest, "rounds must be between 1 and 30")
		return
	}

	var exists bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_draft_picks WHERE league_id = $1 AND season = $2)", leagueID, req.Season,
	).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		respondError(w, http.StatusConflict, "draft picks for that season have already been generated")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	teamIDs, err := h.leagueTeamIDs(r.Context(), tx, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(teamIDs) == 0 {
		respondError(w, http.StatusUnprocessableEntity, "league has no teams")
		return
	}
	if err := generateDraftClassTx(r.Context(), tx, leagueID, req.Season, req.Rounds, teamIDs); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload, _ := json.Marshal(map[string]any{"action": "generate_picks", "season": req.Season, "rounds": req.Rounds, "teams": len(teamIDs)})
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'rollover',$3,$4)`,
		leagueID, req.Season, payload, user.ID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{
		"status": "generated", "season": req.Season, "rounds": req.Rounds, "picks": req.Rounds * len(teamIDs),
	})
}

type usePickReq struct {
	GsisID       string `json:"gsis_id"`
	Slot         string `json:"slot"`
	Salary       int    `json:"salary"`
	SignedSeason int    `json:"signed_season"`
	YearsTotal   *int   `json:"years_total"`
}

// UseLeagueDraftPick spends a pick on a player: assigns the roster+contract
// row (via assignRosterTx, the same insert AssignLeagueRoster uses) to the
// pick's current owner and stamps the pick used. acquired_via is always
// "draft" — that's this endpoint's entire purpose, so it isn't client-set.
//
// POST /api/leagues/{id}/picks/{pickId}/use
func (h *Handler) UseLeagueDraftPick(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	pickID, err := parseID(r, "pickId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid pick id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req usePickReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.GsisID == "" {
		respondError(w, http.StatusBadRequest, "gsis_id is required")
		return
	}
	if req.Slot == "" {
		req.Slot = "BN"
	}
	if !validSlots[req.Slot] {
		respondError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	if req.Salary < 0 {
		respondError(w, http.StatusBadRequest, "salary cannot be negative")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var currentTeamID int64
	var season int
	var used *string
	err = tx.QueryRow(r.Context(), `
		SELECT current_team_id, used_on_gsis_id, season
		FROM league_draft_picks WHERE id = $1 AND league_id = $2 FOR UPDATE
	`, pickID, leagueID).Scan(&currentTeamID, &used, &season)
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusNotFound, "pick not found in this league")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if used != nil {
		respondError(w, http.StatusConflict, "pick has already been used")
		return
	}

	signedSeason := req.SignedSeason
	if signedSeason == 0 {
		signedSeason = season
	}

	if err := assignRosterTx(r.Context(), tx, leagueID, currentTeamID, req.GsisID, req.Slot, "draft", req.Salary, signedSeason, req.YearsTotal); err != nil {
		respondError(w, http.StatusConflict, "player is already rostered in this league (or not a known player)")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE league_draft_picks SET used_on_gsis_id = $1 WHERE id = $2`, req.GsisID, pickID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload, _ := json.Marshal(map[string]any{"pick_id": pickID, "team_id": currentTeamID, "gsis_id": req.GsisID, "salary": req.Salary})
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'draft',$3,$4)`,
		leagueID, season, payload, user.ID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "used"})
}
