package handlers

import (
	"context"
	"net/http"
	"sort"
	"strconv"
)

type nbaCategoryResult struct {
	Category string  `json:"category"`
	Value    float64 `json:"value"`
	Won      bool    `json:"won"`
}

type nbaMatchupTeamResp struct {
	TeamID        int64               `json:"team_id"`
	Name          string              `json:"name"`
	Categories    []nbaCategoryResult `json:"categories,omitempty"`
	CategoriesWon int                 `json:"categories_won"`
}

type nbaMatchupResp struct {
	Week       int                  `json:"week"`
	Status     string               `json:"status"` // preevent | postevent
	IsPlayoffs string               `json:"is_playoffs"`
	Teams      []nbaMatchupTeamResp `json:"teams"`
}

type nbaScoreboardResp struct {
	Week     int              `json:"week"`
	Matchups []nbaMatchupResp `json:"matchups"`
}

// GetLeagueScoreboardNBA serves a native NBA league's weekly roto matchups —
// the full per-category breakdown per team, not a single point total, so the
// UI can render a real category grid. Defaults to the latest week a
// schedule has been generated for when ?week= is omitted, same convention
// as the NFL scoreboard.
//
// GET /api/leagues/{id}/scoreboard-nba?week=N&season=
func (h *Handler) GetLeagueScoreboardNBA(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
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
			"SELECT COALESCE(MAX(week), 0) FROM league_matchups_nba WHERE league_id = $1 AND season = $2",
			leagueID, season,
		).Scan(&week)
	}
	if week == 0 {
		respondJSON(w, http.StatusOK, nbaScoreboardResp{Matchups: []nbaMatchupResp{}})
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT m.id, m.team1_id, t1.name, m.team2_id, t2.name, m.is_playoffs, m.computed_at IS NOT NULL
		FROM league_matchups_nba m
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
		id                 int64
		t1ID, t2ID         int64
		t1Name, t2Name     string
		isPlayoffs, scored bool
	}
	var scanned []row
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.id, &rw.t1ID, &rw.t1Name, &rw.t2ID, &rw.t2Name, &rw.isPlayoffs, &rw.scored); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		scanned = append(scanned, rw)
	}
	rows.Close()

	resp := nbaScoreboardResp{Week: week, Matchups: []nbaMatchupResp{}}
	for _, rw := range scanned {
		playoffsFlag := "0"
		if rw.isPlayoffs {
			playoffsFlag = "1"
		}
		status := "preevent"
		if rw.scored {
			status = "postevent"
		}
		t1 := nbaMatchupTeamResp{TeamID: rw.t1ID, Name: rw.t1Name}
		t2 := nbaMatchupTeamResp{TeamID: rw.t2ID, Name: rw.t2Name}
		if rw.scored {
			cats, err := h.nbaMatchupCategoryResults(r.Context(), rw.id)
			if err == nil {
				t1.Categories, t1.CategoriesWon = cats[rw.t1ID].results, cats[rw.t1ID].won
				t2.Categories, t2.CategoriesWon = cats[rw.t2ID].results, cats[rw.t2ID].won
			}
		}
		resp.Matchups = append(resp.Matchups, nbaMatchupResp{
			Week: week, Status: status, IsPlayoffs: playoffsFlag,
			Teams: []nbaMatchupTeamResp{t1, t2},
		})
	}
	respondJSON(w, http.StatusOK, resp)
}

type nbaTeamCategorySummary struct {
	results []nbaCategoryResult
	won     int
}

func (h *Handler) nbaMatchupCategoryResults(ctx context.Context, matchupID int64) (map[int64]nbaTeamCategorySummary, error) {
	rows, err := h.db.Query(ctx, `
		SELECT team_id, category, value, COALESCE(won, false)
		FROM league_matchup_nba_categories
		WHERE matchup_id = $1
	`, matchupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTeam := map[int64][]nbaCategoryResult{}
	for rows.Next() {
		var teamID int64
		var cr nbaCategoryResult
		if err := rows.Scan(&teamID, &cr.Category, &cr.Value, &cr.Won); err != nil {
			return nil, err
		}
		byTeam[teamID] = append(byTeam[teamID], cr)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make(map[int64]nbaTeamCategorySummary, len(byTeam))
	for teamID, results := range byTeam {
		sort.Slice(results, func(i, j int) bool {
			return categoryOrder(results[i].Category) < categoryOrder(results[j].Category)
		})
		won := 0
		for _, c := range results {
			if c.Won {
				won++
			}
		}
		out[teamID] = nbaTeamCategorySummary{results: results, won: won}
	}
	return out, nil
}

func categoryOrder(cat string) int {
	for i, c := range nbaCategories {
		if c == cat {
			return i
		}
	}
	return len(nbaCategories)
}

type nbaStandingResp struct {
	TeamID         int64    `json:"team_id"`
	Name           string   `json:"name"`
	Rank           int      `json:"rank"`
	Wins           int      `json:"wins"` // matchups won (majority of categories)
	Losses         int      `json:"losses"`
	Ties           int      `json:"ties"`
	Percentage     string   `json:"percentage"`
	CategoryWins   int      `json:"category_wins"` // cumulative category record — the standard roto tiebreaker
	CategoryLosses int      `json:"category_losses"`
	Last5          []string `json:"last5"`
}

type nbaTeamRecord struct {
	teamID                       int64
	wins, losses, ties           int
	categoryWins, categoryLosses int
	last5                        []string
}

// computeNBATeamRecords aggregates scored league_matchups_nba rows into each
// team's match record (majority of categories won, ties possible if a
// league includes an even number of categories) and cumulative category
// record — pure reads, no separate standings table, same convention as the
// NFL side's computeTeamRecords.
func computeNBATeamRecords(ctx context.Context, db dbtx, leagueID int64, season int, teamIDs []int64) (map[int64]*nbaTeamRecord, error) {
	records := make(map[int64]*nbaTeamRecord, len(teamIDs))
	for _, id := range teamIDs {
		records[id] = &nbaTeamRecord{teamID: id}
	}

	rows, err := db.Query(ctx, `
		SELECT m.id, m.team1_id, m.team2_id
		FROM league_matchups_nba m
		WHERE m.league_id = $1 AND m.season = $2 AND m.computed_at IS NOT NULL
		ORDER BY m.week ASC
	`, leagueID, season)
	if err != nil {
		return nil, err
	}
	var matchupIDs []int64
	type pair struct{ t1, t2 int64 }
	byMatchup := map[int64]pair{}
	for rows.Next() {
		var id, t1, t2 int64
		if err := rows.Scan(&id, &t1, &t2); err != nil {
			rows.Close()
			return nil, err
		}
		matchupIDs = append(matchupIDs, id)
		byMatchup[id] = pair{t1, t2}
	}
	rows.Close()

	for _, mid := range matchupIDs {
		p := byMatchup[mid]
		r1, ok1 := records[p.t1]
		r2, ok2 := records[p.t2]
		if !ok1 || !ok2 {
			continue
		}
		crows, err := db.Query(ctx,
			"SELECT team_id, COALESCE(won, false) FROM league_matchup_nba_categories WHERE matchup_id = $1", mid)
		if err != nil {
			return nil, err
		}
		var t1Won, t2Won int
		for crows.Next() {
			var teamID int64
			var won bool
			if err := crows.Scan(&teamID, &won); err != nil {
				crows.Close()
				return nil, err
			}
			if !won {
				continue
			}
			if teamID == p.t1 {
				t1Won++
			} else if teamID == p.t2 {
				t2Won++
			}
		}
		crows.Close()

		r1.categoryWins += t1Won
		r1.categoryLosses += t2Won
		r2.categoryWins += t2Won
		r2.categoryLosses += t1Won

		switch {
		case t1Won > t2Won:
			r1.wins++
			r2.losses++
			r1.last5 = append(r1.last5, "W")
			r2.last5 = append(r2.last5, "L")
		case t2Won > t1Won:
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
	return records, nil
}

// GetLeagueStandingsNBA aggregates scored league_matchups_nba rows into
// standings — match record first, cumulative category record as the
// tiebreaker (the standard roto-league convention), same "pure read, no
// separate standings table" approach as the NFL side.
//
// GET /api/leagues/{id}/standings-nba?season=
func (h *Handler) GetLeagueStandingsNBA(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	season := h.leagueSeasonInt(r, leagueID)
	if s := r.URL.Query().Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	names := map[int64]string{}
	var teamIDs []int64
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
		names[id] = name
		teamIDs = append(teamIDs, id)
	}
	trows.Close()

	records, err := computeNBATeamRecords(r.Context(), h.db, leagueID, season, teamIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]nbaStandingResp, 0, len(teamIDs))
	for _, id := range teamIDs {
		rec := records[id]
		games := rec.wins + rec.losses + rec.ties
		pct := 0.0
		if games > 0 {
			pct = (float64(rec.wins) + 0.5*float64(rec.ties)) / float64(games)
		}
		last5 := rec.last5
		if len(last5) > 5 {
			last5 = last5[len(last5)-5:]
		}
		resp = append(resp, nbaStandingResp{
			TeamID: id, Name: names[id], Wins: rec.wins, Losses: rec.losses, Ties: rec.ties,
			Percentage:     strconv.FormatFloat(pct, 'f', 3, 64),
			CategoryWins:   rec.categoryWins,
			CategoryLosses: rec.categoryLosses,
			Last5:          last5,
		})
	}
	sort.Slice(resp, func(i, j int) bool {
		pi, _ := strconv.ParseFloat(resp[i].Percentage, 64)
		pj, _ := strconv.ParseFloat(resp[j].Percentage, 64)
		if pi != pj {
			return pi > pj
		}
		return resp[i].CategoryWins > resp[j].CategoryWins
	})
	for i := range resp {
		resp[i].Rank = i + 1
	}
	respondJSON(w, http.StatusOK, resp)
}
