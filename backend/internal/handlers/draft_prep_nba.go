package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// draftPrepEntryNBA is one player on your personal NBA draft board. Only
// players you've actually marked have a row — everyone else is implicitly
// unrated. Simplified relative to the NFL board (no custom_rank/tier,
// planned_cost, my_value) — those exist on the NFL side because it has
// auction dollars and a tiering system to plan against; native NBA leagues
// have neither yet, so there's nothing for a cost or tier column to mean.
type draftPrepEntryNBA struct {
	ESPNID string `json:"espn_id"`
	// +1 to target, -1 to avoid. nil = no opinion.
	Interest *int   `json:"interest"`
	Note     string `json:"note"`
}

type draftPrepRespNBA struct {
	Season  int                 `json:"season"`
	Players []draftPrepEntryNBA `json:"players"`
}

func validInterestNBA(v *int) bool {
	return v == nil || *v == interestTarget || *v == interestAvoid
}

// GetDraftPrepNBA returns the signed-in user's NBA draft board for a league
// and season.
//
// GET /api/leagues/{id}/draft-prep-nba?season=
func (h *Handler) GetDraftPrepNBA(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.prepSeason(r)

	rows, err := h.db.Query(r.Context(), `
		SELECT espn_id, interest, note
		FROM draft_prep_players_nba
		WHERE user_id = $1 AND league_id = $2 AND season = $3
		ORDER BY interest DESC NULLS LAST, espn_id
	`, user.ID, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	players := []draftPrepEntryNBA{}
	for rows.Next() {
		var e draftPrepEntryNBA
		if err := rows.Scan(&e.ESPNID, &e.Interest, &e.Note); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		players = append(players, e)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, draftPrepRespNBA{Season: season, Players: players})
}

// UpsertDraftPrepPlayerNBA sets one player's interest flag and note.
//
// PUT /api/leagues/{id}/draft-prep-nba/{espnId}?season=
//
// A player with no interest and no note carries no information, so that
// combination deletes the row instead of storing an empty one.
func (h *Handler) UpsertDraftPrepPlayerNBA(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	espnID := chi.URLParam(r, "espnId")
	if espnID == "" {
		respondError(w, http.StatusBadRequest, "missing player id")
		return
	}
	season := h.prepSeason(r)

	var body struct {
		Interest *int   `json:"interest"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if !validInterestNBA(body.Interest) {
		respondError(w, http.StatusBadRequest, "interest must be 1 (target) or -1 (avoid)")
		return
	}
	if len(body.Note) > 500 {
		body.Note = body.Note[:500]
	}

	if body.Interest == nil && body.Note == "" {
		if _, err := h.db.Exec(r.Context(), `
			DELETE FROM draft_prep_players_nba
			WHERE user_id = $1 AND league_id = $2 AND season = $3 AND espn_id = $4
		`, user.ID, leagueID, season, espnID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, draftPrepEntryNBA{ESPNID: espnID})
		return
	}

	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO draft_prep_players_nba (user_id, league_id, season, espn_id, interest, note, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (user_id, league_id, season, espn_id) DO UPDATE
		SET interest = EXCLUDED.interest, note = EXCLUDED.note, updated_at = NOW()
	`, user.ID, leagueID, season, espnID, body.Interest, body.Note); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, draftPrepEntryNBA{ESPNID: espnID, Interest: body.Interest, Note: body.Note})
}
