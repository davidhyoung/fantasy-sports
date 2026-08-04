// growth_baseline.go computes an age/position-conditioned baseline year-over-year
// growth rate, used to shrink comp-derived growth estimates toward a population
// prior when a target has very few comps (docs/stats/bayesian-shrinkage.md,
// "Extension: shrinking derived growth rates"). Mirrors computeGroupMeans's
// existing precedent as a shared, pure, no-DB-access function called identically
// from computeProjections (main.go) and projectSeasonBacktest (backtest.go).
package main

import "github.com/davidyoung/fantasy-sports/backend/internal/aging"

// growthBaselineKey buckets by position group and career phase — reusing the
// existing 4-phase bucketing from internal/aging rather than inventing new ones.
type growthBaselineKey struct {
	posGroup string
	phase    string
}

// computeGrowthBaselines returns, for each (position group, career phase), the
// mean year-over-year fpts_ppr_pg growth ratio observed across every
// consecutive-season pair for the same player in the given profile pool. The
// caller controls temporal integrity by controlling what's in profiles — the
// backtest path passes its already-restricted historicProfiles, production
// passes everything.
func computeGrowthBaselines(profiles []seasonProfile) map[growthBaselineKey]float64 {
	byPlayerSeason := make(map[string]map[int]*seasonProfile)
	for i := range profiles {
		p := &profiles[i]
		if byPlayerSeason[p.GsisID] == nil {
			byPlayerSeason[p.GsisID] = make(map[int]*seasonProfile)
		}
		byPlayerSeason[p.GsisID][p.Season] = p
	}

	sums := make(map[growthBaselineKey]float64)
	counts := make(map[growthBaselineKey]int)
	for _, seasons := range byPlayerSeason {
		for season, cur := range seasons {
			next, ok := seasons[season+1]
			if !ok || cur.FptsPPRPG == 0 {
				continue
			}
			g := clampGrowth(next.FptsPPRPG / cur.FptsPPRPG)
			key := growthBaselineKey{cur.PositionGroup, aging.Phase(cur.PositionGroup, cur.Age)}
			sums[key] += g
			counts[key]++
		}
	}

	baselines := make(map[growthBaselineKey]float64, len(sums))
	for key, sum := range sums {
		baselines[key] = sum / float64(counts[key])
	}
	return baselines
}
