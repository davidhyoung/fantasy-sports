package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

// nbaCategories is the fixed roto category vocabulary, in display order.
// Unlike NFL's scoring.CanonicalStat (per-stat point weights), a native NBA
// league's league_settings.scoring JSONB holds a flat {category: 1}
// inclusion map — which of these categories count, not how many points
// each is worth (see migrations/000035, .claude/plans). fg_pct/ft_pct are
// always computed from aggregated makes/attempts, never averaged per-game.
var nbaCategories = []string{
	"points", "rebounds", "assists", "steals", "blocks", "turnovers", "three_pm", "fg_pct", "ft_pct",
}

// nbaSlotEligibility lists which real NBA positions may fill each dedicated
// or flex slot. A slot with no entry (BN, IR, UTIL) accepts any position.
// Mirrors league_rosters.go's slotEligiblePositions pattern. ESPN's own NBA
// position taxonomy only has G/F/C — no PG/SG or SF/PF split exists to key a
// narrower slot off of (verified against real roster data).
var nbaSlotEligibility = map[string][]string{
	"G": {"G"},
	"F": {"F"},
	"C": {"C"},
}

func nbaSlotEligibleForPosition(slot, position string) bool {
	eligible, ok := nbaSlotEligibility[slot]
	if !ok {
		return true
	}
	for _, p := range eligible {
		if p == position {
			return true
		}
	}
	return false
}

var validNBASlots = map[string]bool{
	"G": true, "F": true, "C": true, "UTIL": true, "BN": true, "IR": true,
}

var validNBAAcquiredVia = map[string]bool{
	"draft": true, "trade": true, "waiver": true, "fa": true,
}

type nbaCategoryEntry struct {
	Category string  `json:"category"`
	Value    float64 `json:"value"`
}

type nbaRosterEntryResp struct {
	ESPNID      string             `json:"espn_id"`
	Name        string             `json:"name"`
	Position    string             `json:"position"`
	Team        string             `json:"team"`
	HeadshotURL string             `json:"headshot_url,omitempty"`
	TeamID      int64              `json:"team_id"`
	Slot        string             `json:"slot"`
	AcquiredVia string             `json:"acquired_via"`
	AcquiredAt  time.Time          `json:"acquired_at"`
	Stats       []nbaCategoryEntry `json:"stats,omitempty"`
}

type nbaFreeAgentResp struct {
	ESPNID      string             `json:"espn_id"`
	Name        string             `json:"name"`
	Position    string             `json:"position"`
	Team        string             `json:"team"`
	HeadshotURL string             `json:"headshot_url,omitempty"`
	Stats       []nbaCategoryEntry `json:"stats,omitempty"`
}

func nbaRatio(made, att float64) float64 {
	if att == 0 {
		return 0
	}
	return made / att
}

// validNBAStatPeriods is the Players/Roster tab timeframe picker's
// vocabulary. "season"/"season_avg" share the current-season total query,
// differing only in whether counting categories get divided by games
// played; "last_season"/"last_season_avg" are the same pair one season back
// (season-1); "last4weeks"/"lastweek" stay within the current season but add
// a trailing game_date cutoff. fg_pct/ft_pct are always a ratio of
// aggregated makes/attempts, never averaged, so "_avg" has no effect on
// them.
var validNBAStatPeriods = map[string]bool{
	"season": true, "season_avg": true, "last_season": true, "last_season_avg": true,
	"last4weeks": true, "lastweek": true,
}

func parseNBAStatPeriod(r *http.Request) string {
	p := r.URL.Query().Get("period")
	if validNBAStatPeriods[p] {
		return p
	}
	return "season"
}

// nbaRawCategoryTotals returns every canonical category's aggregated total
// for the given players over the requested period, unfiltered by any
// league's scoring settings — the shared core both nbaCategoryStats
// (league-scoped) and the standalone player-detail endpoint build on.
func (h *Handler) nbaRawCategoryTotals(ctx context.Context, season int, period string, espnIDs []string) (map[string]map[string]float64, error) {
	out := map[string]map[string]float64{}
	if len(espnIDs) == 0 {
		return out, nil
	}

	statSeason := season
	if period == "last_season" || period == "last_season_avg" {
		statSeason = season - 1
	}

	query := `
		SELECT espn_id,
		       COALESCE(SUM(points),0), COALESCE(SUM(rebounds),0), COALESCE(SUM(assists),0),
		       COALESCE(SUM(steals),0), COALESCE(SUM(blocks),0), COALESCE(SUM(turnovers),0),
		       COALESCE(SUM(fg3_made),0),
		       COALESCE(SUM(fg_made),0), COALESCE(SUM(fg_att),0),
		       COALESCE(SUM(ft_made),0), COALESCE(SUM(ft_att),0),
		       COUNT(*)
		FROM nba_player_stats
		WHERE espn_id = ANY($1) AND season = $2`
	args := []any{espnIDs, statSeason}
	switch period {
	case "last4weeks":
		query += " AND game_date >= $3"
		args = append(args, time.Now().AddDate(0, 0, -28))
	case "lastweek":
		query += " AND game_date >= $3"
		args = append(args, time.Now().AddDate(0, 0, -7))
	}
	query += " GROUP BY espn_id"

	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var espnID string
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		var games int
		if err := rows.Scan(&espnID, &pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta, &games); err != nil {
			return nil, err
		}
		divisor := 1.0
		if (period == "season_avg" || period == "last_season_avg") && games > 0 {
			divisor = float64(games)
		}
		out[espnID] = map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor, "steals": stl / divisor, "blocks": blk / divisor,
			"turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
	}
	return out, rows.Err()
}

// nbaCategoryStats returns aggregated category totals for the given players
// over the requested period, restricted to whichever categories this
// league's scoring actually includes (nonzero in league_settings.scoring) —
// same "relevant" filtering convention relevantPlayerStats established for
// NFL.
func (h *Handler) nbaCategoryStats(ctx context.Context, leagueID int64, season int, period string, espnIDs []string) (map[string][]nbaCategoryEntry, error) {
	out := map[string][]nbaCategoryEntry{}
	if len(espnIDs) == 0 {
		return out, nil
	}

	var scoringRaw []byte
	if err := h.db.QueryRow(ctx,
		"SELECT scoring FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&scoringRaw); err != nil {
		return out, nil
	}
	var included map[string]float64
	if err := json.Unmarshal(scoringRaw, &included); err != nil {
		return out, nil
	}
	var relevant []string
	for _, c := range nbaCategories {
		if included[c] != 0 {
			relevant = append(relevant, c)
		}
	}
	if len(relevant) == 0 {
		return out, nil
	}

	totalsByPlayer, err := h.nbaRawCategoryTotals(ctx, season, period, espnIDs)
	if err != nil {
		return nil, err
	}
	for espnID, totals := range totalsByPlayer {
		entries := make([]nbaCategoryEntry, 0, len(relevant))
		for _, c := range relevant {
			entries = append(entries, nbaCategoryEntry{Category: c, Value: totals[c]})
		}
		out[espnID] = entries
	}
	return out, nil
}

// GetLeagueRostersNBA returns every rostered player in a native NBA league.
//
// GET /api/leagues/{id}/rosters-nba?period=
func (h *Handler) GetLeagueRostersNBA(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.leagueSeasonInt(r, leagueID)
	period := parseNBAStatPeriod(r)

	rows, err := h.db.Query(r.Context(), `
		SELECT lr.espn_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       lr.team_id, lr.slot, lr.acquired_via, lr.acquired_at
		FROM league_rosters_nba lr
		JOIN nba_players p ON p.espn_id = lr.espn_id
		WHERE lr.league_id = $1
		ORDER BY lr.team_id, lr.slot, p.name
	`, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	roster := []nbaRosterEntryResp{}
	espnIDs := make([]string, 0)
	for rows.Next() {
		var e nbaRosterEntryResp
		if err := rows.Scan(&e.ESPNID, &e.Name, &e.Position, &e.Team, &e.HeadshotURL,
			&e.TeamID, &e.Slot, &e.AcquiredVia, &e.AcquiredAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		roster = append(roster, e)
		espnIDs = append(espnIDs, e.ESPNID)
	}
	rows.Close()

	statsByPlayer, err := h.nbaCategoryStats(r.Context(), leagueID, season, period, espnIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range roster {
		roster[i].Stats = statsByPlayer[roster[i].ESPNID]
	}

	respondJSON(w, http.StatusOK, roster)
}

// GetLeagueFreeAgentsNBA returns players in a native NBA league with no
// roster row — nba_players minus league_rosters_nba.
//
// GET /api/leagues/{id}/free-agents-nba?position=&limit=&offset=&search=&period=
func (h *Handler) GetLeagueFreeAgentsNBA(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.leagueSeasonInt(r, leagueID)
	period := parseNBAStatPeriod(r)

	q := r.URL.Query()
	position := q.Get("position")
	search := q.Get("search")
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
		SELECT p.espn_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, '')
		FROM nba_players p
		LEFT JOIN league_rosters_nba lr ON lr.league_id = $1 AND lr.espn_id = p.espn_id
		WHERE lr.espn_id IS NULL
		  -- Excludes historical stat-stub rows (cmd/import-nba inserts a
		  -- position-less stub for any box-score player not on a currently
		  -- tracked roster) — a free agent should be someone actually
		  -- eligible to roster today, not a decades-old player ESPN's
		  -- scoreboard happened to mention.
		  AND p.position IS NOT NULL AND p.position != ''
		  AND ($2 = '' OR p.position = $2)
		  AND ($3 = '' OR p.name ILIKE '%' || $3 || '%')
		ORDER BY p.name
		LIMIT $4 OFFSET $5
	`, leagueID, position, search, limit, offset)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	agents := []nbaFreeAgentResp{}
	espnIDs := make([]string, 0)
	for rows.Next() {
		var a nbaFreeAgentResp
		if err := rows.Scan(&a.ESPNID, &a.Name, &a.Position, &a.Team, &a.HeadshotURL); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agents = append(agents, a)
		espnIDs = append(espnIDs, a.ESPNID)
	}
	rows.Close()

	statsByPlayer, err := h.nbaCategoryStats(r.Context(), leagueID, season, period, espnIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range agents {
		agents[i].Stats = statsByPlayer[agents[i].ESPNID]
	}

	respondJSON(w, http.StatusOK, agents)
}

type assignRosterNBAReq struct {
	ESPNID      string `json:"espn_id"`
	TeamID      int64  `json:"team_id"`
	Slot        string `json:"slot"`
	AcquiredVia string `json:"acquired_via"`
}

// AssignLeagueRosterNBA puts a player on a native NBA league's roster — no
// contract/cap step (deferred, see .claude/plans): a redraft-style
// assign/drop roster is enough for category scoring to mean something.
//
// POST /api/leagues/{id}/rosters-nba
func (h *Handler) AssignLeagueRosterNBA(w http.ResponseWriter, r *http.Request) {
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

	var req assignRosterNBAReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.ESPNID == "" || req.TeamID == 0 {
		respondError(w, http.StatusBadRequest, "espn_id and team_id are required")
		return
	}
	if req.Slot == "" {
		req.Slot = "BN"
	}
	if !validNBASlots[req.Slot] {
		respondError(w, http.StatusBadRequest, "invalid slot")
		return
	}
	if req.AcquiredVia == "" || !validNBAAcquiredVia[req.AcquiredVia] {
		respondError(w, http.StatusBadRequest, "invalid acquired_via")
		return
	}

	var playerPosition string
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(position, '') FROM nba_players WHERE espn_id = $1", req.ESPNID,
	).Scan(&playerPosition); err != nil {
		respondError(w, http.StatusBadRequest, "unknown player")
		return
	}
	if !nbaSlotEligibleForPosition(req.Slot, playerPosition) {
		respondError(w, http.StatusBadRequest, playerPosition+" is not eligible for slot "+req.Slot)
		return
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

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO league_rosters_nba (league_id, team_id, espn_id, slot, acquired_via)
		VALUES ($1, $2, $3, $4, $5)
	`, leagueID, req.TeamID, req.ESPNID, req.Slot, req.AcquiredVia); err != nil {
		respondError(w, http.StatusConflict, "player is already rostered in this league")
		return
	}
	respondJSON(w, http.StatusCreated, map[string]string{"status": "rostered"})
}

type updateRosterNBAReq struct {
	TeamID *int64  `json:"team_id,omitempty"`
	Slot   *string `json:"slot,omitempty"`
}

// UpdateLeagueRosterNBA moves a rostered player — to another team (a trade)
// and/or a different slot. Only fields present in the body are changed.
//
// PUT /api/leagues/{id}/rosters-nba/{espnId}
func (h *Handler) UpdateLeagueRosterNBA(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	espnID := chi.URLParam(r, "espnId")
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req updateRosterNBAReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Slot != nil {
		if !validNBASlots[*req.Slot] {
			respondError(w, http.StatusBadRequest, "invalid slot")
			return
		}
		var playerPosition string
		if err := h.db.QueryRow(r.Context(),
			"SELECT COALESCE(position, '') FROM nba_players WHERE espn_id = $1", espnID,
		).Scan(&playerPosition); err != nil {
			respondError(w, http.StatusBadRequest, "unknown player")
			return
		}
		if !nbaSlotEligibleForPosition(*req.Slot, playerPosition) {
			respondError(w, http.StatusBadRequest, playerPosition+" is not eligible for slot "+*req.Slot)
			return
		}
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

	res, err := h.db.Exec(r.Context(), `
		UPDATE league_rosters_nba
		SET team_id = COALESCE($3, team_id), slot = COALESCE($4, slot)
		WHERE league_id = $1 AND espn_id = $2
	`, leagueID, espnID, req.TeamID, req.Slot)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "player is not rostered in this league")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DropLeagueRosterNBA releases a player to free agency.
//
// DELETE /api/leagues/{id}/rosters-nba/{espnId}
func (h *Handler) DropLeagueRosterNBA(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	espnID := chi.URLParam(r, "espnId")
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	res, err := h.db.Exec(r.Context(),
		"DELETE FROM league_rosters_nba WHERE league_id = $1 AND espn_id = $2", leagueID, espnID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "player is not rostered in this league")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
}
