package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
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

	// Draft order comes from how teams finished the season before the one
	// being drafted for — the standard "next year's order is set by this
	// year's standings" rule. A season with no scored weeks yet (including a
	// league's first-ever season) falls back to leagueTeamIDs' deterministic
	// id-ascending order inside reverseStandingsOrder itself.
	teamIDs, err := h.reverseStandingsOrder(r.Context(), tx, leagueID, req.Season-1)
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
	GsisID string `json:"gsis_id"`
	Slot   string `json:"slot"`
}

// UseLeagueDraftPick spends a pick on a player: assigns the roster+contract
// row (via assignRosterTx, the same insert AssignLeagueRoster uses) to the
// pick's current owner and stamps the pick used. acquired_via is always
// "draft" — that's this endpoint's entire purpose, so it isn't client-set.
// Salary and contract length are derived entirely from the pick's own slot
// (overall_pick + round) via the rookie scale — not accepted from the
// caller — since a fixed, knowable-in-advance price is the whole point of
// trading picks before it's known who gets drafted with them. See
// rookie_scale.go / .claude/plans/dynasty-transactions.md.
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

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var currentTeamID int64
	var season, round int
	var overallPick *int
	var used *string
	err = tx.QueryRow(r.Context(), `
		SELECT current_team_id, used_on_gsis_id, season, round, overall_pick
		FROM league_draft_picks WHERE id = $1 AND league_id = $2 FOR UPDATE
	`, pickID, leagueID).Scan(&currentTeamID, &used, &season, &round, &overallPick)
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
	if overallPick == nil {
		respondError(w, http.StatusUnprocessableEntity, "this pick has no draft position assigned — regenerate the draft class")
		return
	}
	// A future pick is tradable in advance (the entire point of pre-generating
	// a class ahead of rollover), but spending it early would sign a contract
	// for a season the league hasn't reached yet — every other assumption in
	// this system (cap-year alignment, dead-money scheduling) depends on a
	// contract's signed_season being the season actually being played.
	if currentSeason := h.leagueSeasonInt(r, leagueID); season != currentSeason {
		respondError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("this pick is for season %d, not the league's current season %d — it can't be used until then", season, currentSeason))
		return
	}

	var totalPicks int
	if err := tx.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM league_draft_picks WHERE league_id = $1 AND season = $2", leagueID, season,
	).Scan(&totalPicks); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var rookieScaleRaw []byte
	if err := tx.QueryRow(r.Context(),
		"SELECT rookie_scale FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&rookieScaleRaw); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var rookieScale models.RookieScale
	if err := json.Unmarshal(rookieScaleRaw, &rookieScale); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	salary, err := h.rookieScaleSalary(r.Context(), leagueID, season, *overallPick, totalPicks, rookieScale)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	yearsTotal := rookieScaleYears(rookieScale, round)

	if msg, err := capCheckAdd(r.Context(), tx, leagueID, currentTeamID, season, req.Slot, salary); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	} else if msg != "" {
		respondError(w, http.StatusUnprocessableEntity, msg)
		return
	}

	if err := assignRosterTx(r.Context(), tx, leagueID, currentTeamID, req.GsisID, req.Slot, "draft", salary, season, &yearsTotal); err != nil {
		respondError(w, http.StatusConflict, "player is already rostered in this league (or not a known player)")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE league_draft_picks SET used_on_gsis_id = $1 WHERE id = $2`, req.GsisID, pickID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"pick_id": pickID, "team_id": currentTeamID, "gsis_id": req.GsisID,
		"overall_pick": *overallPick, "salary": salary, "years_total": yearsTotal,
	})
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
