package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"
)

func (h *Handler) ListLeagues(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(),
		"SELECT id, name, sport, season, COALESCE(yahoo_key, ''), COALESCE(logo_url, ''), source, format, created_at FROM leagues ORDER BY id",
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	leagues := []models.League{}
	for rows.Next() {
		var l models.League
		if err := rows.Scan(&l.ID, &l.Name, &l.Sport, &l.Season, &l.YahooKey, &l.LogoURL, &l.Source, &l.Format, &l.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		leagues = append(leagues, l)
	}
	respondJSON(w, http.StatusOK, leagues)
}

// createLeagueReq is the body for creating a native league — one with no
// external league behind it. There is no unauthenticated or Yahoo-backed path
// through this endpoint: Yahoo leagues arrive only via POST /api/sync.
type createLeagueReq struct {
	Name     string            `json:"name"`
	Sport    string            `json:"sport"`
	Season   string            `json:"season"`
	Format   string            `json:"format"` // dynasty|keeper|redraft; default redraft
	Settings createSettingsReq `json:"settings"`
	Teams    []createTeamReq   `json:"teams"`
}

type createSettingsReq struct {
	NumTeams           int                `json:"num_teams"`
	Budget             int                `json:"budget"`
	Slots              map[string]int     `json:"slots"`
	Scoring            map[string]float64 `json:"scoring"`
	TaxiSlots          int                `json:"taxi_slots"`
	IRSlots            int                `json:"ir_slots"`
	DraftRounds        int                `json:"draft_rounds"`
	RegularSeasonWeeks int                `json:"regular_season_weeks"`
	// Pointers, not plain floats: 0 is a real, deliberate choice here ("no
	// cap discount at all for stashes") distinct from "not sent" — a plain
	// float64 can't tell the two apart, and collapsing them would silently
	// override a league that wants a 0% discount back to the default every
	// time settings are saved.
	TaxiCapPct *float64 `json:"taxi_cap_pct"`
	IRCapPct   *float64 `json:"ir_cap_pct"`
	// Same pointer reasoning as TaxiCapPct/IRCapPct — a league genuinely
	// might want a 0% reservation floor (sign anyone for any offer).
	FAReservationPct *float64 `json:"fa_reservation_pct"`
	// RookieScale is optional on create too — its zero value already means
	// "use the defaults" (see models.RookieScale), so no pointer needed.
	RookieScale models.RookieScale `json:"rookie_scale"`
}

// Cap discounting defaults for stashed players — see league_settings'
// taxi_cap_pct/ir_cap_pct.
const (
	defaultTaxiCapPct       = 0.25
	defaultIRCapPct         = 0.50
	defaultFAReservationPct = 0.50
)

func pctOrDefault(v *float64, def float64) (float64, error) {
	if v == nil {
		return def, nil
	}
	if *v < 0 || *v > 1 {
		return 0, fmt.Errorf("cap percentage must be between 0 and 1")
	}
	return *v, nil
}

// validateNativeSlots rejects a starting-lineup slot vocabulary that a native
// league can't actually support. nflverse has no team-defense rows (no
// gsis_id to hang a league_rosters row off), so DEF is allowed by the Yahoo
// draft-values override vocabulary but would always fail at the DB layer for
// a native league — reject it up front instead. Yahoo leagues never call
// this; their DEF comes from Yahoo's own player universe.
func validateNativeSlots(slots map[string]int) error {
	if n, ok := slots["DEF"]; ok && n != 0 {
		return fmt.Errorf("DEF is not supported for native leagues yet")
	}
	return nil
}

type createTeamReq struct {
	Name string `json:"name"`
}

type createLeagueResp struct {
	models.League
	Settings models.LeagueSettings `json:"settings"`
	Teams    []models.Team         `json:"teams"`
}

// CreateLeague creates a native league — roster slots, scoring, budget and
// starting teams in one transaction, so the draft board and draft-prep work
// against it the moment it exists.
//
// POST /api/leagues
func (h *Handler) CreateLeague(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	var req createLeagueReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" || req.Sport == "" || req.Season == "" {
		respondError(w, http.StatusBadRequest, "name, sport and season are required")
		return
	}
	switch req.Format {
	case "":
		req.Format = "redraft"
	case "dynasty", "keeper", "redraft":
	default:
		respondError(w, http.StatusBadRequest, "format must be dynasty, keeper or redraft")
		return
	}
	if len(req.Teams) < 2 || len(req.Teams) > 32 {
		respondError(w, http.StatusBadRequest, "a league needs between 2 and 32 teams")
		return
	}
	for _, t := range req.Teams {
		if t.Name == "" {
			respondError(w, http.StatusBadRequest, "every team needs a name")
			return
		}
	}
	if req.Settings.NumTeams == 0 {
		req.Settings.NumTeams = len(req.Teams)
	}
	if req.Settings.NumTeams != len(req.Teams) {
		respondError(w, http.StatusBadRequest, "settings.num_teams must match the number of teams")
		return
	}
	if req.Settings.Budget <= 0 {
		req.Settings.Budget = 200
	}
	if len(req.Settings.Slots) == 0 {
		respondError(w, http.StatusBadRequest, "settings.slots is required")
		return
	}
	if len(req.Settings.Scoring) == 0 {
		respondError(w, http.StatusBadRequest, "settings.scoring is required")
		return
	}
	if err := validateNativeSlots(req.Settings.Slots); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Settings.DraftRounds <= 0 {
		req.Settings.DraftRounds = 4
	}
	if req.Settings.RegularSeasonWeeks <= 0 {
		req.Settings.RegularSeasonWeeks = 14
	}
	taxiCapPct, err := pctOrDefault(req.Settings.TaxiCapPct, defaultTaxiCapPct)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	irCapPct, err := pctOrDefault(req.Settings.IRCapPct, defaultIRCapPct)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	faReservationPct, err := pctOrDefault(req.Settings.FAReservationPct, defaultFAReservationPct)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	slotsJSON, err := json.Marshal(req.Settings.Slots)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid slots")
		return
	}
	scoringJSON, err := json.Marshal(req.Settings.Scoring)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid scoring")
		return
	}
	rookieScaleJSON, err := json.Marshal(req.Settings.RookieScale)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid rookie_scale")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var l models.League
	err = tx.QueryRow(r.Context(), `
		INSERT INTO leagues (name, sport, season, source, format, user_id)
		VALUES ($1, $2, $3, 'native', $4, $5)
		RETURNING id, name, sport, season, source, format, created_at
	`, req.Name, req.Sport, req.Season, req.Format, user.ID,
	).Scan(&l.ID, &l.Name, &l.Sport, &l.Season, &l.Source, &l.Format, &l.CreatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	settings := models.LeagueSettings{
		LeagueID:           l.ID,
		NumTeams:           req.Settings.NumTeams,
		Budget:             req.Settings.Budget,
		Slots:              req.Settings.Slots,
		Scoring:            req.Settings.Scoring,
		TaxiSlots:          req.Settings.TaxiSlots,
		IRSlots:            req.Settings.IRSlots,
		DraftRounds:        req.Settings.DraftRounds,
		RegularSeasonWeeks: req.Settings.RegularSeasonWeeks,
		TaxiCapPct:         taxiCapPct,
		IRCapPct:           irCapPct,
		RookieScale:        req.Settings.RookieScale,
		FAReservationPct:   faReservationPct,
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO league_settings (league_id, num_teams, budget, slots, scoring, taxi_slots, ir_slots, draft_rounds, regular_season_weeks, taxi_cap_pct, ir_cap_pct, rookie_scale, fa_reservation_pct)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, l.ID, settings.NumTeams, settings.Budget, slotsJSON, scoringJSON, settings.TaxiSlots, settings.IRSlots, settings.DraftRounds, settings.RegularSeasonWeeks, settings.TaxiCapPct, settings.IRCapPct, rookieScaleJSON, settings.FAReservationPct,
	); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	teams := make([]models.Team, 0, len(req.Teams))
	for _, t := range req.Teams {
		var team models.Team
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO teams (league_id, name) VALUES ($1, $2)
			RETURNING id, league_id, name
		`, l.ID, t.Name).Scan(&team.ID, &team.LeagueID, &team.Name); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		teams = append(teams, team)
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, createLeagueResp{League: l, Settings: settings, Teams: teams})
}

func (h *Handler) GetLeague(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var l models.League
	err = h.db.QueryRow(r.Context(),
		"SELECT id, name, sport, season, COALESCE(yahoo_key, ''), COALESCE(logo_url, ''), source, format, created_at FROM leagues WHERE id = $1", id,
	).Scan(&l.ID, &l.Name, &l.Sport, &l.Season, &l.YahooKey, &l.LogoURL, &l.Source, &l.Format, &l.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	respondJSON(w, http.StatusOK, l)
}

// requireCommissioner checks that the requesting user owns (commissions) the
// given native league. Yahoo leagues aren't editable through this API at all
// — their fantasy context comes from Yahoo — so any non-native league is
// rejected regardless of ownership.
func (h *Handler) requireCommissioner(r *http.Request, user *models.User, leagueID int64) (status int, msg string) {
	var ownerID *int64
	var source string
	err := h.db.QueryRow(r.Context(),
		"SELECT user_id, source FROM leagues WHERE id = $1", leagueID,
	).Scan(&ownerID, &source)
	if err != nil {
		return http.StatusNotFound, "league not found"
	}
	if source != "native" {
		return http.StatusUnprocessableEntity, "only native leagues can be edited through this API"
	}
	if ownerID == nil || *ownerID != user.ID {
		return http.StatusForbidden, "only this league's commissioner can do that"
	}
	return 0, ""
}
