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
		SELECT league_id, num_teams, budget, slots, scoring, taxi_slots, ir_slots, draft_rounds, regular_season_weeks, taxi_cap_pct, ir_cap_pct
		FROM league_settings WHERE league_id = $1
	`, leagueID).Scan(&s.LeagueID, &s.NumTeams, &s.Budget, &slotsRaw, &scoringRaw, &s.TaxiSlots, &s.IRSlots, &s.DraftRounds, &s.RegularSeasonWeeks, &s.TaxiCapPct, &s.IRCapPct)
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
	if req.RegularSeasonWeeks <= 0 {
		req.RegularSeasonWeeks = 14
	}
	// This is an update, not a creation — an omitted field falls back to
	// whatever the league already has, not the global default, or every save
	// from a client that doesn't yet send these two fields would silently
	// reset a deliberately-customized discount back to 0.25/0.50.
	var currentTaxiCapPct, currentIRCapPct float64
	if err := h.db.QueryRow(r.Context(),
		"SELECT taxi_cap_pct, ir_cap_pct FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&currentTaxiCapPct, &currentIRCapPct); err != nil {
		respondError(w, http.StatusNotFound, "no settings for this league")
		return
	}
	taxiCapPct, err := capPctOrDefault(req.TaxiCapPct, currentTaxiCapPct)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	irCapPct, err := capPctOrDefault(req.IRCapPct, currentIRCapPct)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
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
		    taxi_slots = $6, ir_slots = $7, draft_rounds = $8, regular_season_weeks = $9,
		    taxi_cap_pct = $10, ir_cap_pct = $11, updated_at = NOW()
		WHERE league_id = $1
	`, leagueID, req.NumTeams, req.Budget, slotsJSON, scoringJSON, req.TaxiSlots, req.IRSlots, req.DraftRounds, req.RegularSeasonWeeks, taxiCapPct, irCapPct)
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
		TaxiCapPct: taxiCapPct, IRCapPct: irCapPct,
		DraftRounds: req.DraftRounds, RegularSeasonWeeks: req.RegularSeasonWeeks,
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

// updateTeamReq is the body for PUT .../teams/{teamId}. Both fields are
// optional and independent — Name renames, Claim assigns/releases team
// ownership (teams.user_id) for the calling user. A native league team
// starts unclaimed (created with no user_id at all — only Yahoo sync sets
// that on its own upsert), so this is how "My Team" becomes reachable.
type updateTeamReq struct {
	Name  *string `json:"name"`
	Claim *bool   `json:"claim"`
}

// UpdateLeagueTeam renames a team and/or claims/releases it as the calling
// user's own team on a native league.
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

	var req updateTeamReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name != nil && *req.Name == "" {
		respondError(w, http.StatusBadRequest, "name cannot be empty")
		return
	}
	if req.Name == nil && req.Claim == nil {
		respondError(w, http.StatusBadRequest, "name or claim is required")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	if req.Name != nil {
		tag, err := tx.Exec(r.Context(),
			"UPDATE teams SET name = $3 WHERE id = $1 AND league_id = $2", teamID, leagueID, *req.Name,
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "team not found")
			return
		}
	}

	if req.Claim != nil {
		if *req.Claim {
			// A user owns at most one team per league — release any other
			// team of theirs here before claiming this one.
			if _, err := tx.Exec(r.Context(),
				"UPDATE teams SET user_id = NULL WHERE league_id = $1 AND user_id = $2", leagueID, user.ID,
			); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			tag, err := tx.Exec(r.Context(),
				"UPDATE teams SET user_id = $3 WHERE id = $1 AND league_id = $2", teamID, leagueID, user.ID,
			)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if tag.RowsAffected() == 0 {
				respondError(w, http.StatusNotFound, "team not found")
				return
			}
		} else {
			tag, err := tx.Exec(r.Context(),
				"UPDATE teams SET user_id = NULL WHERE id = $1 AND league_id = $2", teamID, leagueID,
			)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if tag.RowsAffected() == 0 {
				respondError(w, http.StatusNotFound, "team not found")
				return
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
