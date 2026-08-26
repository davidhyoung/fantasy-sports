package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

var errAlreadyRolled = errors.New("rollover already run for that season")

// lockLeagueAtSeason locks the league row and confirms its season hasn't
// moved since the rollover request was computed — the correct idempotency
// gate for "has this rollover already happened," as opposed to checking
// whether next season's draft picks exist (they legitimately can, ahead of
// time, since picks are tradable in advance of the actual rollover).
func lockLeagueAtSeason(ctx context.Context, tx pgx.Tx, leagueID int64, expectedFromSeason int) error {
	var seasonStr string
	if err := tx.QueryRow(ctx, "SELECT season FROM leagues WHERE id = $1 FOR UPDATE", leagueID).Scan(&seasonStr); err != nil {
		return err
	}
	if seasonStr != strconv.Itoa(expectedFromSeason) {
		return errAlreadyRolled
	}
	return nil
}

type rolloverReq struct {
	ToSeason int `json:"to_season"` // optional, defaults to current season + 1
	Rounds   int `json:"rounds"`    // optional draft-class size override
}

// RolloverLeague advances a native league to its next season. Behavior is
// selected by leagues.format: dynasty carries contracts forward and expires
// the ones that hit years_total; redraft releases everyone; keeper leagues
// aren't supported yet — there's no native keeper-designation flow, only the
// Yahoo-draft-result-shaped keeper_wishlist/keeper_rules machinery.
//
// A plain switch rather than a Rollover interface: this handler is the only
// consumer, and there are only two real implementations plus a stub — an
// interface here would add a type and a constructor for no swap-site benefit.
//
// POST /api/leagues/{id}/rollover
func (h *Handler) RolloverLeague(w http.ResponseWriter, r *http.Request) {
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

	var format, fromSeasonStr string
	if err := h.db.QueryRow(r.Context(), "SELECT format, season FROM leagues WHERE id = $1", leagueID).Scan(&format, &fromSeasonStr); err != nil {
		respondError(w, http.StatusNotFound, "league not found")
		return
	}
	fromSeason, err := strconv.Atoi(fromSeasonStr)
	if err != nil {
		respondError(w, http.StatusUnprocessableEntity, "league season is not a parseable year")
		return
	}

	var req rolloverReq
	json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck — tolerate empty body, defaults apply
	toSeason := req.ToSeason
	if toSeason == 0 {
		toSeason = fromSeason + 1
	}
	if toSeason <= fromSeason {
		respondError(w, http.StatusBadRequest, "to_season must be after the league's current season")
		return
	}
	rounds := req.Rounds
	if rounds == 0 {
		h.db.QueryRow(r.Context(), "SELECT draft_rounds FROM league_settings WHERE league_id = $1", leagueID).Scan(&rounds) //nolint:errcheck
		if rounds == 0 {
			rounds = 4
		}
	}

	// A free-agency window's season is frozen at open time (see OpenFAWindow)
	// and never revisited — resolving one after rollover had already moved
	// leagues.season forward would sign contracts stamped with a season the
	// league has already left behind, the same signed_season/currently-played
	// mismatch UseLeagueDraftPick already guards against for a future pick.
	var faWindowOpen bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_fa_windows WHERE league_id = $1 AND resolved_at IS NULL)", leagueID,
	).Scan(&faWindowOpen); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if faWindowOpen {
		respondError(w, http.StatusUnprocessableEntity, "resolve the open free agency window before rolling over the season")
		return
	}

	switch format {
	case "dynasty":
		err = h.rolloverDynasty(r.Context(), user, leagueID, fromSeason, toSeason, rounds)
	case "redraft":
		err = h.rolloverRedraft(r.Context(), user, leagueID, fromSeason, toSeason, rounds)
	case "keeper":
		respondError(w, http.StatusNotImplemented, "rollover is not implemented for native keeper leagues yet")
		return
	default:
		respondError(w, http.StatusUnprocessableEntity, "unknown league format")
		return
	}
	if errors.Is(err, errAlreadyRolled) {
		respondError(w, http.StatusConflict, "league has already been rolled over to that season")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "rolled_over", "from_season": fromSeason, "to_season": toSeason})
}

// rolloverDynasty: every contract's years_used increments; contracts that
// have reached years_total release their player to FA (league_rosters
// delete cascades the contract row via FK); everyone else carries forward
// as-is; a fresh rookie draft class is generated for the new season;
// leagues.season advances; each team's unused cap space from the season
// just ended banks into the new one; a fresh offseason free-agency window
// opens. All in one transaction, logged as kind='rollover'.
func (h *Handler) rolloverDynasty(ctx context.Context, user *models.User, leagueID int64, fromSeason, toSeason, rounds int) error {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockLeagueAtSeason(ctx, tx, leagueID, fromSeason); err != nil {
		return err
	}

	// Bank each team's leftover cap space *before* anything below changes
	// the roster — banked(T, toSeason) has to reflect what a team actually
	// spent across the season that just ended (including contracts about to
	// expire, which were live all season, and any dead money charged to it),
	// not the roster as it'll look the instant after this transaction commits.
	var budget int
	if err := tx.QueryRow(ctx, "SELECT budget FROM league_settings WHERE league_id = $1", leagueID).Scan(&budget); err != nil {
		return err
	}
	teamIDsForBanking, err := h.leagueTeamIDs(ctx, tx, leagueID)
	if err != nil {
		return err
	}
	for _, teamID := range teamIDsForBanking {
		cb, err := teamCap(ctx, tx, leagueID, teamID, fromSeason)
		if err != nil {
			return err
		}
		// Deliberately no floor at zero: a team that finished the season
		// over cap carries that deficit forward as negative banked space,
		// same "being over the cap is its own punishment" posture as
		// everywhere else in this model — there's no amnesty for a team
		// that mismanaged its cap, rollover included.
		if _, err := tx.Exec(ctx, `
			INSERT INTO league_team_seasons (league_id, team_id, season, base_budget, banked)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (league_id, team_id, season) DO UPDATE
			SET base_budget = excluded.base_budget, banked = excluded.banked
		`, leagueID, teamID, toSeason, budget, cb.Available); err != nil {
			return err
		}
	}

	rows, err := tx.Query(ctx, `
		SELECT gsis_id FROM league_contracts
		WHERE league_id = $1 AND years_total IS NOT NULL AND years_used >= years_total
	`, leagueID)
	if err != nil {
		return err
	}
	var expiring []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			rows.Close()
			return err
		}
		expiring = append(expiring, g)
	}
	rows.Close()

	if len(expiring) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM league_rosters WHERE league_id = $1 AND gsis_id = ANY($2)`, leagueID, expiring); err != nil {
			return err
		}
	}
	// Bump years_used only for contracts that survived the release above.
	if _, err := tx.Exec(ctx, `UPDATE league_contracts SET years_used = years_used + 1 WHERE league_id = $1`, leagueID); err != nil {
		return err
	}

	// Next season's draft order comes from how teams finished the season
	// that just ended — the standard "next year's order is set by this
	// year's standings" rule.
	teamIDs, err := h.reverseStandingsOrder(ctx, tx, leagueID, fromSeason)
	if err != nil {
		return err
	}
	if err := generateDraftClassTx(ctx, tx, leagueID, toSeason, rounds, teamIDs); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE leagues SET season = $1 WHERE id = $2`, strconv.Itoa(toSeason), leagueID); err != nil {
		return err
	}

	// A fresh offseason window opens automatically — same posture as the
	// draft class: rollover sets up next season's *state*, it doesn't act
	// on it. Signing still requires the explicit ResolveFAWindow click,
	// same as spending a pick still requires UseLeagueDraftPick.
	if _, err := tx.Exec(ctx, `INSERT INTO league_fa_windows (league_id, season, kind) VALUES ($1, $2, 'offseason')`, leagueID, toSeason); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"from_season": fromSeason, "to_season": toSeason, "released": expiring, "draft_rounds": rounds})
	if _, err := tx.Exec(ctx, `INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'rollover',$3,$4)`, leagueID, toSeason, payload, user.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// rolloverRedraft: every rostered player releases (contracts cascade), a
// fresh full draft class is generated, leagues.season advances, a fresh
// offseason free-agency window opens. No expiring-vs-carrying distinction —
// redraft leagues re-draft everyone. No cap banking either: a redraft
// roster resets to nothing every season, so there's no persistent roster
// for saved cap space to mean anything for — banking is a dynasty-specific
// strategy element.
func (h *Handler) rolloverRedraft(ctx context.Context, user *models.User, leagueID int64, fromSeason, toSeason, rounds int) error {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := lockLeagueAtSeason(ctx, tx, leagueID, fromSeason); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM league_rosters WHERE league_id = $1`, leagueID)
	if err != nil {
		return err
	}

	teamIDs, err := h.reverseStandingsOrder(ctx, tx, leagueID, fromSeason)
	if err != nil {
		return err
	}
	if err := generateDraftClassTx(ctx, tx, leagueID, toSeason, rounds, teamIDs); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `UPDATE leagues SET season = $1 WHERE id = $2`, strconv.Itoa(toSeason), leagueID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO league_fa_windows (league_id, season, kind) VALUES ($1, $2, 'offseason')`, leagueID, toSeason); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{"from_season": fromSeason, "to_season": toSeason, "released_count": tag.RowsAffected(), "draft_rounds": rounds})
	if _, err := tx.Exec(ctx, `INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'rollover',$3,$4)`, leagueID, toSeason, payload, user.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
