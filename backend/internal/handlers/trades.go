package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
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

	moves := []map[string]any{}
	teamsSeen := map[int64]bool{}
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
			var fromTeamID int64
			err := tx.QueryRow(r.Context(),
				`SELECT team_id FROM league_rosters WHERE league_id = $1 AND gsis_id = $2 FOR UPDATE`,
				leagueID, a.GsisID,
			).Scan(&fromTeamID)
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(w, http.StatusBadRequest, "player is not rostered in this league")
				return
			}
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if fromTeamID == a.ToTeamID {
				respondError(w, http.StatusBadRequest, "player is already on the destination team")
				return
			}
			if _, err := tx.Exec(r.Context(),
				`UPDATE league_rosters SET team_id = $1 WHERE league_id = $2 AND gsis_id = $3`,
				a.ToTeamID, leagueID, a.GsisID,
			); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			teamsSeen[fromTeamID] = true
			teamsSeen[a.ToTeamID] = true
			moves = append(moves, map[string]any{"asset": "player", "gsis_id": a.GsisID, "from_team_id": fromTeamID, "to_team_id": a.ToTeamID})
		} else {
			var fromTeamID int64
			var used *string
			var round, season int
			err := tx.QueryRow(r.Context(),
				`SELECT current_team_id, used_on_gsis_id, round, season FROM league_draft_picks WHERE id = $1 AND league_id = $2 FOR UPDATE`,
				a.PickID, leagueID,
			).Scan(&fromTeamID, &used, &round, &season)
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
			if fromTeamID == a.ToTeamID {
				respondError(w, http.StatusBadRequest, "pick is already owned by the destination team")
				return
			}
			if _, err := tx.Exec(r.Context(),
				`UPDATE league_draft_picks SET current_team_id = $1 WHERE id = $2`, a.ToTeamID, a.PickID,
			); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			teamsSeen[fromTeamID] = true
			teamsSeen[a.ToTeamID] = true
			moves = append(moves, map[string]any{"asset": "pick", "pick_id": a.PickID, "season": season, "round": round, "from_team_id": fromTeamID, "to_team_id": a.ToTeamID})
		}
	}
	if len(teamsSeen) < 2 {
		respondError(w, http.StatusBadRequest, "a trade must involve at least two teams")
		return
	}

	payload, _ := json.Marshal(map[string]any{"moves": moves})
	season := h.leagueSeasonInt(r, leagueID)
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
