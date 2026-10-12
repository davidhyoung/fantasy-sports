package handlers

import (
	"context"
	"math"
	"sort"
	"strconv"
)

type nbaProjectionComp struct {
	ESPNID      string             `json:"espn_id"`
	Name        string             `json:"name"`
	MatchSeason int                `json:"match_season"`
	NextSeason  int                `json:"next_season"`
	Similarity  float64            `json:"similarity"`
	MatchStats  []nbaCategoryEntry `json:"match_stats"`
	NextStats   []nbaCategoryEntry `json:"next_stats"`
}

type nbaProjectionResp struct {
	// The season being projected (BaseSeason + 1) and the one actually used
	// as the growth starting point — usually the player's most recent
	// season, but see minProjectionBaseGames below.
	BaseSeason   int                 `json:"base_season"`
	TargetSeason int                 `json:"target_season"`
	Stats        []nbaCategoryEntry  `json:"stats"`
	CompCount    int                 `json:"comp_count"`
	Comps        []nbaProjectionComp `json:"comps"`
}

type nbaProjCandidate struct {
	espnID    string
	name      string
	birthYear int // 0 = unknown
	season    int
	games     int
	values    map[string]float64
}

// minProjectionBaseGames guards against growing a projection forward from a
// tiny-sample season (e.g. a 2-game cameo at the start of the current
// season) — below this, the player's best-sample season is used as the
// growth starting point instead of their nominally "most recent" one.
const minProjectionBaseGames = 10

func birthYearFromDate(birthDate string) int {
	if len(birthDate) < 4 {
		return 0
	}
	y, err := strconv.Atoi(birthDate[:4])
	if err != nil {
		return 0
	}
	return y
}

// nbaPlayerProjection computes a comp-based next-season per-game projection
// — an NBA analog of the NFL comp-based system (docs/projection-algorithm.md),
// deliberately scoped down for where NBA data is today: computed on demand
// (no offline batch pipeline, no persisted table), no shrinkage, no grade
// dimension, no rookie/draft-slot path, no backtest/autotune, no outcome
// distribution.
//
// Comps are matched on three dimension groups — statline (the 9 canonical
// categories), age, and year-over-year trajectory (this-season-minus-prior
// per category) — group z-scores averaged before distance, same shape the
// NFL system uses so adding categories to a group doesn't dilute its
// weight. Only candidates with an observable *next* season are eligible at
// all, since growth is "what did comps like this do the following year" —
// a candidate with no next-season row is dropped rather than scored as a
// zero-growth washout (unlike NFL): the historical backfill is still in
// progress, so "no row yet" doesn't reliably mean "left the league." If
// every comp drops out, the projection flatly repeats the target's own
// rate (growth = 1.0).
func (h *Handler) nbaPlayerProjection(ctx context.Context, espnID, position, birthDateStr string, seasons []nbaPlayerSeasonResp) (*nbaProjectionResp, error) {
	if position == "" || len(seasons) == 0 {
		return nil, nil
	}

	base := seasons[0]
	if base.Games < minProjectionBaseGames {
		for _, s := range seasons {
			if s.Games > base.Games {
				base = s
			}
		}
	}
	baseValues := nbaStatsMap(base.Stats)

	var basePrior *nbaPlayerSeasonResp
	for i := range seasons {
		if seasons[i].Season == base.Season-1 {
			basePrior = &seasons[i]
			break
		}
	}
	baseTrajectory := make(map[string]float64, len(nbaCategories))
	for _, cat := range nbaCategories {
		if basePrior != nil {
			baseTrajectory[cat] = baseValues[cat] - nbaStatsMap(basePrior.Stats)[cat]
		}
	}
	baseBirthYear := birthYearFromDate(birthDateStr)
	baseAge := 0.0
	if baseBirthYear > 0 {
		baseAge = float64(base.Season - baseBirthYear)
	}

	rows, err := h.db.Query(ctx, `
		SELECT ps.espn_id, p.name, COALESCE(EXTRACT(YEAR FROM p.birth_date), 0), ps.season, COUNT(*),
		       COALESCE(SUM(ps.points),0), COALESCE(SUM(ps.rebounds),0), COALESCE(SUM(ps.assists),0),
		       COALESCE(SUM(ps.steals),0), COALESCE(SUM(ps.blocks),0), COALESCE(SUM(ps.turnovers),0),
		       COALESCE(SUM(ps.fg3_made),0),
		       COALESCE(SUM(ps.fg_made),0), COALESCE(SUM(ps.fg_att),0),
		       COALESCE(SUM(ps.ft_made),0), COALESCE(SUM(ps.ft_att),0)
		FROM nba_player_stats ps
		JOIN nba_players p ON p.espn_id = ps.espn_id
		WHERE p.position = $1
		GROUP BY ps.espn_id, p.name, p.birth_date, ps.season
		HAVING COUNT(*) >= 20
	`, position)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// bySeason[espnID][season] holds every qualifying (espn_id, season) in
	// the position pool, so next/prior-season lookups are a map hit rather
	// than another query.
	bySeason := map[string]map[int]*nbaProjCandidate{}
	for rows.Next() {
		var c nbaProjCandidate
		var byYear float64
		var games int
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&c.espnID, &c.name, &byYear, &c.season, &games,
			&pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		c.birthYear = int(byYear)
		c.games = games
		divisor := float64(games)
		c.values = map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
		if bySeason[c.espnID] == nil {
			bySeason[c.espnID] = map[int]*nbaProjCandidate{}
		}
		cc := c
		bySeason[c.espnID][c.season] = &cc
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	type candidate struct {
		cur, next, prior *nbaProjCandidate
		age, trajectory  map[string]float64
	}
	var pool []candidate
	for pid, seasonMap := range bySeason {
		if pid == espnID {
			continue
		}
		for s, cur := range seasonMap {
			next, ok := seasonMap[s+1]
			if !ok {
				continue // no observable growth — excluded, not zero-scored
			}
			prior := seasonMap[s-1]
			traj := make(map[string]float64, len(nbaCategories))
			if prior != nil {
				for _, cat := range nbaCategories {
					traj[cat] = cur.values[cat] - prior.values[cat]
				}
			}
			pool = append(pool, candidate{cur: cur, next: next, prior: prior, trajectory: traj})
		}
	}
	if len(pool) == 0 {
		return &nbaProjectionResp{BaseSeason: base.Season, TargetSeason: base.Season + 1, Stats: base.Stats, CompCount: 0}, nil
	}

	ageOf := func(c *nbaProjCandidate) float64 {
		if c.birthYear == 0 {
			return 0
		}
		return float64(c.season - c.birthYear)
	}

	// z-score basis spans the candidate pool plus the target's own base
	// season, same "never measured against a population it wasn't part of"
	// convention as nbaPlayerComps.
	type catStat struct{ mean, stdev float64 }
	n := float64(len(pool)) + 1
	statline := make(map[string]catStat, len(nbaCategories))
	trajectory := make(map[string]catStat, len(nbaCategories))
	for _, cat := range nbaCategories {
		sum := baseValues[cat]
		for _, c := range pool {
			sum += c.cur.values[cat]
		}
		mean := sum / n
		d := baseValues[cat] - mean
		varSum := d * d
		for _, c := range pool {
			d := c.cur.values[cat] - mean
			varSum += d * d
		}
		statline[cat] = catStat{mean: mean, stdev: math.Sqrt(varSum / n)}

		sumT := baseTrajectory[cat]
		for _, c := range pool {
			sumT += c.trajectory[cat]
		}
		meanT := sumT / n
		dT := baseTrajectory[cat] - meanT
		varSumT := dT * dT
		for _, c := range pool {
			dT := c.trajectory[cat] - meanT
			varSumT += dT * dT
		}
		trajectory[cat] = catStat{mean: meanT, stdev: math.Sqrt(varSumT / n)}
	}
	sumAge := baseAge
	for _, c := range pool {
		sumAge += ageOf(c.cur)
	}
	meanAge := sumAge / n
	dAge := baseAge - meanAge
	varAge := dAge * dAge
	for _, c := range pool {
		d := ageOf(c.cur) - meanAge
		varAge += d * d
	}
	ageStdev := math.Sqrt(varAge / n)

	avgZ := func(values map[string]float64, stats map[string]catStat) float64 {
		var sum float64
		for _, cat := range nbaCategories {
			s := stats[cat]
			if s.stdev == 0 {
				continue
			}
			sum += (values[cat] - s.mean) / s.stdev
		}
		return sum / float64(len(nbaCategories))
	}
	ageZ := func(age float64) float64 {
		if ageStdev == 0 {
			return 0
		}
		return (age - meanAge) / ageStdev
	}

	// Dimension groups, weighted and averaged before distance — adding more
	// categories to the statline group doesn't dilute its weight relative
	// to the single-value age group.
	const (
		wStatline   = 2.0
		wTrajectory = 1.0
		wAge        = 1.0
	)
	baseGroups := [3]float64{avgZ(baseValues, statline), avgZ(baseTrajectory, trajectory), ageZ(baseAge)}

	type scored struct {
		c    candidate
		dist float64
	}
	scoredPool := make([]scored, 0, len(pool))
	for _, c := range pool {
		groups := [3]float64{avgZ(c.cur.values, statline), avgZ(c.trajectory, trajectory), ageZ(ageOf(c.cur))}
		d0, d1, d2 := baseGroups[0]-groups[0], baseGroups[1]-groups[1], baseGroups[2]-groups[2]
		weighted := wStatline*d0*d0 + wTrajectory*d1*d1 + wAge*d2*d2
		dist := math.Sqrt(weighted / (wStatline + wTrajectory + wAge))
		scoredPool = append(scoredPool, scored{c: c, dist: dist})
	}
	sort.Slice(scoredPool, func(i, j int) bool { return scoredPool[i].dist < scoredPool[j].dist })

	const maxComps = 15
	if len(scoredPool) > maxComps {
		scoredPool = scoredPool[:maxComps]
	}

	// growth_i = next/match per counting category (clamped), next-match
	// delta for rate categories (clamped) — same shape as NFL's growth
	// ratio, split by stat type since a ratio makes no sense for a bounded
	// percentage.
	type growthEntry struct {
		sim    float64
		growth map[string]float64 // counting cats: multiplier; pct cats: additive delta
	}
	growths := make([]growthEntry, 0, len(scoredPool))
	for _, s := range scoredPool {
		sim := 1 / (1 + s.dist)
		g := make(map[string]float64, len(nbaCategories))
		for _, cat := range nbaCategories {
			mv, nv := s.c.cur.values[cat], s.c.next.values[cat]
			if cat == "fg_pct" || cat == "ft_pct" {
				g[cat] = math.Max(-0.3, math.Min(0.3, nv-mv))
				continue
			}
			if mv < 0.5 {
				g[cat] = 1.0
				continue
			}
			g[cat] = math.Max(0.3, math.Min(2.5, nv/mv))
		}
		growths = append(growths, growthEntry{sim: sim, growth: g})
	}

	projected := make(map[string]float64, len(nbaCategories))
	compsOut := make([]nbaProjectionComp, 0, len(scoredPool))
	if len(growths) == 0 {
		for _, cat := range nbaCategories {
			projected[cat] = baseValues[cat]
		}
	} else {
		var sumSimSq float64
		for _, g := range growths {
			sumSimSq += g.sim * g.sim
		}
		for _, cat := range nbaCategories {
			var acc float64
			for _, g := range growths {
				w := g.sim * g.sim / sumSimSq
				acc += w * g.growth[cat]
			}
			if cat == "fg_pct" || cat == "ft_pct" {
				projected[cat] = math.Max(0, math.Min(1, baseValues[cat]+acc))
			} else {
				projected[cat] = math.Max(0, baseValues[cat]*acc)
			}
		}
		for i, s := range scoredPool {
			compsOut = append(compsOut, nbaProjectionComp{
				ESPNID: s.c.cur.espnID, Name: s.c.cur.name,
				MatchSeason: s.c.cur.season, NextSeason: s.c.next.season,
				Similarity: growths[i].sim,
				MatchStats: nbaEntriesFromMap(s.c.cur.values), NextStats: nbaEntriesFromMap(s.c.next.values),
			})
		}
	}

	entries := nbaEntriesFromMap(projected)
	return &nbaProjectionResp{
		BaseSeason: base.Season, TargetSeason: base.Season + 1,
		Stats: entries, CompCount: len(growths), Comps: compsOut,
	}, nil
}

func nbaStatsMap(entries []nbaCategoryEntry) map[string]float64 {
	m := make(map[string]float64, len(entries))
	for _, e := range entries {
		m[e.Category] = e.Value
	}
	return m
}

func nbaEntriesFromMap(values map[string]float64) []nbaCategoryEntry {
	entries := make([]nbaCategoryEntry, 0, len(nbaCategories))
	for _, cat := range nbaCategories {
		entries = append(entries, nbaCategoryEntry{Category: cat, Value: values[cat]})
	}
	return entries
}
