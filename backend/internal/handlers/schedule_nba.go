package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

type generateScheduleNBAReq struct {
	Season          int    `json:"season"`
	Weeks           int    `json:"weeks"`
	SeasonStartDate string `json:"season_start_date"` // YYYY-MM-DD — week 1 starts here
}

// GenerateLeagueScheduleNBA creates a regular-season round-robin schedule for
// a native NBA league — same roundRobinRounds logic GenerateLeagueSchedule
// uses for NFL (confirmed sport-generic), writing into league_matchups_nba
// instead. Unlike NFL (where week numbers already map onto real NFL weeks),
// NBA has no such mapping, so this also stamps league_settings.season_start_date
// — week N's games are that date + 7*(N-1) through +7*N-1 (see ScoreLeagueWeekNBA).
//
// POST /api/leagues/{id}/schedule-nba/generate
func (h *Handler) GenerateLeagueScheduleNBA(w http.ResponseWriter, r *http.Request) {
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

	var req generateScheduleNBAReq
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
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
	startDate, err := time.Parse("2006-01-02", req.SeasonStartDate)
	if err != nil {
		respondError(w, http.StatusBadRequest, "season_start_date (YYYY-MM-DD) is required")
		return
	}

	var exists bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_matchups_nba WHERE league_id = $1 AND season = $2)", leagueID, req.Season,
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

	if _, err := tx.Exec(r.Context(),
		"UPDATE league_settings SET season_start_date = $2 WHERE league_id = $1", leagueID, startDate,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rounds := roundRobinRounds(teamIDs)
	matchupCount := 0
	for week := 1; week <= req.Weeks; week++ {
		for _, pair := range rounds[(week-1)%len(rounds)] {
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO league_matchups_nba (league_id, season, week, team1_id, team2_id)
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

// nbaWeekRange returns the [start, end] calendar dates (inclusive) for a
// league's fantasy week N, given its season_start_date.
func nbaWeekRange(seasonStartDate time.Time, week int) (start, end time.Time) {
	start = seasonStartDate.AddDate(0, 0, 7*(week-1))
	end = start.AddDate(0, 0, 6)
	return start, end
}

type nbaTeamCategoryTotals map[string]float64 // category -> value

// nbaTeamCategoryStats sums a team's starters' real nba_player_stats rows
// within [start, end] into the 9 roto categories — fg_pct/ft_pct computed
// from the team's aggregated makes/attempts across every starter and game in
// the window, never averaged per-player or per-game.
func (h *Handler) nbaTeamCategoryStats(r *http.Request, leagueID, teamID int64, season int, start, end time.Time) (nbaTeamCategoryTotals, error) {
	rows, err := h.db.Query(r.Context(), `
		SELECT COALESCE(SUM(s.points),0), COALESCE(SUM(s.rebounds),0), COALESCE(SUM(s.assists),0),
		       COALESCE(SUM(s.steals),0), COALESCE(SUM(s.blocks),0), COALESCE(SUM(s.turnovers),0),
		       COALESCE(SUM(s.fg3_made),0),
		       COALESCE(SUM(s.fg_made),0), COALESCE(SUM(s.fg_att),0),
		       COALESCE(SUM(s.ft_made),0), COALESCE(SUM(s.ft_att),0)
		FROM league_rosters_nba lr
		JOIN nba_player_stats s ON s.espn_id = lr.espn_id
		WHERE lr.league_id = $1 AND lr.team_id = $2 AND lr.season = $3 AND lr.slot NOT IN ('BN', 'IR')
		  AND s.season = $3 AND s.game_date BETWEEN $4 AND $5
	`, leagueID, teamID, season, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	totals := nbaTeamCategoryTotals{}
	if rows.Next() {
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		totals["points"] = pts
		totals["rebounds"] = reb
		totals["assists"] = ast
		totals["steals"] = stl
		totals["blocks"] = blk
		totals["turnovers"] = tov
		totals["three_pm"] = fg3m
		totals["fg_pct"] = nbaRatio(fgm, fga)
		totals["ft_pct"] = nbaRatio(ftm, fta)
	}
	return totals, rows.Err()
}

// relevantNBACategories returns this league's included roto categories
// (nonzero in league_settings.scoring), in nbaCategories display order.
func (h *Handler) relevantNBACategories(r *http.Request, leagueID int64) ([]string, error) {
	var scoringRaw []byte
	if err := h.db.QueryRow(r.Context(),
		"SELECT scoring FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&scoringRaw); err != nil {
		return nil, err
	}
	var included map[string]float64
	if err := json.Unmarshal(scoringRaw, &included); err != nil {
		return nil, err
	}
	var relevant []string
	for _, c := range nbaCategories {
		if included[c] != 0 {
			relevant = append(relevant, c)
		}
	}
	return relevant, nil
}

// ScoreLeagueWeekNBA computes and freezes each matchup's per-category roto
// results for one week, from real stats. Only starter-slotted players count.
// An explicit, re-runnable commissioner action (same philosophy as
// ScoreLeagueWeek) — never automatic.
//
// For a turnovers-style "lower is better" category, the lower total wins;
// every other category is higher-is-better.
//
// POST /api/leagues/{id}/scoreboard-nba/score?week=N&season=
func (h *Handler) ScoreLeagueWeekNBA(w http.ResponseWriter, r *http.Request) {
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

	var seasonStartDate *time.Time
	if err := h.db.QueryRow(r.Context(),
		"SELECT season_start_date FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&seasonStartDate); err != nil || seasonStartDate == nil {
		respondError(w, http.StatusUnprocessableEntity, "no schedule generated for this league yet")
		return
	}
	start, end := nbaWeekRange(*seasonStartDate, week)

	relevant, err := h.relevantNBACategories(r, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(relevant) == 0 {
		respondError(w, http.StatusUnprocessableEntity, "league has no scoring categories configured")
		return
	}

	type matchup struct{ id, team1, team2 int64 }
	rows, err := h.db.Query(r.Context(),
		"SELECT id, team1_id, team2_id FROM league_matchups_nba WHERE league_id = $1 AND season = $2 AND week = $3",
		leagueID, season, week,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var matchups []matchup
	for rows.Next() {
		var m matchup
		if err := rows.Scan(&m.id, &m.team1, &m.team2); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		matchups = append(matchups, m)
	}
	rows.Close()
	if len(matchups) == 0 {
		respondError(w, http.StatusNotFound, "no matchups scheduled for that week")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	for _, m := range matchups {
		t1, err := h.nbaTeamCategoryStats(r, leagueID, m.team1, season, start, end)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t2, err := h.nbaTeamCategoryStats(r, leagueID, m.team2, season, start, end)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		for _, cat := range relevant {
			v1, v2 := t1[cat], t2[cat]
			var t1Won, t2Won bool
			switch {
			case cat == "turnovers":
				t1Won, t2Won = v1 < v2, v2 < v1
			default:
				t1Won, t2Won = v1 > v2, v2 > v1
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO league_matchup_nba_categories (matchup_id, team_id, category, value, won)
				VALUES ($1,$2,$3,$4,$5), ($1,$6,$3,$7,$8)
				ON CONFLICT (matchup_id, team_id, category) DO UPDATE SET value = EXCLUDED.value, won = EXCLUDED.won
			`, m.id, m.team1, cat, v1, t1Won, m.team2, v2, t2Won); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}

		if _, err := tx.Exec(r.Context(),
			"UPDATE league_matchups_nba SET computed_at = NOW() WHERE id = $1", m.id,
		); err != nil {
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
