// backtest.go backtests the comp-based projection engine against real NBA
// history: for a target season N, project every eligible player using only
// data from seasons < N (temporal integrity — see projectAll/
// loadSeasonAggregates's asOfSeason cutoff), then compare the projected
// overall fantasy-value rank to the rank those same players actually
// achieved in season N. Mirrors cmd/projections/backtest.go's shape
// (same primary metric — rank correlation — same accept/reject discipline),
// trimmed to NBA's simpler, overall-only (no position split) ranking model.
package main

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

func marshalConfig(cfg projConfig) ([]byte, error) {
	return json.Marshal(cfg)
}

const minEligibleGames = 20 // same floor as comp-candidate eligibility

// loadActualSeason aggregates real nba_player_stats for season itself into
// per-game category rates, restricted to players with a real sample
// (minEligibleGames) — this is what "actually happened" means for a backtest
// target season.
func loadActualSeason(ctx context.Context, pool *pgxpool.Pool, season int) (map[string]map[string]float64, error) {
	rows, err := pool.Query(ctx, `
		SELECT espn_id, COUNT(*),
		       COALESCE(SUM(points),0), COALESCE(SUM(rebounds),0), COALESCE(SUM(assists),0),
		       COALESCE(SUM(steals),0), COALESCE(SUM(blocks),0), COALESCE(SUM(turnovers),0),
		       COALESCE(SUM(fg3_made),0),
		       COALESCE(SUM(fg_made),0), COALESCE(SUM(fg_att),0),
		       COALESCE(SUM(ft_made),0), COALESCE(SUM(ft_att),0)
		FROM nba_player_stats
		WHERE season = $1
		GROUP BY espn_id
		HAVING COUNT(*) >= $2
	`, season, minEligibleGames)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]map[string]float64{}
	for rows.Next() {
		var espnID string
		var games int
		var pts, reb, ast, stl, blk, tov, fg3m, fgm, fga, ftm, fta float64
		if err := rows.Scan(&espnID, &games, &pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgm, &fga, &ftm, &fta); err != nil {
			return nil, err
		}
		divisor := float64(games)
		out[espnID] = map[string]float64{
			"points": pts / divisor, "rebounds": reb / divisor, "assists": ast / divisor,
			"steals": stl / divisor, "blocks": blk / divisor, "turnovers": tov / divisor, "three_pm": fg3m / divisor,
			"fg_pct": nbaRatio(fgm, fga), "ft_pct": nbaRatio(ftm, fta),
		}
	}
	return out, rows.Err()
}

// ── correlation/error helpers (ported from cmd/projections/backtest.go's
// pearsonCorr/spearmanCorr/ranks — sport-agnostic statistics, not NFL-specific) ──

func ranksOf(vals []float64) []float64 {
	idx := make([]int, len(vals))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool { return vals[idx[i]] < vals[idx[j]] })
	out := make([]float64, len(vals))
	for rank, i := range idx {
		out[i] = float64(rank + 1)
	}
	return out
}

func pearsonCorr(x, y []float64) float64 {
	n := float64(len(x))
	if n < 2 {
		return 0
	}
	var sx, sy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
	}
	mx, my := sx/n, sy/n
	var sumXY, sumX2, sumY2 float64
	for i := range x {
		dx, dy := x[i]-mx, y[i]-my
		sumXY += dx * dy
		sumX2 += dx * dx
		sumY2 += dy * dy
	}
	if sumX2 == 0 || sumY2 == 0 {
		return 0
	}
	return sumXY / math.Sqrt(sumX2*sumY2)
}

func spearmanCorr(x, y []float64) float64 {
	return pearsonCorr(ranksOf(x), ranksOf(y))
}

type backtestResult struct {
	targetSeason    int
	rankCorrelation float64
	rmse, mae       float64
	playerCount     int
}

// evaluateSeason projects targetSeason from data strictly before it, compares
// to what actually happened, and returns one backtestResult.
func evaluateSeason(ctx context.Context, pool *pgxpool.Pool, targetSeason int, cfg projConfig) (*backtestResult, error) {
	bySeason, err := loadSeasonAggregates(ctx, pool, targetSeason)
	if err != nil {
		return nil, err
	}
	applyRateStatShrinkage(bySeason, cfg.RateStatShrinkageK)
	projections := projectAll(bySeason, targetSeason, cfg)
	if len(projections) == 0 {
		return nil, nil
	}
	projValues := make(map[string]map[string]float64, len(projections))
	for _, p := range projections {
		projValues[p.espnID] = p.values
	}
	projScores := fantasyZScoreSums(projValues)

	actualValues, err := loadActualSeason(ctx, pool, targetSeason)
	if err != nil {
		return nil, err
	}
	actualScores := fantasyZScoreSums(actualValues)

	var projVec, actVec []float64
	var sqErr, absErr float64
	var errN int
	for espnID, projScore := range projScores {
		actScore, ok := actualScores[espnID]
		if !ok {
			continue
		}
		projVec = append(projVec, projScore)
		actVec = append(actVec, actScore)

		// Secondary diagnostic: RMSE/MAE on projected vs actual points/game
		// (one representative counting stat, not the tuning objective).
		if pv, ok := projValues[espnID]["points"]; ok {
			if av, ok := actualValues[espnID]["points"]; ok {
				d := pv - av
				sqErr += d * d
				absErr += math.Abs(d)
				errN++
			}
		}
	}
	if len(projVec) < 5 {
		return nil, nil // too few overlapping players to mean anything
	}

	result := &backtestResult{
		targetSeason:    targetSeason,
		rankCorrelation: spearmanCorr(projVec, actVec),
		playerCount:     len(projVec),
	}
	if errN > 0 {
		result.rmse = math.Sqrt(sqErr / float64(errN))
		result.mae = absErr / float64(errN)
	}
	return result, nil
}

func storeBacktestResults(ctx context.Context, pool *pgxpool.Pool, results []backtestResult, cfg projConfig) error {
	cfgJSON, err := marshalConfig(cfg)
	if err != nil {
		return err
	}
	for _, r := range results {
		if _, err := pool.Exec(ctx, `
			INSERT INTO nba_backtest_results
				(target_season, rank_correlation, rmse, mae, player_count, config_used)
			VALUES ($1,$2,$3,$4,$5,$6)
		`, r.targetSeason, r.rankCorrelation, r.rmse, r.mae, r.playerCount, cfgJSON); err != nil {
			return err
		}
	}
	return nil
}

func runBacktest(ctx context.Context, pool *pgxpool.Pool, from, to int, cfg projConfig) error {
	var results []backtestResult
	for season := from; season <= to; season++ {
		r, err := evaluateSeason(ctx, pool, season, cfg)
		if err != nil {
			return err
		}
		if r == nil {
			log.Printf("  %d: skipped (too few overlapping players)", season)
			continue
		}
		log.Printf("  %d: rank_corr=%.4f rmse=%.2f mae=%.2f n=%d", season, r.rankCorrelation, r.rmse, r.mae, r.playerCount)
		results = append(results, *r)
	}
	if len(results) == 0 {
		log.Println("  no seasons produced a result")
		return nil
	}
	var sum float64
	for _, r := range results {
		sum += r.rankCorrelation
	}
	log.Printf("  mean rank correlation across %d seasons: %.4f", len(results), sum/float64(len(results)))
	return storeBacktestResults(ctx, pool, results, cfg)
}

// scoreConfig is the autotune objective: mean rank correlation across a
// season range, for a given config. Returns -1 (always "worse") if no season
// in range produced a usable result — mirrors NFL's scoreConfig guard.
func scoreConfig(ctx context.Context, pool *pgxpool.Pool, from, to int, cfg projConfig) float64 {
	var sum float64
	var n int
	for season := from; season <= to; season++ {
		r, err := evaluateSeason(ctx, pool, season, cfg)
		if err != nil || r == nil || r.playerCount < 5 {
			continue
		}
		sum += r.rankCorrelation
		n++
	}
	if n == 0 {
		return -1
	}
	return sum / float64(n)
}

// minGain guards against chasing noise in the objective — same fixed
// absolute threshold NFL's autotune uses on its own rank-correlation metric.
const minGain = 0.002

// runAutotune does coordinate-descent over the dimension-group weights and
// both shrinkage knobs jointly — repeating the whole 5-dimension sweep until
// a full pass finds no improvement, since the shrinkage knobs change the
// effective comp distances the weights are balancing, so the optimal weights
// can shift once shrinkage is active (and vice versa) — then validates the
// winner against the default config on a held-out season range, keeping
// whichever wins. Same train/validate/compare-to-default discipline as NFL's
// runAutotune.
func runAutotune(ctx context.Context, pool *pgxpool.Pool, trainFrom, trainTo, valFrom, valTo int) error {
	best := defaultProjConfig()
	bestScore := scoreConfig(ctx, pool, trainFrom, trainTo, best)
	log.Printf("  baseline train score (default config): %.4f", bestScore)

	weightGrid := []float64{0.5, 1.0, 1.5, 2.0, 2.5, 3.0}
	growthGrid := []float64{0.0, 2.0, 5.0, 10.0, 20.0}
	rateGrid := []float64{0.0, 20.0, 50.0, 100.0, 200.0}

	passImproved := true
	for passImproved {
		passImproved = false
		for _, field := range []string{"statline", "trajectory", "age"} {
			for _, v := range weightGrid {
				cand := best
				switch field {
				case "statline":
					cand.WeightStatline = v
				case "trajectory":
					cand.WeightTrajectory = v
				case "age":
					cand.WeightAge = v
				}
				score := scoreConfig(ctx, pool, trainFrom, trainTo, cand)
				if score > bestScore+minGain {
					bestScore = score
					best = cand
					passImproved = true
					log.Printf("  weight_%s=%.1f improves train score to %.4f", field, v, score)
				}
			}
		}
		for _, v := range growthGrid {
			cand := best
			cand.GrowthShrinkageK = v
			score := scoreConfig(ctx, pool, trainFrom, trainTo, cand)
			if score > bestScore+minGain {
				bestScore = score
				best = cand
				passImproved = true
				log.Printf("  growth_shrinkage_k=%.1f improves train score to %.4f", v, score)
			}
		}
		for _, v := range rateGrid {
			cand := best
			cand.RateStatShrinkageK = v
			score := scoreConfig(ctx, pool, trainFrom, trainTo, cand)
			if score > bestScore+minGain {
				bestScore = score
				best = cand
				passImproved = true
				log.Printf("  rate_stat_shrinkage_k=%.1f improves train score to %.4f", v, score)
			}
		}
	}

	defaultVal := scoreConfig(ctx, pool, valFrom, valTo, defaultProjConfig())
	tunedVal := scoreConfig(ctx, pool, valFrom, valTo, best)
	log.Printf("  validation (held-out %d-%d): default=%.4f tuned=%.4f", valFrom, valTo, defaultVal, tunedVal)
	if tunedVal < defaultVal {
		log.Println("  tuned config did not beat default on validation — discarding, keeping default")
		best = defaultProjConfig()
	}

	return saveProjConfig(best)
}
