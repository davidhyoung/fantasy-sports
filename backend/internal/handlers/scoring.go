package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"
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
	TeamKey string `json:"team_key"`
	// TeamID is additive — populated only on the native path, so a native
	// frontend can key off a real id instead of reverse-engineering a fake
	// team_key. Yahoo responses leave this zero/omitted, unchanged shape.
	TeamID          int64  `json:"team_id,omitempty"`
	Name            string `json:"name"`
	LogoURL         string `json:"logo_url,omitempty"`
	Points          string `json:"points"`
	ProjectedPoints string `json:"projected_points"`
}

type standingResp struct {
	TeamKey       string `json:"team_key"`
	TeamID        int64  `json:"team_id,omitempty"` // additive, native path only — see matchupTeamResp
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
	// Native leagues only (see below) — chronological "W"/"L"/"T" for the
	// team's last up to 5 scored matchups, and whether this rank currently
	// makes the postseason. There's no real playoff-teams setting yet, so the
	// cutoff is a heuristic off league size (see playoffCutoff), not a
	// configured value — flagged as a simplification, not exact.
	Last5         []string `json:"last5,omitempty"`
	MakesPlayoffs bool     `json:"makes_playoffs,omitempty"`
}

// playoffCutoff approximates how many teams make the postseason from league
// size alone, since native leagues have no real playoff-teams setting (no
// bracket/seeding is built at all — see native-leagues plan). Small leagues
// send a majority to the postseason; larger ones settle to half.
func playoffCutoff(numTeams int) int {
	switch {
	case numTeams <= 4:
		return 2
	case numTeams <= 8:
		return 4
	default:
		return numTeams / 2
	}
}

// leagueSource looks up a league's source ("yahoo"|"native"); empty string
// on error (treated as not-native by callers, matching prior behavior for a
// missing/invalid league id — the existing 404 path still fires downstream).
func (h *Handler) leagueSource(r *http.Request, leagueID int64) string {
	var source string
	h.db.QueryRow(r.Context(), "SELECT source FROM leagues WHERE id = $1", leagueID).Scan(&source) //nolint:errcheck
	return source
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

	if h.leagueSource(r, id) == "native" {
		h.nativeScoreboard(w, r, id)
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

	if h.leagueSource(r, id) == "native" {
		h.nativeStandings(w, r, id)
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

// nativeScoreboard serves GetLeagueScoreboard for a native league from
// league_matchups instead of Yahoo. Defaults to the latest week a schedule
// has been generated for when ?week= is omitted.
func (h *Handler) nativeScoreboard(w http.ResponseWriter, r *http.Request, leagueID int64) {
	q := r.URL.Query()
	season := h.leagueSeasonInt(r, leagueID)
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	week := 0
	if wStr := q.Get("week"); wStr != "" {
		if v, err := strconv.Atoi(wStr); err == nil {
			week = v
		}
	}
	if week == 0 {
		h.db.QueryRow(r.Context(), //nolint:errcheck
			"SELECT COALESCE(MAX(week), 0) FROM league_matchups WHERE league_id = $1 AND season = $2",
			leagueID, season,
		).Scan(&week)
	}
	if week == 0 {
		respondJSON(w, http.StatusOK, scoreboardResp{Matchups: []matchupResp{}})
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT m.team1_id, t1.name, m.team1_points, m.team2_id, t2.name, m.team2_points,
		       m.is_playoffs, m.computed_at IS NOT NULL
		FROM league_matchups m
		JOIN teams t1 ON t1.id = m.team1_id
		JOIN teams t2 ON t2.id = m.team2_id
		WHERE m.league_id = $1 AND m.season = $2 AND m.week = $3
		ORDER BY m.id
	`, leagueID, season, week)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type row struct {
		t1ID, t2ID       int64
		t1Name, t2Name   string
		p1, p2           float64
		isPlayoffs, scored bool
	}
	var scanned []row
	teamSet := map[int64]bool{}
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.t1ID, &rw.t1Name, &rw.p1, &rw.t2ID, &rw.t2Name, &rw.p2, &rw.isPlayoffs, &rw.scored); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		scanned = append(scanned, rw)
		if !rw.scored {
			teamSet[rw.t1ID] = true
			teamSet[rw.t2ID] = true
		}
	}
	unscoredTeamIDs := make([]int64, 0, len(teamSet))
	for id := range teamSet {
		unscoredTeamIDs = append(unscoredTeamIDs, id)
	}
	// Projected points give an unscored card something better than a blank
	// dash — never written, purely a display estimate (see nativeProjectedPoints).
	projected := h.nativeProjectedPoints(r.Context(), leagueID, unscoredTeamIDs, season)

	resp := scoreboardResp{Week: week, Matchups: []matchupResp{}}
	for _, rw := range scanned {
		playoffsFlag := "0"
		if rw.isPlayoffs {
			playoffsFlag = "1"
		}
		status := "preevent"
		if rw.scored {
			status = "postevent"
		}
		t1 := matchupTeamResp{TeamID: rw.t1ID, Name: rw.t1Name, Points: fmt.Sprintf("%.2f", rw.p1)}
		t2 := matchupTeamResp{TeamID: rw.t2ID, Name: rw.t2Name, Points: fmt.Sprintf("%.2f", rw.p2)}
		if !rw.scored {
			t1.ProjectedPoints = fmt.Sprintf("%.1f", projected[rw.t1ID])
			t2.ProjectedPoints = fmt.Sprintf("%.1f", projected[rw.t2ID])
		}
		resp.Matchups = append(resp.Matchups, matchupResp{
			Week:       week,
			Status:     status,
			IsPlayoffs: playoffsFlag,
			Teams:      []matchupTeamResp{t1, t2},
		})
	}
	respondJSON(w, http.StatusOK, resp)
}

// nativeStandings serves GetLeagueStandings for a native league by
// aggregating scored league_matchups rows — there's no separate standings
// table, wins/losses/points are a pure read over matchup history.
func (h *Handler) nativeStandings(w http.ResponseWriter, r *http.Request, leagueID int64) {
	season := h.leagueSeasonInt(r, leagueID)
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	type record struct {
		teamID              int64
		name                string
		wins, losses, ties  int
		pointsFor, pointsAg float64
		last5               []string
	}
	records := map[int64]*record{}

	trows, err := h.db.Query(r.Context(), "SELECT id, name FROM teams WHERE league_id = $1", leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for trows.Next() {
		var id int64
		var name string
		if err := trows.Scan(&id, &name); err != nil {
			trows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		records[id] = &record{teamID: id, name: name}
	}
	trows.Close()

	// Ordered by week so last5 (below) reflects actual chronological order,
	// not insertion/scan order.
	mrows, err := h.db.Query(r.Context(), `
		SELECT team1_id, team2_id, team1_points, team2_points
		FROM league_matchups
		WHERE league_id = $1 AND season = $2 AND computed_at IS NOT NULL
		ORDER BY week ASC
	`, leagueID, season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for mrows.Next() {
		var t1, t2 int64
		var p1, p2 float64
		if err := mrows.Scan(&t1, &t2, &p1, &p2); err != nil {
			mrows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		r1, ok1 := records[t1]
		r2, ok2 := records[t2]
		if !ok1 || !ok2 {
			continue
		}
		r1.pointsFor += p1
		r1.pointsAg += p2
		r2.pointsFor += p2
		r2.pointsAg += p1
		switch {
		case p1 > p2:
			r1.wins++
			r2.losses++
			r1.last5 = append(r1.last5, "W")
			r2.last5 = append(r2.last5, "L")
		case p2 > p1:
			r2.wins++
			r1.losses++
			r1.last5 = append(r1.last5, "L")
			r2.last5 = append(r2.last5, "W")
		default:
			r1.ties++
			r2.ties++
			r1.last5 = append(r1.last5, "T")
			r2.last5 = append(r2.last5, "T")
		}
	}
	mrows.Close()

	list := make([]*record, 0, len(records))
	for _, rec := range records {
		list = append(list, rec)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].wins != list[j].wins {
			return list[i].wins > list[j].wins
		}
		return list[i].pointsFor > list[j].pointsFor
	})

	cutoff := playoffCutoff(len(list))
	resp := make([]standingResp, 0, len(list))
	for i, rec := range list {
		total := rec.wins + rec.losses + rec.ties
		pct := "0.000"
		if total > 0 {
			pct = fmt.Sprintf("%.3f", float64(rec.wins)/float64(total))
		}
		last5 := rec.last5
		if len(last5) > 5 {
			last5 = last5[len(last5)-5:]
		}
		resp = append(resp, standingResp{
			TeamID:        rec.teamID,
			Name:          rec.name,
			Rank:          i + 1,
			Wins:          rec.wins,
			Losses:        rec.losses,
			Ties:          rec.ties,
			Percentage:    pct,
			PointsFor:     fmt.Sprintf("%.2f", rec.pointsFor),
			PointsAgainst: fmt.Sprintf("%.2f", rec.pointsAg),
			Last5:         last5,
			MakesPlayoffs: i+1 <= cutoff,
		})
	}
	respondJSON(w, http.StatusOK, resp)
}
