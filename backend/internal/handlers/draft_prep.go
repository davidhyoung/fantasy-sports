package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// --- Types ---

// draftPrepEntry is one player on your personal draft board. Only players you've
// actually marked have a row; everyone else is implicitly unrated and unranked.
type draftPrepEntry struct {
	GsisID string `json:"gsis_id"`
	// Interest is +1 to target the player, -1 to avoid him. nil = no opinion.
	Interest   *int   `json:"interest"`
	CustomRank *int   `json:"custom_rank"` // nil = unranked
	// CustomTier overrides the algorithm's tier grouping for this player. nil
	// falls back to the computed tier the draft-values response sends.
	CustomTier *int   `json:"custom_tier"`
	Note       string `json:"note"`
	// PlannedCost is what you intend to pay. nil = not in the team plan at all,
	// which is why it is not defaulted to 0 — $0 is a meaningful bid.
	PlannedCost *int `json:"planned_cost"`
	// MyValue is the user's own valuation, distinct from the algorithm's
	// auction_value. nil = no opinion yet. Set directly, or filled in
	// automatically by the client when the player is moved on the board or in
	// the tiers view (interpolated between the new neighbours' values).
	MyValue *int `json:"my_value"`
}

type draftPrepResp struct {
	Season  int              `json:"season"`
	Players []draftPrepEntry `json:"players"`
}

// Interest is binary. The sign is kept rather than a boolean so ordering still
// works and widening the scale again would be a constraint change, not a schema
// one. nil (not 0) is the single representation of "no opinion", matching the
// column's CHECK constraint.
const (
	interestTarget = 1
	interestAvoid  = -1
)

func validInterest(v *int) bool {
	return v == nil || *v == interestTarget || *v == interestAvoid
}

// prepSeason resolves the ?season= parameter, defaulting to the configured season.
func (h *Handler) prepSeason(r *http.Request) int {
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 1900 && v < 2200 {
			return v
		}
	}
	return h.config.DefaultSeason
}

// --- Handlers ---

// GetDraftPrep returns the signed-in user's draft board for a league and season.
//
// GET /api/leagues/{id}/draft-prep?season=2026
func (h *Handler) GetDraftPrep(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.prepSeason(r)

	rows, err := h.db.Query(r.Context(), `
		SELECT gsis_id, interest, custom_rank, custom_tier, note, planned_cost, my_value
		FROM draft_prep_players
		WHERE user_id = $1 AND league_id = $2 AND season = $3
		ORDER BY custom_rank NULLS LAST, interest DESC NULLS LAST, gsis_id
	`, user.ID, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	players := []draftPrepEntry{}
	for rows.Next() {
		var e draftPrepEntry
		if err := rows.Scan(&e.GsisID, &e.Interest, &e.CustomRank, &e.CustomTier, &e.Note, &e.PlannedCost, &e.MyValue); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		players = append(players, e)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, draftPrepResp{Season: season, Players: players})
}

// UpsertDraftPrepPlayer sets one player's interest level, board rank, tier
// override, note, planned cost and personal value.
//
// PUT /api/leagues/{id}/draft-prep/{gsisId}?season=2026
//
// A player with no interest, rank, tier override, note, planned cost or
// personal value carries no information, so that combination deletes the row
// instead of storing an empty one.
func (h *Handler) UpsertDraftPrepPlayer(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	gsisID := chi.URLParam(r, "gsisId")
	if gsisID == "" {
		respondError(w, http.StatusBadRequest, "missing player id")
		return
	}
	season := h.prepSeason(r)

	var body struct {
		Interest    *int   `json:"interest"`
		CustomRank  *int   `json:"custom_rank"`
		CustomTier  *int   `json:"custom_tier"`
		Note        string `json:"note"`
		PlannedCost *int   `json:"planned_cost"`
		MyValue     *int   `json:"my_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validInterest(body.Interest) {
		respondError(w, http.StatusBadRequest, "interest must be 1 (target) or -1 (avoid)")
		return
	}
	if body.PlannedCost != nil && (*body.PlannedCost < 0 || *body.PlannedCost > 10000) {
		respondError(w, http.StatusBadRequest, "planned_cost out of range")
		return
	}
	if body.MyValue != nil && (*body.MyValue < 0 || *body.MyValue > 10000) {
		respondError(w, http.StatusBadRequest, "my_value out of range")
		return
	}
	if body.CustomRank != nil && (*body.CustomRank < 1 || *body.CustomRank > 10000) {
		respondError(w, http.StatusBadRequest, "custom_rank out of range")
		return
	}
	if body.CustomTier != nil && (*body.CustomTier < 1 || *body.CustomTier > 20) {
		respondError(w, http.StatusBadRequest, "custom_tier out of range")
		return
	}
	if len(body.Note) > 500 {
		body.Note = body.Note[:500]
	}

	if body.Interest == nil && body.CustomRank == nil && body.CustomTier == nil && body.Note == "" && body.PlannedCost == nil && body.MyValue == nil {
		if _, err := h.db.Exec(r.Context(), `
			DELETE FROM draft_prep_players
			WHERE user_id = $1 AND league_id = $2 AND season = $3 AND gsis_id = $4
		`, user.ID, leagueID, season, gsisID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, draftPrepEntry{GsisID: gsisID})
		return
	}

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO draft_prep_players (user_id, league_id, season, gsis_id, interest, custom_rank, custom_tier, note, planned_cost, my_value, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (user_id, league_id, season, gsis_id) DO UPDATE
		SET interest = EXCLUDED.interest, custom_rank = EXCLUDED.custom_rank, custom_tier = EXCLUDED.custom_tier,
		    note = EXCLUDED.note, planned_cost = EXCLUDED.planned_cost, my_value = EXCLUDED.my_value, updated_at = NOW()
	`, user.ID, leagueID, season, gsisID, body.Interest, body.CustomRank, body.CustomTier, body.Note, body.PlannedCost, body.MyValue); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, draftPrepEntry{
		GsisID:      gsisID,
		Interest:    body.Interest,
		CustomRank:  body.CustomRank,
		CustomTier:  body.CustomTier,
		Note:        body.Note,
		PlannedCost: body.PlannedCost,
		MyValue:     body.MyValue,
	})
}

// ReorderDraftPrep replaces the board order in one write — the ordering is a
// whole-list property, so sending it player by player would leave the board in
// an inconsistent state between requests.
//
// PUT /api/leagues/{id}/draft-prep/order?season=2026
// Body: {"gsis_ids": ["00-0034796", ...]}  (position in the array = rank)
func (h *Handler) ReorderDraftPrep(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.prepSeason(r)

	var body struct {
		GsisIDs []string `json:"gsis_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.GsisIDs) > 10000 {
		respondError(w, http.StatusBadRequest, "too many players")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	// Clear existing ranks first so players dropped from the order don't keep a
	// stale position; tags and notes on those rows survive.
	if _, err := tx.Exec(r.Context(), `
		UPDATE draft_prep_players SET custom_rank = NULL, updated_at = NOW()
		WHERE user_id = $1 AND league_id = $2 AND season = $3 AND custom_rank IS NOT NULL
	`, user.ID, leagueID, season); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Whole board in one statement — array position becomes the rank. Written as a
	// single round trip because a reorder covers every ranked player, and a loop of
	// several hundred execs would make each nudge of the board feel like work.
	if len(body.GsisIDs) > 0 {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO draft_prep_players (user_id, league_id, season, gsis_id, custom_rank, updated_at)
			SELECT $1, $2, $3, board.gsis_id, board.ord, NOW()
			FROM unnest($4::text[]) WITH ORDINALITY AS board(gsis_id, ord)
			ON CONFLICT (user_id, league_id, season, gsis_id) DO UPDATE
			SET custom_rank = EXCLUDED.custom_rank, updated_at = NOW()
		`, user.ID, leagueID, season, body.GsisIDs); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]int{"ranked": len(body.GsisIDs)})
}
