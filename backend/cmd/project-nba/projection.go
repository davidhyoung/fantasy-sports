// projection.go is a batch, backtestable analog of
// internal/handlers/nba_projections.go's nbaPlayerProjection — same comp
// distance math (statline/trajectory/age dimension groups, growth clamps),
// but run once for every eligible player at once (not per-request) and
// parameterized by an explicit asOfSeason cutoff so a historical backtest can
// restrict the candidate pool to "only seasons before the one being
// projected," which a live per-request handler has no reason to do.
//
// This file deliberately duplicates rather than imports from
// internal/handlers: those types/helpers are unexported, and the live
// per-player projection path is left untouched (see docs/nba-projection-algorithm.md).
package main

import (
	"context"
	"math"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nbaCategories is the fixed roto category vocabulary — mirrors
// internal/handlers/league_rosters_nba.go's nbaCategories exactly.
var nbaCategories = []string{
	"points", "rebounds", "assists", "steals", "blocks", "turnovers", "three_pm", "fg_pct", "ft_pct",
}

func nbaRatio(made, att float64) float64 {
	if att == 0 {
		return 0
	}
	return made / att
}

// projConfig holds the tunable knobs Phase 2's backtest/autotune sweeps.
// Defaults reproduce the live handler's fixed constants exactly — loading
// this config and running with every field at its default is a no-op
// relative to today's shipped per-player projection.
type projConfig struct {
	WeightStatline   float64 `json:"weight_statline"`
	WeightTrajectory float64 `json:"weight_trajectory"`
	WeightAge        float64 `json:"weight_age"`
	// GrowthShrinkageK pulls a comp-derived growth rate toward an
	// age/position growth baseline when the comp pool is thin, weighted as a
	// pseudo-comp of this size. 0 = exact no-op (no such term is added).
	GrowthShrinkageK float64 `json:"growth_shrinkage_k"`
	// RateStatShrinkageK shrinks small-sample FG%/FT% toward the position
	// mean before z-scoring/distance, weighted by attempts vs this constant.
	// 0 = exact no-op.
	RateStatShrinkageK float64 `json:"rate_stat_shrinkage_k"`
}

func defaultProjConfig() projConfig {
	return projConfig{WeightStatline: 2.0, WeightTrajectory: 1.0, WeightAge: 1.0}
}

const (
	minProjectionBaseGames = 10 // below this, fall back to the player's best-sample season
	minCandidateGames      = 20 // comp candidates need a real sample, same floor as the live handler
	maxComps               = 15
)

type seasonAgg struct {
	espnID    string
	name      string
	position  string
	birthYear int
	season    int
	games     int
	values    map[string]float64
	attFG     float64
	attFT     float64
}

// loadSeasonAggregates aggregates nba_player_stats into one row per
// (player, season). asOfSeason > 0 restricts to seasons strictly before it —
// the backtester's temporal-integrity cutoff; asOfSeason == 0 means "no
// cutoff," used by the live `-project` run.
func loadSeasonAggregates(ctx context.Context, pool *pgxpool.Pool, asOfSeason int) (map[string]map[int]*seasonAgg, error) {
	query := `
		SELECT ps.espn_id, p.name, p.position, COALESCE(EXTRACT(YEAR FROM p.birth_date), 0),
		       ps.season, COUNT(*),
		       COALESCE(SUM(ps.points),0), COALESCE(SUM(ps.rebounds),0), COALESCE(SUM(ps.assists),0),
		       COALESCE(SUM(ps.steals),0), COALESCE(SUM(ps.blocks),0), COALESCE(SUM(ps.turnovers),0),
		       COALESCE(SUM(ps.fg3_made),0),
		       COALESCE(SUM(ps.fg_made),0), COALESCE(SUM(ps.fg_att),0),
		       COALESCE(SUM(ps.ft_made),0), COALESCE(SUM(ps.ft_att),0)
		FROM nba_player_stats ps
		JOIN nba_players p ON p.espn_id = ps.espn_id
		WHERE p.position IS NOT NULL AND p.position != ''`
	args := []any{}
	if asOfSeason > 0 {
		query += ` AND ps.season < $1`
		args = append(args, asOfSeason)
	}
	query += `
		GROUP BY ps.espn_id, p.name, p.position, p.birth_date, ps.season`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]map[int]*seasonAgg{}
	for rows.Next() {
		var a seasonAgg
		var byYear float64
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&a.espnID, &a.name, &a.position, &byYear, &a.season, &a.games,
			&pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		a.birthYear = int(byYear)
		divisor := float64(a.games)
		a.values = map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
		a.attFG, a.attFT = fga, fta
		if out[a.espnID] == nil {
			out[a.espnID] = map[int]*seasonAgg{}
		}
		out[a.espnID][a.season] = &a
	}
	return out, rows.Err()
}

// applyRateStatShrinkage pulls each season's small-sample FG%/FT% toward the
// position-group mean (computed across the same restricted pool, so the
// backtester's temporal-integrity cutoff is inherited automatically),
// weighted by that season's own attempts vs. k — ports
// docs/stats/bayesian-shrinkage.md's formula (shrunk = (n·obs + k·mean)/(n+k))
// to NBA's two rate categories. k <= 0 is an exact no-op. Mutates values in
// place; called once right after loadSeasonAggregates, before any comp
// matching or z-scoring sees the data — same "shrink before use" discipline
// the NFL pipeline follows.
func applyRateStatShrinkage(bySeason map[string]map[int]*seasonAgg, k float64) {
	if k <= 0 {
		return
	}
	type posMean struct{ fgSum, fgN, ftSum, ftN float64 }
	means := map[string]*posMean{}
	for _, seasons := range bySeason {
		for _, a := range seasons {
			m := means[a.position]
			if m == nil {
				m = &posMean{}
				means[a.position] = m
			}
			m.fgSum += a.values["fg_pct"]
			m.fgN++
			m.ftSum += a.values["ft_pct"]
			m.ftN++
		}
	}
	for _, seasons := range bySeason {
		for _, a := range seasons {
			m := means[a.position]
			if m == nil || m.fgN == 0 {
				continue
			}
			fgMean, ftMean := m.fgSum/m.fgN, m.ftSum/m.ftN
			a.values["fg_pct"] = (a.attFG*a.values["fg_pct"] + k*fgMean) / (a.attFG + k)
			a.values["ft_pct"] = (a.attFT*a.values["ft_pct"] + k*ftMean) / (a.attFT + k)
		}
	}
}

func ageAt(a *seasonAgg) float64 {
	if a == nil || a.birthYear == 0 {
		return 0
	}
	return float64(a.season - a.birthYear)
}

type playerProjection struct {
	espnID     string
	name       string
	position   string
	baseSeason int
	targetSeason int
	values     map[string]float64
	compCount  int
}

// projectAll computes a comp-based projection for every player who actually
// played in targetSeason-1 (the only players it makes sense to project
// forward into targetSeason — someone whose last action was years earlier is
// not a target this run, not merely a thin sample), using comp candidates
// drawn from bySeason (which the caller has already season-restricted via
// loadSeasonAggregates's asOfSeason cutoff). Mirrors nbaPlayerProjection's
// distance/growth formulas exactly at cfg's defaults.
func projectAll(bySeason map[string]map[int]*seasonAgg, targetSeason int, cfg projConfig) []playerProjection {
	// Build the candidate pool once: every (player, season) with an observable
	// next season and enough games to be a real sample — shared across every
	// target's comp search, mirroring the live handler's per-target query but
	// computed once for the whole batch instead of once per player.
	type candidate struct {
		cur, next, prior *seasonAgg
		trajectory       map[string]float64
	}
	var pool []candidate
	for _, seasons := range bySeason {
		for s, cur := range seasons {
			if cur.games < minCandidateGames {
				continue
			}
			next, ok := seasons[s+1]
			if !ok {
				continue // no observable growth — excluded, not zero-scored (backfill incomplete)
			}
			prior := seasons[s-1]
			traj := make(map[string]float64, len(nbaCategories))
			if prior != nil {
				for _, cat := range nbaCategories {
					traj[cat] = cur.values[cat] - prior.values[cat]
				}
			}
			pool = append(pool, candidate{cur: cur, next: next, prior: prior, trajectory: traj})
		}
	}

	// z-score basis spans the whole candidate pool — recomputed fresh every
	// run (and, for the backtester, fresh every target season), same
	// "recompute from the restricted pool" discipline as NFL's
	// recomputeZScores.
	type stat struct{ mean, stdev float64 }
	n := float64(len(pool))
	if n == 0 {
		return nil
	}
	statline := make(map[string]stat, len(nbaCategories))
	trajectory := make(map[string]stat, len(nbaCategories))
	for _, cat := range nbaCategories {
		var sum, sumT float64
		for _, c := range pool {
			sum += c.cur.values[cat]
			sumT += c.trajectory[cat]
		}
		mean, meanT := sum/n, sumT/n
		var varSum, varSumT float64
		for _, c := range pool {
			d := c.cur.values[cat] - mean
			varSum += d * d
			dT := c.trajectory[cat] - meanT
			varSumT += dT * dT
		}
		statline[cat] = stat{mean: mean, stdev: math.Sqrt(varSum / n)}
		trajectory[cat] = stat{mean: meanT, stdev: math.Sqrt(varSumT / n)}
	}
	var sumAge float64
	for _, c := range pool {
		sumAge += ageAt(c.cur)
	}
	meanAge := sumAge / n
	var varAge float64
	for _, c := range pool {
		d := ageAt(c.cur) - meanAge
		varAge += d * d
	}
	ageStdev := math.Sqrt(varAge / n)

	avgZ := func(values map[string]float64, stats map[string]stat) float64 {
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

	// Growth baselines: the plain (unweighted) mean growth per category,
	// grouped by position, across the whole candidate pool — the
	// age/position-conditioned "what do players like this typically do"
	// fallback growth_shrinkage_k blends toward when a target's own comp
	// pool is thin (docs/algorithm-review.md §6's growth_baseline.go, ported).
	type growthSum struct {
		sum map[string]float64
		n   float64
	}
	baselineSums := map[string]*growthSum{}
	for _, c := range pool {
		gs := baselineSums[c.cur.position]
		if gs == nil {
			gs = &growthSum{sum: make(map[string]float64, len(nbaCategories))}
			baselineSums[c.cur.position] = gs
		}
		for _, cat := range nbaCategories {
			mv, nv := c.cur.values[cat], c.next.values[cat]
			if cat == "fg_pct" || cat == "ft_pct" {
				gs.sum[cat] += math.Max(-0.3, math.Min(0.3, nv-mv))
				continue
			}
			if mv < 0.5 {
				gs.sum[cat] += 1.0
				continue
			}
			gs.sum[cat] += math.Max(0.3, math.Min(2.5, nv/mv))
		}
		gs.n++
	}
	baselineGrowth := map[string]map[string]float64{}
	for position, gs := range baselineSums {
		m := make(map[string]float64, len(nbaCategories))
		for _, cat := range nbaCategories {
			if gs.n > 0 {
				m[cat] = gs.sum[cat] / gs.n
			} else {
				m[cat] = 1.0
			}
		}
		baselineGrowth[position] = m
	}

	var out []playerProjection
	for espnID, seasons := range bySeason {
		// Only players who actually played in targetSeason-1 are in scope —
		// someone whose last action was years earlier isn't a target this
		// run (unlike the live per-player handler, which is only ever asked
		// about players already known to be currently relevant).
		base, ok := seasons[targetSeason-1]
		if !ok {
			continue
		}
		// Too thin a sample to anchor growth from: fall back to the best
		// sample among every season up to and including targetSeason-1 —
		// identical rule to the live handler, just bounded to "the past" for
		// this target season rather than the player's whole career.
		if base.games < minProjectionBaseGames {
			for s, cand := range seasons {
				if s <= targetSeason-1 && cand.games > base.games {
					base = cand
				}
			}
		}
		baseValues := base.values
		basePrior := seasons[base.season-1]
		baseTrajectory := make(map[string]float64, len(nbaCategories))
		if basePrior != nil {
			for _, cat := range nbaCategories {
				baseTrajectory[cat] = baseValues[cat] - basePrior.values[cat]
			}
		}
		baseAge := ageAt(base)
		baseGroups := [3]float64{avgZ(baseValues, statline), avgZ(baseTrajectory, trajectory), ageZ(baseAge)}
		totalWeight := cfg.WeightStatline + cfg.WeightTrajectory + cfg.WeightAge

		type scored struct {
			cand candidate
			dist float64
		}
		var scoredPool []scored
		for _, c := range pool {
			if c.cur.espnID == espnID {
				continue // never your own comp
			}
			groups := [3]float64{avgZ(c.cur.values, statline), avgZ(c.trajectory, trajectory), ageZ(ageAt(c.cur))}
			d0, d1, d2 := baseGroups[0]-groups[0], baseGroups[1]-groups[1], baseGroups[2]-groups[2]
			weighted := cfg.WeightStatline*d0*d0 + cfg.WeightTrajectory*d1*d1 + cfg.WeightAge*d2*d2
			scoredPool = append(scoredPool, scored{cand: c, dist: math.Sqrt(weighted / totalWeight)})
		}
		sort.Slice(scoredPool, func(i, j int) bool { return scoredPool[i].dist < scoredPool[j].dist })
		if len(scoredPool) > maxComps {
			scoredPool = scoredPool[:maxComps]
		}

		type growthEntry struct {
			sim    float64
			growth map[string]float64
		}
		var growths []growthEntry
		for _, s := range scoredPool {
			sim := 1 / (1 + s.dist)
			g := make(map[string]float64, len(nbaCategories))
			for _, cat := range nbaCategories {
				mv, nv := s.cand.cur.values[cat], s.cand.next.values[cat]
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
		if len(growths) == 0 {
			for _, cat := range nbaCategories {
				projected[cat] = baseValues[cat]
			}
		} else {
			var sumSimSq float64
			for _, g := range growths {
				sumSimSq += g.sim * g.sim
			}
			baseline := baselineGrowth[base.position]
			for _, cat := range nbaCategories {
				var acc float64
				for _, g := range growths {
					w := g.sim * g.sim / sumSimSq
					acc += w * g.growth[cat]
				}
				// Shrink toward the position growth baseline when the real
				// comp pool is thin (sumSimSq small) — a pseudo-comp of
				// weight cfg.GrowthShrinkageK. k<=0 or no baseline for this
				// position is an exact no-op.
				if cfg.GrowthShrinkageK > 0 && baseline != nil {
					acc = (sumSimSq*acc + cfg.GrowthShrinkageK*baseline[cat]) / (sumSimSq + cfg.GrowthShrinkageK)
				}
				if cat == "fg_pct" || cat == "ft_pct" {
					projected[cat] = math.Max(0, math.Min(1, baseValues[cat]+acc))
				} else {
					projected[cat] = math.Max(0, baseValues[cat]*acc)
				}
			}
		}

		out = append(out, playerProjection{
			espnID: espnID, name: base.name, position: base.position,
			baseSeason: base.season, targetSeason: targetSeason,
			values: projected, compCount: len(growths),
		})
	}
	return out
}

// fantasyZScoreSums computes one overall fantasy-value z-score sum per player
// over a set of stat lines (projected or actual) — no position split, since
// NBA category-league rankings are published overall (confirmed via
// docs/stats/nba-consensus-rankings.md's source catalog), unlike NFL where
// raw point volume differs by position. Turnovers is negated so "higher
// population z-sum" always means "better for fantasy" — same convention as
// frontend/.../lib/nbaRanking.ts's nbaFantasyZScore/nbaComputeRanks, ported to
// Go for server-side (batch/backtest) use. The raw sums, not just the final
// rank, are what the backtester correlates — re-ranking a subset of players
// should re-derive ranks from scratch over just that subset (see ranksOf in
// backtest.go), not reuse ranks computed over a different population.
func fantasyZScoreSums(values map[string]map[string]float64) map[string]float64 {
	n := float64(len(values))
	if n == 0 {
		return nil
	}
	type stat struct{ mean, stdev float64 }
	stats := make(map[string]stat, len(nbaCategories))
	for _, cat := range nbaCategories {
		var sum float64
		for _, v := range values {
			sum += v[cat]
		}
		mean := sum / n
		var varSum float64
		for _, v := range values {
			d := v[cat] - mean
			varSum += d * d
		}
		stats[cat] = stat{mean: mean, stdev: math.Sqrt(varSum / n)}
	}

	scores := make(map[string]float64, len(values))
	for id, v := range values {
		var total float64
		for _, cat := range nbaCategories {
			s := stats[cat]
			if s.stdev == 0 {
				continue
			}
			z := (v[cat] - s.mean) / s.stdev
			if cat == "turnovers" {
				z = -z
			}
			total += z
		}
		scores[id] = total
	}
	return scores
}

// ranksFromScores converts "higher is better" scores into a 1-based overall
// rank (1 = best).
func ranksFromScores(scores map[string]float64) map[string]int {
	type scored struct {
		id    string
		total float64
	}
	scoredList := make([]scored, 0, len(scores))
	for id, total := range scores {
		scoredList = append(scoredList, scored{id: id, total: total})
	}
	sort.Slice(scoredList, func(i, j int) bool { return scoredList[i].total > scoredList[j].total })
	ranks := make(map[string]int, len(scoredList))
	for i, s := range scoredList {
		ranks[s.id] = i + 1
	}
	return ranks
}

// rankByFantasyValue is the convenience composition used by the live
// -project run, which only needs the final rank, not the intermediate score.
func rankByFantasyValue(values map[string]map[string]float64) map[string]int {
	return ranksFromScores(fantasyZScoreSums(values))
}
