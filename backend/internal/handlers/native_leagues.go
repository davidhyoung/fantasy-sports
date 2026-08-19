package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

// GetLeagueSettings returns a native league's roster/scoring configuration —
// the same canonical vocabulary the draft-values ?slots=&scoring= overrides use.
//
// GET /api/leagues/{id}/settings
func (h *Handler) GetLeagueSettings(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}

	var s models.LeagueSettings
	var slotsRaw, scoringRaw []byte
	err = h.db.QueryRow(r.Context(), `
		SELECT league_id, num_teams, budget, slots, scoring, taxi_slots, ir_slots, draft_rounds
		FROM league_settings WHERE league_id = $1
	`, leagueID).Scan(&s.LeagueID, &s.NumTeams, &s.Budget, &slotsRaw, &scoringRaw, &s.TaxiSlots, &s.IRSlots, &s.DraftRounds)
	if err != nil {
		respondError(w, http.StatusNotFound, "no settings for this league")
		return
	}
	if err := json.Unmarshal(slotsRaw, &s.Slots); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := json.Unmarshal(scoringRaw, &s.Scoring); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, s)
}

// UpdateLeagueSettings replaces a native league's roster/scoring configuration
// wholesale — a setting only means something relative to every other setting
// in the blob, same convention as the draft-prep board's reorder endpoint.
//
// PUT /api/leagues/{id}/settings
func (h *Handler) UpdateLeagueSettings(w http.ResponseWriter, r *http.Request) {
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

	var req createSettingsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.NumTeams <= 0 {
		respondError(w, http.StatusBadRequest, "num_teams is required")
		return
	}
	if req.Budget <= 0 {
		respondError(w, http.StatusBadRequest, "budget is required")
		return
	}
	if len(req.Slots) == 0 {
		respondError(w, http.StatusBadRequest, "slots is required")
		return
	}
	if len(req.Scoring) == 0 {
		respondError(w, http.StatusBadRequest, "scoring is required")
		return
	}
	if err := validateNativeSlots(req.Slots); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DraftRounds <= 0 {
		req.DraftRounds = 4
	}
	slotsJSON, err := json.Marshal(req.Slots)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid slots")
		return
	}
	scoringJSON, err := json.Marshal(req.Scoring)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid scoring")
		return
	}

	tag, err := h.db.Exec(r.Context(), `
		UPDATE league_settings
		SET num_teams = $2, budget = $3, slots = $4, scoring = $5,
		    taxi_slots = $6, ir_slots = $7, draft_rounds = $8, updated_at = NOW()
		WHERE league_id = $1
	`, leagueID, req.NumTeams, req.Budget, slotsJSON, scoringJSON, req.TaxiSlots, req.IRSlots, req.DraftRounds)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "no settings for this league")
		return
	}

	respondJSON(w, http.StatusOK, models.LeagueSettings{
		LeagueID: leagueID, NumTeams: req.NumTeams, Budget: req.Budget,
		Slots: req.Slots, Scoring: req.Scoring, TaxiSlots: req.TaxiSlots, IRSlots: req.IRSlots,
		DraftRounds: req.DraftRounds,
	})
}

// CreateLeagueTeam adds a team to a native league. Yahoo leagues get their
// teams from /api/sync — commissioners can't add one here.
//
// POST /api/leagues/{id}/teams
func (h *Handler) CreateLeagueTeam(w http.ResponseWriter, r *http.Request) {
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

	var req createTeamReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	var t models.Team
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO teams (league_id, name) VALUES ($1, $2)
		RETURNING id, league_id, name
	`, leagueID, req.Name).Scan(&t.ID, &t.LeagueID, &t.Name)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, t)
}

// UpdateLeagueTeam renames a team on a native league.
//
// PUT /api/leagues/{id}/teams/{teamId}
func (h *Handler) UpdateLeagueTeam(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
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
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req createTeamReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	tag, err := h.db.Exec(r.Context(),
		"UPDATE teams SET name = $3 WHERE id = $1 AND league_id = $2", teamID, leagueID, req.Name,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteLeagueTeam removes a team from a native league.
//
// DELETE /api/leagues/{id}/teams/{teamId}
func (h *Handler) DeleteLeagueTeam(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
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
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	tag, err := h.db.Exec(r.Context(),
		"DELETE FROM teams WHERE id = $1 AND league_id = $2", teamID, leagueID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "team not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}
