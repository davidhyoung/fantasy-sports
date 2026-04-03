package handlers

import (
	"log"
	"net/http"
	"strconv"
)

// --- Clean JSON response types ---
// These flatten the Yahoo XML wrapper structs into simple shapes
// that are easy to consume on the frontend.

type scoreboardResp struct {
	Week     int           `json:"week"`
	Matchups []matchupResp `json:"matchups"`
}

type matchupResp struct {
	Week       int               `json:"week"`
	WeekStart  string            `json:"week_start"`
	WeekEnd    string            `json:"week_end"`
	Status     string            `json:"status"`
	IsPlayoffs string            `json:"is_playoffs"`
	Teams      []matchupTeamResp `json:"teams"`
}

type matchupTeamResp struct {
	TeamKey         string `json:"team_key"`
	Name            string `json:"name"`
	LogoURL         string `json:"logo_url,omitempty"`
	Points          string `json:"points"`
	ProjectedPoints string `json:"projected_points"`
}

type standingResp struct {
	TeamKey       string `json:"team_key"`
	Name          string `json:"name"`
	LogoURL       string `json:"logo_url,omitempty"`
	Rank          int    `json:"rank"`
	PlayoffSeed   int    `json:"playoff_seed"`
	Wins          int    `json:"wins"`
	Losses        int    `json:"losses"`
	Ties          int    `json:"ties"`
	Percentage    string `json:"percentage"`
	PointsFor     string `json:"points_for"`
	PointsAgainst string `json:"points_against"`
	StreakType    string `json:"streak_type"`
	StreakValue   int    `json:"streak_value"`
}

// GetLeagueScoreboard handles GET /api/leagues/{id}/scoreboard?week=N.
// Returns the head-to-head matchups for the requested week (defaults to current week).
func (h *Handler) GetLeagueScoreboard(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	yahooKey, status, msg := h.leagueYahooKey(r, id)
	if status != 0 {
		respondError(w, status, msg)
		return
	}

	// Optional ?week=N — 0 means "current week" (Yahoo default).
	week := 0
	if wStr := r.URL.Query().Get("week"); wStr != "" {
		if parsed, err := strconv.Atoi(wStr); err == nil {
			week = parsed
		}
	}

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[scoring] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	scoreboard, err := yc.GetScoreboard(r.Context(), yahooKey, week)
	if err != nil {
		log.Printf("[scoring] GetScoreboard %s week=%d failed: %v", yahooKey, week, err)
		respondError(w, http.StatusBadGateway, "failed to fetch scoreboard: "+err.Error())
		return
	}
	if scoreboard == nil {
		respondJSON(w, http.StatusOK, scoreboardResp{Matchups: []matchupResp{}})
		return
	}

	resp := scoreboardResp{
		Week:     scoreboard.Week,
		Matchups: make([]matchupResp, 0, len(scoreboard.Matchups.Matchup)),
	}
	for _, m := range scoreboard.Matchups.Matchup {
		mr := matchupResp{
			Week:       m.Week,
			WeekStart:  m.WeekStart,
			WeekEnd:    m.WeekEnd,
			Status:     m.Status,
			IsPlayoffs: m.IsPlayoffs,
			Teams:      make([]matchupTeamResp, 0, len(m.MatchupTeams.Team)),
		}
		for _, t := range m.MatchupTeams.Team {
			mr.Teams = append(mr.Teams, matchupTeamResp{
				TeamKey:         t.TeamKey,
				Name:            t.Name,
				LogoURL:         t.LogoURL(),
				Points:          t.TeamPoints.Total,
				ProjectedPoints: t.TeamProjectedPoints.Total,
			})
		}
		resp.Matchups = append(resp.Matchups, mr)
	}

	respondJSON(w, http.StatusOK, resp)
}

// GetLeagueStandings handles GET /api/leagues/{id}/standings.
// Returns teams sorted by rank with their win/loss record and points totals.
func (h *Handler) GetLeagueStandings(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)

	id, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid id")
		return
	}

	yahooKey, status, msg := h.leagueYahooKey(r, id)
	if status != 0 {
		respondError(w, status, msg)
		return
	}

	yc, err := h.newYahooClient(r, user)
	if err != nil {
		log.Printf("[scoring] failed to load tokens for user %d: %v", user.ID, err)
		respondError(w, http.StatusInternalServerError, "failed to load user tokens")
		return
	}

	teams, err := yc.GetStandings(r.Context(), yahooKey)
	if err != nil {
		log.Printf("[scoring] GetStandings %s failed: %v", yahooKey, err)
		respondError(w, http.StatusBadGateway, "failed to fetch standings: "+err.Error())
		return
	}

	resp := make([]standingResp, 0, len(teams))
	for _, t := range teams {
		resp = append(resp, standingResp{
			TeamKey:       t.TeamKey,
			Name:          t.Name,
			LogoURL:       t.LogoURL(),
			Rank:          t.TeamStandings.Rank,
			PlayoffSeed:   t.TeamStandings.PlayoffSeed,
			Wins:          t.TeamStandings.OutcomeTotals.Wins,
			Losses:        t.TeamStandings.OutcomeTotals.Losses,
			Ties:          t.TeamStandings.OutcomeTotals.Ties,
			Percentage:    t.TeamStandings.OutcomeTotals.Percentage,
			PointsFor:     t.TeamStandings.PointsFor,
			PointsAgainst: t.TeamStandings.PointsAgainst,
			StreakType:    t.TeamStandings.Streak.Type,
			StreakValue:   t.TeamStandings.Streak.Value,
		})
	}

	respondJSON(w, http.StatusOK, resp)
}
