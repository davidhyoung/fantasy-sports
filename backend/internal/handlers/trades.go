package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type tradeAssetReq struct {
	Kind     string `json:"kind"` // "player" | "pick"
	GsisID   string `json:"gsis_id,omitempty"`
	PickID   int64  `json:"pick_id,omitempty"`
	ToTeamID int64  `json:"to_team_id"` // destination team
}

type createTradeReq struct {
	Assets []tradeAssetReq `json:"assets"`
}

// CreateLeagueTrade moves any mix of players and future draft picks between
// teams atomically. Single-user model: the commissioner executes the trade
// directly — no accept/propose handshake — but every move's from/to team is
// still recorded in league_transactions, so a future multi-user accept-flow
// has the data it needs without a schema change.
//
// POST /api/leagues/{id}/trades
func (h *Handler) CreateLeagueTrade(w http.ResponseWriter, r *http.Request) {
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

	var req createTradeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Assets) < 2 {
		respondError(w, http.StatusBadRequest, "a trade needs at least two assets")
		return
	}
	seen := map[string]bool{}
	for _, a := range req.Assets {
		var key string
		switch a.Kind {
		case "player":
			if a.GsisID == "" {
				respondError(w, http.StatusBadRequest, "gsis_id required for a player asset")
				return
			}
			key = "player:" + a.GsisID
		case "pick":
			if a.PickID == 0 {
				respondError(w, http.StatusBadRequest, "pick_id required for a pick asset")
				return
			}
			key = fmt.Sprintf("pick:%d", a.PickID)
		default:
			respondError(w, http.StatusBadRequest, "asset kind must be player or pick")
			return
		}
		if seen[key] {
			respondError(w, http.StatusBadRequest, "duplicate asset in trade")
			return
		}
		seen[key] = true
		if a.ToTeamID == 0 {
			respondError(w, http.StatusBadRequest, "to_team_id is required for every asset")
			return
		}
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	// Pass 1: resolve and lock every asset (FOR UPDATE), same validation as
	// before, but nothing is written yet — a trade's cap effect on a team
	// depends on every asset moving in or out of it, so the check below has
	// to see the whole trade's net effect before any of it is applied.
	type resolved struct {
		asset      tradeAssetReq
		fromTeamID int64
		slot       string // player assets only
		salary     int    // player assets only
		round      int    // pick assets only
		season     int    // pick assets only
	}
	var players, picks []resolved
	for _, a := range req.Assets {
		var toTeamInLeague bool
		if err := tx.QueryRow(r.Context(),
			"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", a.ToTeamID, leagueID,
		).Scan(&toTeamInLeague); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !toTeamInLeague {
			respondError(w, http.StatusBadRequest, "to_team_id is not in this league")
			return
		}

		if a.Kind == "player" {
			var res resolved
			res.asset = a
			err := tx.QueryRow(r.Context(), `
				SELECT lr.team_id, lr.slot, lc.salary
				FROM league_rosters lr
				JOIN league_contracts lc ON lc.league_id = lr.league_id AND lc.gsis_id = lr.gsis_id
				WHERE lr.league_id = $1 AND lr.gsis_id = $2
				FOR UPDATE OF lr
			`, leagueID, a.GsisID).Scan(&res.fromTeamID, &res.slot, &res.salary)
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(w, http.StatusBadRequest, "player is not rostered in this league")
				return
			}
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if res.fromTeamID == a.ToTeamID {
				respondError(w, http.StatusBadRequest, "player is already on the destination team")
				return
			}
			players = append(players, res)
		} else {
			var res resolved
			res.asset = a
			var used *string
			err := tx.QueryRow(r.Context(),
				`SELECT current_team_id, used_on_gsis_id, round, season FROM league_draft_picks WHERE id = $1 AND league_id = $2 FOR UPDATE`,
				a.PickID, leagueID,
			).Scan(&res.fromTeamID, &used, &res.round, &res.season)
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
			if res.fromTeamID == a.ToTeamID {
				respondError(w, http.StatusBadRequest, "pick is already owned by the destination team")
				return
			}
			picks = append(picks, res)
		}
	}

	teamsSeen := map[int64]bool{}
	for _, res := range players {
		teamsSeen[res.fromTeamID] = true
		teamsSeen[res.asset.ToTeamID] = true
	}
	for _, res := range picks {
		teamsSeen[res.fromTeamID] = true
		teamsSeen[res.asset.ToTeamID] = true
	}
	if len(teamsSeen) < 2 {
		respondError(w, http.StatusBadRequest, "a trade must involve at least two teams")
		return
	}

	// Cap gate: only players move payroll (picks don't), and only a team
	// receiving more salary than it sends away is "adding" anything — a
	// team shedding salary via this same trade, even one currently over
	// cap, is always allowed to do that. Same "additions only" rule as
	// every other cap-gated mutation.
	season := h.leagueSeasonInt(r, leagueID)
	capDelta := map[int64]int{}
	countDelta := map[int64]int{}
	settingsCB, err := leagueCapSettings(r.Context(), tx, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, res := range players {
		factored := int(math.Round(float64(res.salary) * slotCapFactor(res.slot, settingsCB.TaxiCapPct, settingsCB.IRCapPct)))
		capDelta[res.asset.ToTeamID] += factored
		capDelta[res.fromTeamID] -= factored
		countDelta[res.asset.ToTeamID]++
		countDelta[res.fromTeamID]--
	}
	for teamID := range teamsSeen {
		if capDelta[teamID] <= 0 && countDelta[teamID] <= 0 {
			continue
		}
		cb, err := teamCap(r.Context(), tx, leagueID, teamID, season)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if countDelta[teamID] > 0 && cb.RosterCount+countDelta[teamID] > cb.RosterMax {
			respondError(w, http.StatusUnprocessableEntity, fmt.Sprintf("team %d's roster would be full (%d/%d spots)", teamID, cb.RosterCount+countDelta[teamID], cb.RosterMax))
			return
		}
		if capDelta[teamID] > cb.Available {
			respondError(w, http.StatusUnprocessableEntity, fmt.Sprintf("team %d would be over the salary cap: this trade needs $%d, they have $%d available", teamID, capDelta[teamID], cb.Available))
			return
		}
	}

	// Pass 2: the checks passed, so actually move everything.
	moves := []map[string]any{}
	for _, res := range players {
		if _, err := tx.Exec(r.Context(),
			`UPDATE league_rosters SET team_id = $1 WHERE league_id = $2 AND gsis_id = $3`,
			res.asset.ToTeamID, leagueID, res.asset.GsisID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		moves = append(moves, map[string]any{"asset": "player", "gsis_id": res.asset.GsisID, "from_team_id": res.fromTeamID, "to_team_id": res.asset.ToTeamID})
	}
	for _, res := range picks {
		if _, err := tx.Exec(r.Context(),
			`UPDATE league_draft_picks SET current_team_id = $1 WHERE id = $2`, res.asset.ToTeamID, res.asset.PickID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		moves = append(moves, map[string]any{"asset": "pick", "pick_id": res.asset.PickID, "season": res.season, "round": res.round, "from_team_id": res.fromTeamID, "to_team_id": res.asset.ToTeamID})
	}

	payload, _ := json.Marshal(map[string]any{"moves": moves})
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'trade',$3,$4)`,
		leagueID, season, payload, user.ID,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{"status": "traded", "moves": moves})
}

// transactionResp is one entry in a native league's transaction log.
type transactionResp struct {
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

// ListLeagueTransactions returns a native league's recent activity —
// drafts, trades, and rollovers — newest first. Read-only, no commissioner
// gate, same as the other native-league list endpoints.
//
// GET /api/leagues/{id}/transactions?season=
func (h *Handler) ListLeagueTransactions(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	var season int
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT kind, payload, created_at FROM league_transactions
		WHERE league_id = $1 AND ($2 = 0 OR season = $2)
		ORDER BY created_at DESC LIMIT 50
	`, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	txns := []transactionResp{}
	for rows.Next() {
		var t transactionResp
		var raw []byte
		if err := rows.Scan(&t.Kind, &raw, &t.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		json.Unmarshal(raw, &t.Payload) //nolint:errcheck — payload is always valid JSON, we wrote it
		txns = append(txns, t)
	}
	respondJSON(w, http.StatusOK, txns)
}
