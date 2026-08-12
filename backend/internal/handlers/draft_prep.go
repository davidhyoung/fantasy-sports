package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// --- Types ---

// draftPrepEntry is one player on your personal draft board. Only players you've
// actually marked have a row; everyone else is implicitly untagged and unranked.
type draftPrepEntry struct {
	GsisID     string `json:"gsis_id"`
	Tag        string `json:"tag"`         // "target" | "sleeper" | "avoid" | ""
	CustomRank *int   `json:"custom_rank"` // nil = unranked
	Note       string `json:"note"`
}

type draftPrepResp struct {
	Season  int              `json:"season"`
	Players []draftPrepEntry `json:"players"`
}

// validTags are the only tags accepted; anything else is rejected rather than
// silently stored, since the column carries a matching CHECK constraint.
var validTags = map[string]bool{"": true, "target": true, "sleeper": true, "avoid": true}

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
		SELECT gsis_id, COALESCE(tag, ''), custom_rank, note
		FROM draft_prep_players
		WHERE user_id = $1 AND league_id = $2 AND season = $3
		ORDER BY custom_rank NULLS LAST, gsis_id
	`, user.ID, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	players := []draftPrepEntry{}
	for rows.Next() {
		var e draftPrepEntry
		if err := rows.Scan(&e.GsisID, &e.Tag, &e.CustomRank, &e.Note); err != nil {
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

// UpsertDraftPrepPlayer sets one player's tag, board rank and note.
//
// PUT /api/leagues/{id}/draft-prep/{gsisId}?season=2026
//
// A player with no tag, no rank and no note carries no information, so that
// combination deletes the row instead of storing an empty one.
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
		Tag        string `json:"tag"`
		CustomRank *int   `json:"custom_rank"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validTags[body.Tag] {
		respondError(w, http.StatusBadRequest, "tag must be target, sleeper or avoid")
		return
	}
	if body.CustomRank != nil && (*body.CustomRank < 1 || *body.CustomRank > 10000) {
		respondError(w, http.StatusBadRequest, "custom_rank out of range")
		return
	}
	if len(body.Note) > 500 {
		body.Note = body.Note[:500]
	}

	if body.Tag == "" && body.CustomRank == nil && body.Note == "" {
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

	var tag any
	if body.Tag != "" {
		tag = body.Tag
	}
	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO draft_prep_players (user_id, league_id, season, gsis_id, tag, custom_rank, note, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (user_id, league_id, season, gsis_id) DO UPDATE
		SET tag = EXCLUDED.tag, custom_rank = EXCLUDED.custom_rank,
		    note = EXCLUDED.note, updated_at = NOW()
	`, user.ID, leagueID, season, gsisID, tag, body.CustomRank, body.Note); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, draftPrepEntry{
		GsisID:     gsisID,
		Tag:        body.Tag,
		CustomRank: body.CustomRank,
		Note:       body.Note,
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
