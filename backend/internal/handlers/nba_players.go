package handlers

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type nbaPlayerDetailResp struct {
	ESPNID       string                `json:"espn_id"`
	Name         string                `json:"name"`
	Position     string                `json:"position"`
	Team         string                `json:"team"`
	HeadshotURL  string                `json:"headshot_url"`
	BirthDate    string                `json:"birth_date"`
	Height       int                   `json:"height"` // inches
	Weight       int                   `json:"weight"` // pounds
	College      string                `json:"college"`
	YearsExp     int                   `json:"years_exp"`
	JerseyNumber int                   `json:"jersey_number"`
	Stats        []nbaCategoryEntry    `json:"stats"`
	Seasons      []nbaPlayerSeasonResp `json:"seasons"`
	Comps        []nbaPlayerCompResp   `json:"comps"`
}

type nbaPlayerSeasonResp struct {
	Season int                `json:"season"`
	Team   string             `json:"team"`
	Games  int                `json:"games"`
	Stats  []nbaCategoryEntry `json:"stats"`
}

type nbaPlayerCompResp struct {
	ESPNID string             `json:"espn_id"`
	Name   string             `json:"name"`
	Season int                `json:"season"`
	Team   string             `json:"team"`
	Games  int                `json:"games"`
	Stats  []nbaCategoryEntry `json:"stats"`
}

// nbaPlayerSeasons returns one row per real NBA season this player has any
// stats for — a career table, independent of the detail view's period
// picker above (which answers "how's this player doing lately"; this
// answers "what has their whole career looked like"). Per-game averages for
// counting categories (same convention as period "season_avg"); fg_pct/
// ft_pct are always a ratio of aggregated makes/attempts, never averaged.
// `team` is whichever team their most recent game that season was with —
// good enough for a compact table; a mid-season trade isn't split into two
// rows the way some career-stat sites do.
func (h *Handler) nbaPlayerSeasons(ctx context.Context, espnID string) ([]nbaPlayerSeasonResp, error) {
	rows, err := h.db.Query(ctx, `
		SELECT season, (array_agg(team ORDER BY game_date DESC))[1], COUNT(*),
		       COALESCE(SUM(points),0), COALESCE(SUM(rebounds),0), COALESCE(SUM(assists),0),
		       COALESCE(SUM(steals),0), COALESCE(SUM(blocks),0), COALESCE(SUM(turnovers),0),
		       COALESCE(SUM(fg3_made),0),
		       COALESCE(SUM(fg_made),0), COALESCE(SUM(fg_att),0),
		       COALESCE(SUM(ft_made),0), COALESCE(SUM(ft_att),0)
		FROM nba_player_stats
		WHERE espn_id = $1
		GROUP BY season
		ORDER BY season DESC
	`, espnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []nbaPlayerSeasonResp
	for rows.Next() {
		var s nbaPlayerSeasonResp
		var team string
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&s.Season, &team, &s.Games, &pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		s.Team = team
		divisor := 1.0
		if s.Games > 0 {
			divisor = float64(s.Games)
		}
		totals := map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
		s.Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
		for _, c := range nbaCategories {
			s.Stats = append(s.Stats, nbaCategoryEntry{Category: c, Value: totals[c]})
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

type nbaCompCandidate struct {
	espnID string
	name   string
	season int
	team   string
	games  int
	values map[string]float64
}

// nbaPlayerComps finds the `limit` historical player-seasons whose per-game
// stat line most resembles the target player's own best-sample season (the
// one with the most games played, so a two-game cameo doesn't define them)
// — Euclidean distance over z-scored categories, restricted to the same
// ESPN position bucket (G/F/C) since a point guard's comps should be other
// guards, not a center who happens to score similarly. Unlike the
// fantasy-value z-scores elsewhere in this app, turnovers is NOT inverted
// here — this measures statistical resemblance, not fantasy value, so a
// high-turnover playmaker should read as similar to another high-turnover
// playmaker. Self-excluded (a player is never their own comp).
func (h *Handler) nbaPlayerComps(ctx context.Context, espnID, position string, seasons []nbaPlayerSeasonResp, limit int) ([]nbaPlayerCompResp, error) {
	if position == "" || len(seasons) == 0 {
		return nil, nil
	}
	target := seasons[0]
	for _, s := range seasons {
		if s.Games > target.Games {
			target = s
		}
	}
	targetValues := make(map[string]float64, len(nbaCategories))
	for _, e := range target.Stats {
		targetValues[e.Category] = e.Value
	}

	rows, err := h.db.Query(ctx, `
		SELECT ps.espn_id, p.name, ps.season, (array_agg(ps.team ORDER BY ps.game_date DESC))[1], COUNT(*),
		       COALESCE(SUM(ps.points),0), COALESCE(SUM(ps.rebounds),0), COALESCE(SUM(ps.assists),0),
		       COALESCE(SUM(ps.steals),0), COALESCE(SUM(ps.blocks),0), COALESCE(SUM(ps.turnovers),0),
		       COALESCE(SUM(ps.fg3_made),0),
		       COALESCE(SUM(ps.fg_made),0), COALESCE(SUM(ps.fg_att),0),
		       COALESCE(SUM(ps.ft_made),0), COALESCE(SUM(ps.ft_att),0)
		FROM nba_player_stats ps
		JOIN nba_players p ON p.espn_id = ps.espn_id
		WHERE p.position = $1 AND ps.espn_id != $2
		GROUP BY ps.espn_id, ps.season, p.name
		HAVING COUNT(*) >= 20
	`, position, espnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []nbaCompCandidate
	for rows.Next() {
		var c nbaCompCandidate
		var games int
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&c.espnID, &c.name, &c.season, &c.team, &games,
			&pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		c.games = games
		divisor := float64(games)
		c.values = map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// z-score basis spans the candidate pool plus the target itself, so the
	// target's own value is never measured against a population it wasn't
	// part of.
	type catStat struct{ mean, stdev float64 }
	statsByCat := make(map[string]catStat, len(nbaCategories))
	for _, cat := range nbaCategories {
		n := float64(len(candidates)) + 1
		sum := targetValues[cat]
		for _, c := range candidates {
			sum += c.values[cat]
		}
		mean := sum / n
		d := targetValues[cat] - mean
		varSum := d * d
		for _, c := range candidates {
			d := c.values[cat] - mean
			varSum += d * d
		}
		statsByCat[cat] = catStat{mean: mean, stdev: math.Sqrt(varSum / n)}
	}

	zVector := func(values map[string]float64) []float64 {
		z := make([]float64, len(nbaCategories))
		for i, cat := range nbaCategories {
			s := statsByCat[cat]
			if s.stdev == 0 {
				continue
			}
			z[i] = (values[cat] - s.mean) / s.stdev
		}
		return z
	}

	targetZ := zVector(targetValues)
	type scoredCandidate struct {
		c    nbaCompCandidate
		dist float64
	}
	scored := make([]scoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		cz := zVector(c.values)
		var sumSq float64
		for i := range targetZ {
			d := targetZ[i] - cz[i]
			sumSq += d * d
		}
		scored = append(scored, scoredCandidate{c: c, dist: math.Sqrt(sumSq)})
	}
	sort.Slice(scored, func(i, j int) bool { return scored[i].dist < scored[j].dist })
	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := make([]nbaPlayerCompResp, 0, len(scored))
	for _, s := range scored {
		entries := make([]nbaCategoryEntry, 0, len(nbaCategories))
		for _, cat := range nbaCategories {
			entries = append(entries, nbaCategoryEntry{Category: cat, Value: s.c.values[cat]})
		}
		out = append(out, nbaPlayerCompResp{
			ESPNID: s.c.espnID, Name: s.c.name, Season: s.c.season, Team: s.c.team, Games: s.c.games, Stats: entries,
		})
	}
	return out, nil
}

// GetNBAPlayer returns a single NBA player's metadata plus aggregated
// category totals for the requested period (same vocabulary as the
// Players/Roster tabs' timeframe picker — see parseNBAStatPeriod) across
// every canonical category, unfiltered by any league's relevant-category
// scoring, since this is a standalone detail view rather than a
// league-scoped table. `league_id`, when given, resolves which real season
// "season"/"last_season" mean (that league's own season); omitted, it falls
// back to the server's configured default season.
//
// GET /api/nba/players/{espnId}?period=&league_id=
func (h *Handler) GetNBAPlayer(w http.ResponseWriter, r *http.Request) {
	espnID := chi.URLParam(r, "espnId")
	if espnID == "" {
		respondError(w, http.StatusBadRequest, "invalid player id")
		return
	}

	var p nbaPlayerDetailResp
	err := h.db.QueryRow(r.Context(), `
		SELECT espn_id, name, COALESCE(position, ''), COALESCE(team, ''), COALESCE(headshot_url, ''),
		       COALESCE(to_char(birth_date, 'YYYY-MM-DD'), ''), COALESCE(height, 0), COALESCE(weight, 0),
		       COALESCE(college, ''), COALESCE(years_exp, 0), COALESCE(jersey_number, 0)
		FROM nba_players WHERE espn_id = $1
	`, espnID).Scan(&p.ESPNID, &p.Name, &p.Position, &p.Team, &p.HeadshotURL,
		&p.BirthDate, &p.Height, &p.Weight, &p.College, &p.YearsExp, &p.JerseyNumber)
	if err != nil {
		respondError(w, http.StatusNotFound, "player not found")
		return
	}

	season := h.config.DefaultSeason
	if leagueIDStr := r.URL.Query().Get("league_id"); leagueIDStr != "" {
		if leagueID, err := strconv.ParseInt(leagueIDStr, 10, 64); err == nil {
			season = h.leagueSeasonInt(r, leagueID)
		}
	}
	period := parseNBAStatPeriod(r)

	totals, err := h.nbaRawCategoryTotals(r.Context(), season, period, []string{espnID})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	playerTotals := totals[espnID]
	p.Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
	for _, c := range nbaCategories {
		p.Stats = append(p.Stats, nbaCategoryEntry{Category: c, Value: playerTotals[c]})
	}

	seasons, err := h.nbaPlayerSeasons(r.Context(), espnID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Seasons = seasons

	comps, err := h.nbaPlayerComps(r.Context(), espnID, p.Position, seasons, 5)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Comps = comps

	respondJSON(w, http.StatusOK, p)
}
