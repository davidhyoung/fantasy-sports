package handlers

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

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
	Projection   *nbaProjectionResp    `json:"projection"`
	// Darko is nil unless the player has an imported DARKO projection for the
	// default season (cmd/project-nba -import-darko / -import-darko-extras).
	Darko *nbaDarkoDetailResp `json:"darko"`
}

type nbaDarkoDetailResp struct {
	Minutes      float64         `json:"minutes"` // DARKO's projected minutes per game
	SnapshotDate string          `json:"snapshot_date"`
	Comps        json.RawMessage `json:"comps"` // nearest historical player-seasons at the same age
	Trend        json.RawMessage `json:"trend"` // season-end DPM (DARKO ending-year seasons)
}

type nbaPlayerSeasonResp struct {
	Season int                `json:"season"`
	Team   string             `json:"team"`
	Games  int                `json:"games"`
	Stats  []nbaCategoryEntry `json:"stats"`
}

type nbaPlayerCompResp struct {
	ESPNID string `json:"espn_id"`
	Name   string `json:"name"`
	Season int    `json:"season"`
	Team   string `json:"team"`
	Games  int    `json:"games"`
	// Age at the matched season, when known (0 = unknown) — shown so a
	// comp's career-stage resemblance (see nbaCareerStageOrdinal) is
	// legible, not just asserted.
	Age   int                `json:"age"`
	Stats []nbaCategoryEntry `json:"stats"`
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
	espnID    string
	name      string
	season    int
	team      string
	games     int
	height    int // inches, 0 = unknown
	weight    int // pounds, 0 = unknown
	birthYear int // 0 = unknown
	values    map[string]float64
}

// nbaCareerStageOrdinal buckets a player's age at a season into young (0) /
// prime (1) / post-prime (2) — a coarse, explicit stand-in for "where are
// they in their career arc" that's more legible than raw age and groups
// naturally with the ordinal-distance treatment below. 0 (unknown age) maps
// to prime — not a claim about the player, just the least-biased default
// when the comp pool's real distribution is unknown.
func nbaCareerStageOrdinal(age float64) float64 {
	switch {
	case age <= 0:
		return 1
	case age <= 24:
		return 0
	case age <= 30:
		return 1
	default:
		return 2
	}
}

// nbaPlayerComps finds the `limit` historical player-seasons that most
// resemble the target player's own best-sample season (the one with the
// most games played, so a two-game cameo doesn't define them) — weighted
// Euclidean distance over three dimension groups, each averaged to a single
// z-score before distance (so adding categories to the statline group
// doesn't dilute its weight — same shape nbaPlayerProjection uses):
//
//	statline (9 per-game categories, weight 2.0)
//	physical (height, weight, weight 1.0)
//	career stage (young/prime/post-prime ordinal from age, weight 1.0)
//
// Restricted to the same ESPN position bucket (G/F/C) since a point guard's
// comps should be other guards, not a center who happens to score
// similarly. Unlike the fantasy-value z-scores elsewhere in this app,
// turnovers is NOT inverted here — this measures statistical resemblance,
// not fantasy value, so a high-turnover playmaker should read as similar to
// another high-turnover playmaker. Self-excluded (a player is never their
// own comp). Candidates missing height/weight/birth_date are excluded
// entirely (keeps the physical/stage z-score basis clean); if the *target*
// is missing any of them, those groups are dropped from the distance
// formula rather than failing the whole request.
func (h *Handler) nbaPlayerComps(
	ctx context.Context, espnID, position string, height, weight int, birthDateStr string,
	seasons []nbaPlayerSeasonResp, limit int,
) ([]nbaPlayerCompResp, error) {
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
	targetBirthYear := birthYearFromDate(birthDateStr)
	targetAge := 0.0
	if targetBirthYear > 0 {
		targetAge = float64(target.Season - targetBirthYear)
	}
	havePhysical := height > 0 && weight > 0

	rows, err := h.db.Query(ctx, `
		SELECT ps.espn_id, p.name, ps.season, (array_agg(ps.team ORDER BY ps.game_date DESC))[1], COUNT(*),
		       p.height, p.weight, COALESCE(EXTRACT(YEAR FROM p.birth_date), 0),
		       COALESCE(SUM(ps.points),0), COALESCE(SUM(ps.rebounds),0), COALESCE(SUM(ps.assists),0),
		       COALESCE(SUM(ps.steals),0), COALESCE(SUM(ps.blocks),0), COALESCE(SUM(ps.turnovers),0),
		       COALESCE(SUM(ps.fg3_made),0),
		       COALESCE(SUM(ps.fg_made),0), COALESCE(SUM(ps.fg_att),0),
		       COALESCE(SUM(ps.ft_made),0), COALESCE(SUM(ps.ft_att),0)
		FROM nba_player_stats ps
		JOIN nba_players p ON p.espn_id = ps.espn_id
		WHERE p.position = $1 AND ps.espn_id != $2
		  AND p.height IS NOT NULL AND p.weight IS NOT NULL AND p.birth_date IS NOT NULL
		GROUP BY ps.espn_id, ps.season, p.name, p.height, p.weight, p.birth_date
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
		var byYear float64
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&c.espnID, &c.name, &c.season, &c.team, &games, &c.height, &c.weight, &byYear,
			&pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		c.games = games
		c.birthYear = int(byYear)
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

	candAge := func(c nbaCompCandidate) float64 {
		if c.birthYear == 0 {
			return 0
		}
		return float64(c.season - c.birthYear)
	}

	// z-score basis spans the candidate pool plus the target itself, so the
	// target's own value is never measured against a population it wasn't
	// part of.
	type stat struct{ mean, stdev float64 }
	n := float64(len(candidates)) + 1
	zBasis := func(target float64, pick func(nbaCompCandidate) float64) stat {
		sum := target
		for _, c := range candidates {
			sum += pick(c)
		}
		mean := sum / n
		d := target - mean
		varSum := d * d
		for _, c := range candidates {
			d := pick(c) - mean
			varSum += d * d
		}
		return stat{mean: mean, stdev: math.Sqrt(varSum / n)}
	}
	zOf := func(v float64, s stat) float64 {
		if s.stdev == 0 {
			return 0
		}
		return (v - s.mean) / s.stdev
	}

	statline := make(map[string]stat, len(nbaCategories))
	for _, cat := range nbaCategories {
		statline[cat] = zBasis(targetValues[cat], func(c nbaCompCandidate) float64 { return c.values[cat] })
	}
	heightStat := zBasis(float64(height), func(c nbaCompCandidate) float64 { return float64(c.height) })
	weightStat := zBasis(float64(weight), func(c nbaCompCandidate) float64 { return float64(c.weight) })
	stageStat := zBasis(nbaCareerStageOrdinal(targetAge), func(c nbaCompCandidate) float64 { return nbaCareerStageOrdinal(candAge(c)) })

	avgStatlineZ := func(values map[string]float64) float64 {
		var sum float64
		for _, cat := range nbaCategories {
			sum += zOf(values[cat], statline[cat])
		}
		return sum / float64(len(nbaCategories))
	}
	physicalZ := func(h, w int) float64 {
		return (zOf(float64(h), heightStat) + zOf(float64(w), weightStat)) / 2
	}

	const (
		wStatline = 2.0
		wPhysical = 1.0
		wStage    = 1.0
	)
	targetGroups := [3]float64{avgStatlineZ(targetValues), 0, 0}
	groupWeight := [3]float64{wStatline, 0, 0}
	if havePhysical {
		targetGroups[1] = physicalZ(height, weight)
		groupWeight[1] = wPhysical
	}
	if targetBirthYear > 0 {
		targetGroups[2] = zOf(nbaCareerStageOrdinal(targetAge), stageStat)
		groupWeight[2] = wStage
	}
	var totalWeight float64
	for _, w := range groupWeight {
		totalWeight += w
	}

	type scoredCandidate struct {
		c    nbaCompCandidate
		dist float64
	}
	scored := make([]scoredCandidate, 0, len(candidates))
	for _, c := range candidates {
		groups := [3]float64{
			avgStatlineZ(c.values),
			physicalZ(c.height, c.weight),
			zOf(nbaCareerStageOrdinal(candAge(c)), stageStat),
		}
		var weighted float64
		for i := range groups {
			d := targetGroups[i] - groups[i]
			weighted += groupWeight[i] * d * d
		}
		scored = append(scored, scoredCandidate{c: c, dist: math.Sqrt(weighted / totalWeight)})
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
		age := 0
		if s.c.birthYear > 0 {
			age = s.c.season - s.c.birthYear
		}
		out = append(out, nbaPlayerCompResp{
			ESPNID: s.c.espnID, Name: s.c.name, Season: s.c.season, Team: s.c.team, Games: s.c.games, Age: age, Stats: entries,
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

	comps, err := h.nbaPlayerComps(r.Context(), espnID, p.Position, p.Height, p.Weight, p.BirthDate, seasons, 5)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Comps = comps

	projection, err := h.nbaPlayerProjection(r.Context(), espnID, p.Position, p.BirthDate, seasons)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	p.Projection = projection

	p.Darko = h.nbaDarkoDetail(r.Context(), espnID)

	respondJSON(w, http.StatusOK, p)
}

// nbaDarkoDetail loads the DARKO comps/trend for a player, or nil when the
// player has no DARKO row for the default season. Errors degrade to nil —
// DARKO is supplementary to the rest of the detail response.
func (h *Handler) nbaDarkoDetail(ctx context.Context, espnID string) *nbaDarkoDetailResp {
	var d nbaDarkoDetailResp
	var snap time.Time
	var comps, trend []byte
	err := h.db.QueryRow(ctx, `
		SELECT x_minutes, snapshot_date, comps, trend FROM nba_darko_projections
		WHERE espn_id = $1 AND season = $2
	`, espnID, h.config.DefaultSeason).Scan(&d.Minutes, &snap, &comps, &trend)
	if err != nil {
		return nil
	}
	d.SnapshotDate = snap.Format("2006-01-02")
	if len(comps) == 0 {
		comps = []byte("[]")
	}
	if len(trend) == 0 {
		trend = []byte("[]")
	}
	d.Comps, d.Trend = comps, trend
	return &d
}
