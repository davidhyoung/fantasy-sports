package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/leaguesettings"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/nflstats"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// rosterEntryResp is one player on a native league's roster, enriched with
// player metadata and contract terms.
type rosterEntryResp struct {
	GsisID       string            `json:"gsis_id"`
	Name         string            `json:"name"`
	Position     string            `json:"position"`
	Team         string            `json:"team"`
	HeadshotURL  string            `json:"headshot_url,omitempty"`
	TeamID       int64             `json:"team_id"`
	Slot         string            `json:"slot"`
	AcquiredVia  string            `json:"acquired_via"`
	AcquiredAt   time.Time         `json:"acquired_at"`
	Salary       int               `json:"salary"`
	SignedSeason int               `json:"signed_season"`
	YearsTotal   *int              `json:"years_total"`
	YearsUsed    int               `json:"years_used"`
	Stats        []playerStatEntry `json:"stats,omitempty"`
	// Chronological (oldest first) real fantasy points for the trailing up
	// to 4 weeks with imported stats — see weeklyTrend.
	Trend []float64 `json:"trend,omitempty"`
	// ProjFptsPPR is the same nfl_projections figure GetLeagueFreeAgents
	// already surfaces — nil when no projection exists for this player/season.
	// Only consumer today is the mobile Roster card face (the desktop table
	// shows the league's own per-category columns instead).
	ProjFptsPPR *float64          `json:"proj_fpts_ppr"`
	Notes       []situationalNote `json:"notes,omitempty"`
}

// playerStatEntry is one projected season-total stat category, restricted to
// whatever this league's own scoring actually weights — "relevant" meaning
// it impacts this league's scoring, not just "exists."
type playerStatEntry struct {
	Stat  string  `json:"stat"`
	Value float64 `json:"value"`
}

// relevantPlayerStats returns season-total projected stats for the given
// players, one entry per stat category the league's scoring weights
// nonzero, in leaguesettings.ScoringEditableStats order (so every player's
// array lines up under the same column headers). Rates come straight from
// nfl_projections; ProjectionToCanonicalTotals (the same helper draft-values
// and rollover scoring use) turns per-game rates into season totals.
func (h *Handler) relevantPlayerStats(ctx context.Context, leagueID int64, season int, gsisIDs []string) (map[string][]playerStatEntry, error) {
	out := map[string][]playerStatEntry{}
	if len(gsisIDs) == 0 {
		return out, nil
	}

	mods, ok := leaguesettings.NewNativeSource(h.db, leagueID).ScoringMods(ctx)
	if !ok {
		return out, nil
	}
	var relevant []scoring.CanonicalStat
	for _, s := range leaguesettings.ScoringEditableStats {
		if mods[s] != 0 {
			relevant = append(relevant, s)
		}
	}
	if len(relevant) == 0 {
		return out, nil
	}

	rows, err := h.db.Query(ctx, `
		SELECT gsis_id, proj_games,
		       proj_pass_yds_pg, proj_pass_td_pg, proj_rush_yds_pg, proj_rush_td_pg,
		       proj_rec_pg, proj_rec_yds_pg, proj_rec_td_pg, proj_fg_made_pg, proj_pat_made_pg
		FROM nfl_projections
		WHERE gsis_id = ANY($1) AND target_season = $2
	`, gsisIDs, season)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var gsisID string
		var games int
		var rates scoring.ProjectionRates
		if err := rows.Scan(
			&gsisID, &games,
			&rates.PassYdsPG, &rates.PassTdPG, &rates.RushYdsPG, &rates.RushTdPG,
			&rates.RecPG, &rates.RecYdsPG, &rates.RecTdPG, &rates.FgMadePG, &rates.PatMadePG,
		); err != nil {
			return nil, err
		}
		totals := scoring.ProjectionToCanonicalTotals(rates, float64(games))
		entries := make([]playerStatEntry, 0, len(relevant))
		for _, s := range relevant {
			entries = append(entries, playerStatEntry{Stat: string(s), Value: totals[s]})
		}
		out[gsisID] = entries
	}
	return out, rows.Err()
}

// weeklyTrend returns each player's real fantasy points (this league's own
// scoring modifiers) for the trailing up to 4 weeks that have any imported
// stats for the season — chronological, oldest first, for the Players tab's
// "L4 wks" sparkline. Independent of whether this league has actually run
// ScoreLeagueWeek for those weeks — that's a separate, explicit commissioner
// action; this reads straight from nfl_player_stats like relevantPlayerStats
// reads from nfl_projections.
func (h *Handler) weeklyTrend(ctx context.Context, leagueID int64, season int, gsisIDs []string) (map[string][]float64, error) {
	out := map[string][]float64{}
	if len(gsisIDs) == 0 {
		return out, nil
	}
	mods, ok := leaguesettings.NewNativeSource(h.db, leagueID).ScoringMods(ctx)
	if !ok {
		return out, nil
	}

	rows, err := h.db.Query(ctx, `
		SELECT DISTINCT week FROM nfl_player_stats
		WHERE season = $1 AND season_type = 'REG'
		ORDER BY week DESC LIMIT 4
	`, season)
	if err != nil {
		return nil, err
	}
	var weeks []int
	for rows.Next() {
		var wk int
		if err := rows.Scan(&wk); err != nil {
			rows.Close()
			return nil, err
		}
		weeks = append(weeks, wk)
	}
	rows.Close()
	sort.Ints(weeks) // oldest first, so the trend reads left-to-right

	for _, wk := range weeks {
		stats, err := nflstats.LoadWeekStats(ctx, h.db, season, wk, gsisIDs)
		if err != nil {
			return nil, err
		}
		for _, g := range gsisIDs {
			pts := 0.0
			if s, ok := stats[g]; ok {
				pts = scoring.ScoreWithModifiers(s, mods)
			}
			out[g] = append(out[g], pts)
		}
	}
	return out, nil
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

	season := h.leagueSeasonInt(r, leagueID)
	rows, err := h.db.Query(r.Context(), `
		SELECT
			lr.gsis_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
			lr.team_id, lr.slot, lr.acquired_via, lr.acquired_at,
			COALESCE(lc.salary, 0), COALESCE(lc.signed_season, 0), lc.years_total, COALESCE(lc.years_used, 1),
			pr.proj_fpts_ppr
		FROM league_rosters lr
		JOIN nfl_players p ON p.gsis_id = lr.gsis_id
		LEFT JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
		LEFT JOIN nfl_projections pr ON pr.gsis_id = lr.gsis_id AND pr.target_season = $2
		WHERE lr.league_id = $1
		ORDER BY lr.team_id, lr.slot, p.name
	`, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	roster := []rosterEntryResp{}
	gsisIDs := make([]string, 0)
	for rows.Next() {
		var e rosterEntryResp
		if err := rows.Scan(
			&e.GsisID, &e.Name, &e.Position, &e.Team, &e.HeadshotURL,
			&e.TeamID, &e.Slot, &e.AcquiredVia, &e.AcquiredAt,
			&e.Salary, &e.SignedSeason, &e.YearsTotal, &e.YearsUsed,
			&e.ProjFptsPPR,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		roster = append(roster, e)
		gsisIDs = append(gsisIDs, e.GsisID)
	}
	rows.Close()

	statsByPlayer, err := h.relevantPlayerStats(r.Context(), leagueID, season, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	trendByPlayer, err := h.weeklyTrend(r.Context(), leagueID, season, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	notesByPlayer, err := h.loadNotesForPlayers(r.Context(), h.config.DefaultSeason, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range roster {
		roster[i].Stats = statsByPlayer[roster[i].GsisID]
		roster[i].Trend = trendByPlayer[roster[i].GsisID]
		roster[i].Notes = notesByPlayer[roster[i].GsisID]
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

// DEF is deliberately absent: nflverse has no team-defense rows (no gsis_id
// to hang a league_rosters row off), so a DEF assignment would always fail
// at the DB layer. Kickers are unaffected — real gsis_ids exist for them.
var validSlots = map[string]bool{
	"QB": true, "RB": true, "WR": true, "TE": true, "K": true,
	"FLEX": true, "SFLEX": true, "BN": true, "TAXI": true, "IR": true,
}

var validAcquiredVia = map[string]bool{
	"draft": true, "auction": true, "trade": true, "waiver": true, "fa": true, "keeper": true,
}

// slotEligiblePositions lists which real NFL positions may fill each
// dedicated or flex slot. A slot with no entry here (BN, TAXI, IR, SFLEX)
// accepts any position. BN/TAXI/IR don't count as a starter for weekly
// scoring (ScoreLeagueWeek's `slot NOT IN ('BN','TAXI','IR')` filter), so
// there's no lineup-legality question to enforce for them. SFLEX (superflex)
// is open to every position by definition — QB is simply the position that
// usually wins that spot on scoring, not a slot restriction. FLEX takes
// anyone but QB. Mirrors the frontend's SLOT_ELIGIBILITY (lib/nativeSlots.ts)
// and the long-standing convention already documented (but not enforced) in
// draft-prep's roster.ts.
var slotEligiblePositions = map[string][]string{
	"QB":   {"QB"},
	"RB":   {"RB"},
	"WR":   {"WR"},
	"TE":   {"TE"},
	"K":    {"K"},
	"FLEX": {"RB", "WR", "TE", "K"},
}

func slotEligibleForPosition(slot, position string) bool {
	eligible, ok := slotEligiblePositions[slot]
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

// assignRosterTx inserts a roster row and its paired contract row inside an
// already-open transaction. AssignLeagueRoster and UseLeagueDraftPick share
// this — "put a player on a roster with a contract" is the same operation
// whether the acquisition was a free-agent signing or a drafted rookie.
func assignRosterTx(ctx context.Context, tx pgx.Tx, leagueID, teamID int64, gsisID, slot, acquiredVia string, salary, signedSeason int, yearsTotal *int) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO league_rosters (league_id, team_id, gsis_id, slot, acquired_via)
		VALUES ($1, $2, $3, $4, $5)
	`, leagueID, teamID, gsisID, slot, acquiredVia); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO league_contracts (league_id, gsis_id, salary, signed_season, years_total, years_used)
		VALUES ($1, $2, $3, $4, $5, 1)
	`, leagueID, gsisID, salary, signedSeason, yearsTotal)
	return err
}

// leagueTeamIDs returns every team in a league, ordered by id — the
// deterministic fallback draft order used when there's no real standings to
// seed one from yet (a season with no scored weeks) and the tiebreak within
// reverseStandingsOrder for teams tied on record. Takes dbtx rather than
// pgx.Tx specifically so it can run either inside an open transaction
// (rollover, pick generation) or as a plain read (reverseStandingsOrder).
func (h *Handler) leagueTeamIDs(ctx context.Context, db dbtx, leagueID int64) ([]int64, error) {
	rows, err := db.Query(ctx, "SELECT id FROM teams WHERE league_id = $1 ORDER BY id", leagueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// generateDraftClassTx inserts one full draft class (rounds × teams) for a
// season inside an already-open transaction. Shared by
// GenerateLeagueDraftPicks and dynasty/redraft rollover, both of which need
// "make next season's picks" as one step inside a larger atomic operation.
// teamIDs is the draft order (worst-record-first once real standings exist —
// see reverseStandingsOrder), and that order repeats every round rather than
// snaking: a real dynasty rookie draft is compensatory every round, so the
// team that finished last should pick first in round 2 as well as round 1,
// not last. overall_pick is assigned sequentially across the whole class in
// that same order — it's what rookie-scale contract pricing reads to place a
// pick on the auction-value curve (internal/handlers/rookie_scale.go).
// ON CONFLICT DO NOTHING: a commissioner may have already generated (and
// possibly started trading) next season's picks ahead of the actual
// rollover — that's the entire point of picks being tradable in advance —
// so rollover must not fail or duplicate on rows that already exist; it
// just carries on with whatever's there (any such pre-existing pick simply
// keeps whatever overall_pick, or lack of one, it already had).
func generateDraftClassTx(ctx context.Context, tx pgx.Tx, leagueID int64, season, rounds int, teamIDs []int64) error {
	overall := 1
	for round := 1; round <= rounds; round++ {
		for _, tid := range teamIDs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO league_draft_picks (league_id, season, round, original_team_id, current_team_id, overall_pick)
				VALUES ($1, $2, $3, $4, $4, $5)
				ON CONFLICT (league_id, season, round, original_team_id) DO NOTHING
			`, leagueID, season, round, tid, overall); err != nil {
				return err
			}
			overall++
		}
	}
	return nil
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

	var playerPosition string
	if err := h.db.QueryRow(r.Context(),
		"SELECT COALESCE(position, '') FROM nfl_players WHERE gsis_id = $1", req.GsisID,
	).Scan(&playerPosition); err != nil {
		respondError(w, http.StatusBadRequest, "unknown player")
		return
	}
	if !slotEligibleForPosition(req.Slot, playerPosition) {
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

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	if msg, err := capCheckAdd(r.Context(), tx, leagueID, req.TeamID, req.SignedSeason, req.Slot, req.Salary); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	} else if msg != "" {
		respondError(w, http.StatusUnprocessableEntity, msg)
		return
	}

	if err := assignRosterTx(r.Context(), tx, leagueID, req.TeamID, req.GsisID, req.Slot, req.AcquiredVia, req.Salary, req.SignedSeason, req.YearsTotal); err != nil {
		respondError(w, http.StatusConflict, "player is already rostered in this league (or not a known player)")
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
	if req.Slot != nil {
		var playerPosition string
		if err := h.db.QueryRow(r.Context(),
			"SELECT COALESCE(position, '') FROM nfl_players WHERE gsis_id = $1", gsisID,
		).Scan(&playerPosition); err != nil {
			respondError(w, http.StatusBadRequest, "unknown player")
			return
		}
		if !slotEligibleForPosition(*req.Slot, playerPosition) {
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

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	// Cap-check before writing anything: a slot/salary change alters this
	// contract's cap hit in place; a team_id change is an addition to the
	// destination team (a new roster spot and its full salary, unrelated to
	// whatever the origin team was charged) — same "additions only" rule
	// CreateLeagueTrade and AssignLeagueRoster follow. Locked FOR UPDATE so
	// the numbers this check reasons about are the ones the writes below
	// actually apply to.
	if req.TeamID != nil || req.Slot != nil || req.Salary != nil {
		var curTeamID int64
		var curSlot string
		var curSalary int
		if err := tx.QueryRow(r.Context(), `
			SELECT lr.team_id, lr.slot, lc.salary
			FROM league_rosters lr
			JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
			WHERE lr.league_id = $1 AND lr.gsis_id = $2
			FOR UPDATE OF lr
		`, leagueID, gsisID).Scan(&curTeamID, &curSlot, &curSalary); err != nil {
			respondError(w, http.StatusNotFound, "player is not rostered in this league")
			return
		}

		newSlot := curSlot
		if req.Slot != nil {
			newSlot = *req.Slot
		}
		newSalary := curSalary
		if req.Salary != nil {
			newSalary = *req.Salary
		}
		season := h.leagueSeasonInt(r, leagueID)

		if req.TeamID != nil && *req.TeamID != curTeamID {
			if msg, err := capCheckAdd(r.Context(), tx, leagueID, *req.TeamID, season, newSlot, newSalary); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			} else if msg != "" {
				respondError(w, http.StatusUnprocessableEntity, msg)
				return
			}
		} else if msg, err := capCheckDelta(r.Context(), tx, leagueID, curTeamID, season, curSlot, curSalary, newSlot, newSalary); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		} else if msg != "" {
			respondError(w, http.StatusUnprocessableEntity, msg)
			return
		}
	}

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
// cascades with it (FK ON DELETE CASCADE) — but under the full-dead-cap
// rule, the salary it represented doesn't disappear with it: writeDeadMoney
// charges every season still owed on the contract before the roster/contract
// rows are deleted, so a cut frees the roster spot but never the cap hit.
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

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var teamID int64
	var salary, yearsUsed int
	var yearsTotal *int
	if err := tx.QueryRow(r.Context(), `
		SELECT lr.team_id, lc.salary, lc.years_total, lc.years_used
		FROM league_rosters lr
		JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
		WHERE lr.league_id = $1 AND lr.gsis_id = $2
		FOR UPDATE OF lr
	`, leagueID, gsisID).Scan(&teamID, &salary, &yearsTotal, &yearsUsed); err != nil {
		respondError(w, http.StatusNotFound, "player is not rostered in this league")
		return
	}

	currentSeason := h.leagueSeasonInt(r, leagueID)
	seasons := deadMoneySeasons(yearsTotal, yearsUsed)
	if salary > 0 {
		if err := writeDeadMoney(r.Context(), tx, leagueID, teamID, gsisID, currentSeason, salary, seasons); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if _, err := tx.Exec(r.Context(),
		"DELETE FROM league_rosters WHERE league_id = $1 AND gsis_id = $2", leagueID, gsisID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"gsis_id": gsisID, "team_id": teamID,
		"dead_money_per_season": salary, "dead_money_seasons": seasons,
	})
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'drop',$3,$4)`,
		leagueID, currentSeason, payload, user.ID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "dropped"})
}

// freeAgentResp is one player not currently rostered in a native league.
type freeAgentResp struct {
	GsisID      string            `json:"gsis_id"`
	Name        string            `json:"name"`
	Position    string            `json:"position"`
	Team        string            `json:"team"`
	HeadshotURL string            `json:"headshot_url,omitempty"`
	ProjFptsPPR *float64          `json:"proj_fpts_ppr"`
	Stats       []playerStatEntry `json:"stats,omitempty"`
	Trend       []float64         `json:"trend,omitempty"`
	Notes       []situationalNote `json:"notes,omitempty"`
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
		SELECT p.gsis_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       pr.proj_fpts_ppr
		FROM nfl_players p
		LEFT JOIN league_rosters lr ON lr.league_id = $1 AND lr.gsis_id = p.gsis_id
		LEFT JOIN nfl_projections pr ON pr.gsis_id = p.gsis_id AND pr.target_season = $2
		WHERE lr.gsis_id IS NULL
		  AND ($3 = '' OR p.position = $3)
		  AND ($6 = '' OR p.name ILIKE '%' || $6 || '%')
		ORDER BY pr.proj_fpts_ppr DESC NULLS LAST, p.name
		LIMIT $4 OFFSET $5
	`, leagueID, season, position, limit, offset, search)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	agents := []freeAgentResp{}
	gsisIDs := make([]string, 0)
	for rows.Next() {
		var a freeAgentResp
		if err := rows.Scan(&a.GsisID, &a.Name, &a.Position, &a.Team, &a.HeadshotURL, &a.ProjFptsPPR); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		agents = append(agents, a)
		gsisIDs = append(gsisIDs, a.GsisID)
	}
	rows.Close()

	statsByPlayer, err := h.relevantPlayerStats(r.Context(), leagueID, season, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	trendByPlayer, err := h.weeklyTrend(r.Context(), leagueID, season, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	notesByPlayer, err := h.loadNotesForPlayers(r.Context(), h.config.DefaultSeason, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range agents {
		agents[i].Stats = statsByPlayer[agents[i].GsisID]
		agents[i].Trend = trendByPlayer[agents[i].GsisID]
		agents[i].Notes = notesByPlayer[agents[i].GsisID]
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
