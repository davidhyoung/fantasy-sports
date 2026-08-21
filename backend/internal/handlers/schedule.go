package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/leaguesettings"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/nflstats"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// nativeProjectedPoints estimates one week's worth of projected fantasy
// points for each team's currently-starter-slotted players, using the same
// per-game-rate projection data and scoring pipeline relevantPlayerStats and
// ScoreLeagueWeek already use — just ProjectionToCanonicalTotals(rates, 1)
// (one game) scored through the league's own modifiers instead of actual
// stats. Used only to give an unscored scoreboard card something better than
// a blank dash; never written anywhere.
func (h *Handler) nativeProjectedPoints(ctx context.Context, leagueID int64, teamIDs []int64, season int) map[int64]float64 {
	out := map[int64]float64{}
	if len(teamIDs) == 0 {
		return out
	}
	mods, ok := leaguesettings.NewNativeSource(h.db, leagueID).ScoringMods(ctx)
	if !ok {
		return out
	}

	rows, err := h.db.Query(ctx, `
		SELECT team_id, gsis_id FROM league_rosters
		WHERE league_id = $1 AND team_id = ANY($2) AND slot NOT IN ('BN', 'TAXI', 'IR')
	`, leagueID, teamIDs)
	if err != nil {
		return out
	}
	teamPlayers := map[int64][]string{}
	var allGsis []string
	for rows.Next() {
		var teamID int64
		var gsisID string
		if err := rows.Scan(&teamID, &gsisID); err != nil {
			rows.Close()
			return out
		}
		teamPlayers[teamID] = append(teamPlayers[teamID], gsisID)
		allGsis = append(allGsis, gsisID)
	}
	rows.Close()
	if len(allGsis) == 0 {
		return out
	}

	prows, err := h.db.Query(ctx, `
		SELECT gsis_id,
		       proj_pass_yds_pg, proj_pass_td_pg, proj_rush_yds_pg, proj_rush_td_pg,
		       proj_rec_pg, proj_rec_yds_pg, proj_rec_td_pg, proj_fg_made_pg, proj_pat_made_pg
		FROM nfl_projections
		WHERE gsis_id = ANY($1) AND target_season = $2
	`, allGsis, season)
	if err != nil {
		return out
	}
	perPlayer := map[string]float64{}
	for prows.Next() {
		var gsisID string
		var rates scoring.ProjectionRates
		if err := prows.Scan(
			&gsisID,
			&rates.PassYdsPG, &rates.PassTdPG, &rates.RushYdsPG, &rates.RushTdPG,
			&rates.RecPG, &rates.RecYdsPG, &rates.RecTdPG, &rates.FgMadePG, &rates.PatMadePG,
		); err != nil {
			prows.Close()
			return out
		}
		totals := scoring.ProjectionToCanonicalTotals(rates, 1)
		perPlayer[gsisID] = scoring.ScoreWithModifiers(totals, mods)
	}
	prows.Close()

	for teamID, gsisIDs := range teamPlayers {
		var total float64
		for _, g := range gsisIDs {
			total += perPlayer[g]
		}
		out[teamID] = total
	}
	return out
}

// roundRobinRounds returns the standard "circle method" single round-robin:
// len(teamIDs)-1 rounds (one extra if odd, with a bye each round), each
// round pairing every team against a different opponent exactly once. A
// schedule longer than that just cycles back through these rounds.
func roundRobinRounds(teamIDs []int64) [][][2]int64 {
	arr := append([]int64{}, teamIDs...)
	if len(arr)%2 != 0 {
		arr = append(arr, 0) // 0 = bye
	}
	n := len(arr)
	rounds := make([][][2]int64, n-1)
	for round := 0; round < n-1; round++ {
		var pairs [][2]int64
		for i := 0; i < n/2; i++ {
			a, b := arr[i], arr[n-1-i]
			if a != 0 && b != 0 {
				pairs = append(pairs, [2]int64{a, b})
			}
		}
		rounds[round] = pairs
		// Fix arr[0], rotate everyone else by one position.
		last := arr[n-1]
		for i := n - 1; i > 1; i-- {
			arr[i] = arr[i-1]
		}
		arr[1] = last
	}
	return rounds
}

type generateScheduleReq struct {
	Season int `json:"season"` // defaults to the league's current season
	Weeks  int `json:"weeks"`  // defaults to league_settings.regular_season_weeks
}

// GenerateLeagueSchedule creates a regular-season round-robin schedule.
// Idempotent — 409s if a schedule already exists for that season, checked
// before any insert (same pattern as GenerateLeagueDraftPicks).
//
// POST /api/leagues/{id}/schedule/generate
func (h *Handler) GenerateLeagueSchedule(w http.ResponseWriter, r *http.Request) {
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

	var req generateScheduleReq
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — tolerate empty body, defaults apply
	if req.Season == 0 {
		req.Season = h.leagueSeasonInt(r, leagueID)
	}
	if req.Weeks == 0 {
		h.db.QueryRow(r.Context(), "SELECT regular_season_weeks FROM league_settings WHERE league_id = $1", leagueID).Scan(&req.Weeks) //nolint:errcheck
		if req.Weeks == 0 {
			req.Weeks = 14
		}
	}
	if req.Weeks < 1 || req.Weeks > 30 {
		respondError(w, http.StatusBadRequest, "weeks must be between 1 and 30")
		return
	}

	var exists bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_matchups WHERE league_id = $1 AND season = $2)", leagueID, req.Season,
	).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if exists {
		respondError(w, http.StatusConflict, "a schedule for that season already exists")
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
	if len(teamIDs) < 2 {
		respondError(w, http.StatusUnprocessableEntity, "league needs at least 2 teams")
		return
	}

	rounds := roundRobinRounds(teamIDs)
	matchupCount := 0
	for week := 1; week <= req.Weeks; week++ {
		for _, pair := range rounds[(week-1)%len(rounds)] {
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO league_matchups (league_id, season, week, team1_id, team2_id)
				VALUES ($1, $2, $3, $4, $5)
			`, leagueID, req.Season, week, pair[0], pair[1]); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			matchupCount++
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{
		"status": "generated", "season": req.Season, "weeks": req.Weeks, "matchups": matchupCount,
	})
}

// ScoreLeagueWeek computes and freezes each matchup's points for one week,
// from real (not projected) stats. Only players in a starter slot
// (not BN/TAXI/IR) count. An explicit, re-runnable action — not automatic —
// since the underlying stats arrive via a manual batch import, never live.
//
// POST /api/leagues/{id}/scoreboard/score?week=N&season=
func (h *Handler) ScoreLeagueWeek(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()
	week, err := strconv.Atoi(q.Get("week"))
	if err != nil || week <= 0 {
		respondError(w, http.StatusBadRequest, "week is required")
		return
	}
	season := h.leagueSeasonInt(r, leagueID)
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	mods, ok := leaguesettings.NewNativeSource(h.db, leagueID).ScoringMods(r.Context())
	if !ok {
		respondError(w, http.StatusUnprocessableEntity, "league has no scoring configured")
		return
	}

	type matchup struct{ id, team1, team2 int64 }
	rows, err := h.db.Query(r.Context(),
		"SELECT id, team1_id, team2_id FROM league_matchups WHERE league_id = $1 AND season = $2 AND week = $3",
		leagueID, season, week,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var matchups []matchup
	teamSet := map[int64]bool{}
	for rows.Next() {
		var m matchup
		if err := rows.Scan(&m.id, &m.team1, &m.team2); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		matchups = append(matchups, m)
		teamSet[m.team1] = true
		teamSet[m.team2] = true
	}
	rows.Close()
	if len(matchups) == 0 {
		respondError(w, http.StatusNotFound, "no matchups scheduled for that week")
		return
	}

	teamIDs := make([]int64, 0, len(teamSet))
	for id := range teamSet {
		teamIDs = append(teamIDs, id)
	}

	rrows, err := h.db.Query(r.Context(), `
		SELECT team_id, gsis_id FROM league_rosters
		WHERE league_id = $1 AND team_id = ANY($2) AND slot NOT IN ('BN', 'TAXI', 'IR')
	`, leagueID, teamIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	teamPlayers := map[int64][]string{}
	var allGsis []string
	for rrows.Next() {
		var teamID int64
		var gsisID string
		if err := rrows.Scan(&teamID, &gsisID); err != nil {
			rrows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		teamPlayers[teamID] = append(teamPlayers[teamID], gsisID)
		allGsis = append(allGsis, gsisID)
	}
	rrows.Close()

	weekStats, err := nflstats.LoadWeekStats(r.Context(), h.db, season, week, allGsis)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	teamPoints := map[int64]float64{}
	for teamID, gsisIDs := range teamPlayers {
		var total float64
		for _, g := range gsisIDs {
			if stats, ok := weekStats[g]; ok {
				total += scoring.ScoreWithModifiers(stats, mods)
			}
		}
		teamPoints[teamID] = total
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	for _, m := range matchups {
		if _, err := tx.Exec(r.Context(), `
			UPDATE league_matchups SET team1_points = $1, team2_points = $2, computed_at = NOW()
			WHERE id = $3
		`, teamPoints[m.team1], teamPoints[m.team2], m.id); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "scored", "week": week, "season": season, "matchups": len(matchups)})
}
