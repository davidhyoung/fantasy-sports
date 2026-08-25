package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// dbtx is satisfied by both *pgxpool.Pool and pgx.Tx. Cap math runs both as a
// plain read (GetTeamCap) and as a gate inside an already-open transaction
// that's also locking the rows a mutation is about to change — every
// cap-gated write needs the exact numbers it's about to act on, not a
// separately-committed snapshot, so every helper here takes this instead of
// committing to one or the other.
type dbtx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// capBreakdown is one team's full cap position for one season. See
// .claude/plans/dynasty-transactions.md for the model this implements: a
// hard cap, full dead money on cuts, and cap space that banks between
// seasons.
type capBreakdown struct {
	Season      int     `json:"season"`
	BaseBudget  int     `json:"base_budget"`
	Banked      int     `json:"banked"`
	Cap         int     `json:"cap"` // base_budget + banked
	ActiveSpend int     `json:"active_spend"`
	DeadMoney   int     `json:"dead_money"`
	Spend       int     `json:"spend"` // active_spend + dead_money
	Available   int     `json:"available"`
	RosterCount int     `json:"roster_count"`
	RosterMax   int     `json:"roster_max"`
	TaxiCapPct  float64 `json:"taxi_cap_pct"`
	IRCapPct    float64 `json:"ir_cap_pct"`
}

// slotCapFactor is how much of a contract's salary counts against the cap,
// based on which roster slot the player occupies. A taxi-squad or
// injured-reserve player isn't part of a team's active competitive roster,
// so their salary is discounted rather than exempted — mirrors
// slotEligibleForPosition's "special-case the stash slots" shape.
func slotCapFactor(slot string, taxiCapPct, irCapPct float64) float64 {
	switch slot {
	case "TAXI":
		return taxiCapPct
	case "IR":
		return irCapPct
	default:
		return 1.0
	}
}

// leagueCapSettings loads the league-wide (not team-specific) inputs to cap
// math: the base budget, roster-slot count, and taxi/IR discount rates.
// Split out of teamCap so a check that needs these numbers for several teams
// at once (CreateLeagueTrade) doesn't have to fake a team_id to get them.
func leagueCapSettings(ctx context.Context, db dbtx, leagueID int64) (capBreakdown, error) {
	var cb capBreakdown
	var slotsRaw []byte
	if err := db.QueryRow(ctx, `
		SELECT budget, slots, taxi_cap_pct, ir_cap_pct FROM league_settings WHERE league_id = $1
	`, leagueID).Scan(&cb.BaseBudget, &slotsRaw, &cb.TaxiCapPct, &cb.IRCapPct); err != nil {
		return cb, err
	}
	var slots map[string]int
	if err := json.Unmarshal(slotsRaw, &slots); err != nil {
		return cb, err
	}
	for _, n := range slots {
		cb.RosterMax += n
	}
	return cb, nil
}

// teamCap computes one team's full cap breakdown for a given season. Season
// need not be the league's current season — a contract's coverage
// (signed_season .. signed_season+years_total-1, or forever when
// years_total is NULL, matching how rolloverDynasty never auto-expires a
// NULL-years_total deal) is checked against the requested season, so this
// same query answers both "can this signing happen right now" and "what
// does this team owe in three years" projections.
//
// league_team_seasons has no row until Phase 8's rollover integration writes
// one — a league's first season, and every season before that phase lands,
// falls back to league_settings.budget with banked = 0. That's a genuine
// boundary condition (there is no prior season to have banked anything from
// yet), not a stand-in for unfinished work.
func teamCap(ctx context.Context, db dbtx, leagueID, teamID int64, season int) (capBreakdown, error) {
	cb, err := leagueCapSettings(ctx, db, leagueID)
	if err != nil {
		return cb, err
	}
	cb.Season = season

	if err := db.QueryRow(ctx, `
		SELECT base_budget, banked FROM league_team_seasons WHERE league_id = $1 AND team_id = $2 AND season = $3
	`, leagueID, teamID, season).Scan(&cb.BaseBudget, &cb.Banked); err != nil && err != pgx.ErrNoRows {
		return cb, err
	}
	cb.Cap = cb.BaseBudget + cb.Banked

	rows, err := db.Query(ctx, `
		SELECT lr.slot, lc.salary
		FROM league_rosters lr
		JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
		WHERE lr.league_id = $1 AND lr.team_id = $2
		  AND lc.signed_season <= $3
		  AND (lc.years_total IS NULL OR $3 <= lc.signed_season + lc.years_total - 1)
	`, leagueID, teamID, season)
	if err != nil {
		return cb, err
	}
	for rows.Next() {
		var slot string
		var salary int
		if err := rows.Scan(&slot, &salary); err != nil {
			rows.Close()
			return cb, err
		}
		cb.ActiveSpend += int(math.Round(float64(salary) * slotCapFactor(slot, cb.TaxiCapPct, cb.IRCapPct)))
		cb.RosterCount++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return cb, err
	}

	if err := db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0) FROM league_dead_money WHERE league_id = $1 AND team_id = $2 AND season = $3
	`, leagueID, teamID, season).Scan(&cb.DeadMoney); err != nil {
		return cb, err
	}

	cb.Spend = cb.ActiveSpend + cb.DeadMoney
	cb.Available = cb.Cap - cb.Spend
	return cb, nil
}

// capCheckAdd is the one gate every roster-adding mutation runs before
// writing: room on the roster, and room under the cap for the salary this
// move would add at the given slot. Returns a non-empty, user-facing message
// on failure and empty on success — every call site turns this straight into
// a 422 response, so a typed error would just be unwrapped again one line
// later. "The cap blocks additions, never obligations" (see
// dynasty-transactions.md) — this must only ever gate something being added,
// never an existing contract's cap hit changing on its own (rollover,
// dead money).
func capCheckAdd(ctx context.Context, db dbtx, leagueID, teamID int64, season int, slot string, salary int) (string, error) {
	cb, err := teamCap(ctx, db, leagueID, teamID, season)
	if err != nil {
		return "", err
	}
	if cb.RosterCount+1 > cb.RosterMax {
		return fmt.Sprintf("roster is full (%d/%d spots)", cb.RosterCount, cb.RosterMax), nil
	}
	delta := int(math.Round(float64(salary) * slotCapFactor(slot, cb.TaxiCapPct, cb.IRCapPct)))
	if delta > cb.Available {
		return fmt.Sprintf("over the salary cap: this move needs $%d, team has $%d available", delta, cb.Available), nil
	}
	return "", nil
}

// capCheckDelta gates changing an existing contract's terms (slot and/or
// salary) in place, without moving it to a new team or adding a roster
// spot — same "additions only" rule as capCheckAdd, just without the
// roster-headcount check since the player already occupies a spot on this
// team. A move that lowers cap usage (delta <= 0) is always allowed.
func capCheckDelta(ctx context.Context, db dbtx, leagueID, teamID int64, season int, oldSlot string, oldSalary int, newSlot string, newSalary int) (string, error) {
	cb, err := teamCap(ctx, db, leagueID, teamID, season)
	if err != nil {
		return "", err
	}
	oldFactored := int(math.Round(float64(oldSalary) * slotCapFactor(oldSlot, cb.TaxiCapPct, cb.IRCapPct)))
	newFactored := int(math.Round(float64(newSalary) * slotCapFactor(newSlot, cb.TaxiCapPct, cb.IRCapPct)))
	delta := newFactored - oldFactored
	if delta > cb.Available {
		return fmt.Sprintf("over the salary cap: this change needs $%d more, team has $%d available", delta, cb.Available), nil
	}
	return "", nil
}

// deadMoneySeasons is how many seasons (including the current one) a cut
// player's salary is still owed for under the full-dead-cap rule. A
// year-to-year deal (years_total NULL) owes only the current season; a
// multi-year deal owes the current season plus everything left on it.
func deadMoneySeasons(yearsTotal *int, yearsUsed int) int {
	if yearsTotal == nil {
		return 1
	}
	if left := *yearsTotal - yearsUsed + 1; left > 1 {
		return left
	}
	return 1
}

// writeDeadMoney charges a cut player's salary to `seasons` consecutive
// seasons starting at currentSeason — the full-dead-cap rule: cutting frees
// the roster spot but never the money.
func writeDeadMoney(ctx context.Context, tx pgx.Tx, leagueID, teamID int64, gsisID string, currentSeason, salary, seasons int) error {
	for i := 0; i < seasons; i++ {
		if _, err := tx.Exec(ctx, `
			INSERT INTO league_dead_money (league_id, team_id, season, amount, source_gsis_id)
			VALUES ($1, $2, $3, $4, $5)
		`, leagueID, teamID, currentSeason+i, salary, gsisID); err != nil {
			return err
		}
	}
	return nil
}

// teamCapResp wraps a team's cap breakdowns across a run of seasons — the
// commitment schedule a dynasty manager actually needs to see, not just the
// current season's snapshot. Real multi-season data exists even before
// Phase 8's rollover/banking integration lands: a cut written today already
// charges dead money to future seasons, and multi-year contracts already
// project forward on their own.
type teamCapResp struct {
	LeagueID   int64          `json:"league_id"`
	TeamID     int64          `json:"team_id"`
	Breakdowns []capBreakdown `json:"breakdowns"`
}

// GetTeamCap returns a team's cap breakdown for the league's current season
// and however many seasons after it are requested — dead money from a past
// cut and multi-year contract coverage both already project forward
// correctly; only banked cap space (Phase 8) doesn't yet.
//
// GET /api/leagues/{id}/teams/{teamId}/cap?seasons=N
func (h *Handler) GetTeamCap(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	teamID, err := parseID(r, "teamId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid team id")
		return
	}
	seasons := 3
	if s := r.URL.Query().Get("seasons"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 && v <= 10 {
			seasons = v
		}
	}

	var teamInLeague bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", teamID, leagueID,
	).Scan(&teamInLeague); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !teamInLeague {
		respondError(w, http.StatusNotFound, "team not found in this league")
		return
	}

	startSeason := h.leagueSeasonInt(r, leagueID)
	resp := teamCapResp{LeagueID: leagueID, TeamID: teamID}
	for i := 0; i < seasons; i++ {
		cb, err := teamCap(r.Context(), h.db, leagueID, teamID, startSeason+i)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		resp.Breakdowns = append(resp.Breakdowns, cb)
	}
	respondJSON(w, http.StatusOK, resp)
}
