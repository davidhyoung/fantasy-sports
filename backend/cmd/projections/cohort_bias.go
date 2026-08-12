package main

// cohort_bias.go — measures projection bias split by whether a player's base
// season rose or fell against the one before it.
//
// The stored backtest metrics (RMSE/MAE/correlation) are unsigned and pooled, so
// they cannot answer "are we systematically too high on breakouts?" — an
// over-projection and an under-projection of the same size look identical. This
// mode reports *signed* bias per cohort, which is what separates a model that is
// noisy from one that is skewed.
//
// See docs/algorithm-review.md §7.5 for the finding that motivated it.

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"
)

// cohortSplit is the per-game move between the two seasons before the target
// that separates a breakout from a down year. ±2 ppg is roughly a third of a
// standard deviation of season-over-season change for startable players.
const cohortSplit = 2.0

type cohortStat struct {
	n        int
	sumErr   float64 // signed: projected − actual, per game
	sumAbs   float64
	tooHigh  int
	projSum  float64
	actSum   float64
}

func (c *cohortStat) add(projected, actual float64) {
	c.n++
	err := projected - actual
	c.sumErr += err
	c.sumAbs += absF(err)
	if err > 0 {
		c.tooHigh++
	}
	c.projSum += projected
	c.actSum += actual
}

func (c cohortStat) bias() float64 {
	if c.n == 0 {
		return 0
	}
	return c.sumErr / float64(c.n)
}

func (c cohortStat) mae() float64 {
	if c.n == 0 {
		return 0
	}
	return c.sumAbs / float64(c.n)
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// cohortOf classifies a player by the direction of their base season relative to
// the one before. Players without two prior seasons are unclassifiable and are
// reported separately rather than silently folded into "flat".
func cohortOf(seasonMap map[int]*seasonProfile, baseSeason int) string {
	base, prior := seasonMap[baseSeason], seasonMap[baseSeason-1]
	if base == nil || prior == nil || prior.GamesPlayed < 4 || base.GamesPlayed < 4 {
		return "unknown"
	}
	switch move := base.FptsPPRPG - prior.FptsPPRPG; {
	case move >= cohortSplit:
		return "rose"
	case move <= -cohortSplit:
		return "fell"
	default:
		return "flat"
	}
}

// runCohortBias backtests each season in the range and reports signed bias by
// cohort. With sweep values supplied it repeats the whole run per value of
// TargetBlendDecayUp, so the asymmetric-regression hypothesis can be tested
// against held-out seasons rather than a single year.
func runCohortBias(ctx context.Context, pool *pgxpool.Pool, fromYear, toYear int, cfg projConfig, sweep []float64) error {
	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	actuals, err := loadActuals(ctx, pool)
	if err != nil {
		return fmt.Errorf("load actuals: %w", err)
	}
	log.Printf("=== Cohort bias, seasons %d–%d (%d profiles) ===", fromYear, toYear, len(allProfiles))

	// Index profiles by player so cohort classification can look back two seasons.
	byPlayer := map[string]map[int]*seasonProfile{}
	for i := range allProfiles {
		p := &allProfiles[i]
		if byPlayer[p.GsisID] == nil {
			byPlayer[p.GsisID] = map[int]*seasonProfile{}
		}
		byPlayer[p.GsisID][p.Season] = p
	}

	if len(sweep) == 0 {
		sweep = []float64{cfg.TargetBlendDecayUp}
	}

	fmt.Printf("\n%-10s %-8s %6s %9s %8s %9s\n", "decay_up", "cohort", "n", "bias", "MAE", "too high")
	fmt.Println("  (bias = projected − actual, per game; + means we projected too high)")

	for _, decayUp := range sweep {
		runCfg := cfg
		runCfg.TargetBlendDecayUp = decayUp

		stats := map[string]*cohortStat{}
		overall := &cohortStat{}
		for _, name := range []string{"rose", "flat", "fell", "unknown"} {
			stats[name] = &cohortStat{}
		}

		for target := fromYear; target <= toYear; target++ {
			outcomes := projectSeasonBacktest(runCfg, allProfiles, target)
			seasonActuals := actualsForSeason(actuals, target)
			for gsisID, out := range outcomes {
				act, ok := seasonActuals[gsisID]
				if !ok || act.Games < minEvalGames {
					continue
				}
				actualPG := act.Total / float64(act.Games)
				cohort := cohortOf(byPlayer[gsisID], target-1)
				stats[cohort].add(out.PerGame, actualPG)
				overall.add(out.PerGame, actualPG)
			}
		}

		label := fmt.Sprintf("%.2f", decayUp)
		if decayUp <= 0 {
			label = "off"
		}
		for _, name := range []string{"rose", "flat", "fell", "unknown"} {
			s := stats[name]
			if s.n == 0 {
				continue
			}
			fmt.Printf("%-10s %-8s %6d %+9.3f %8.3f %8.0f%%\n",
				label, name, s.n, s.bias(), s.mae(), float64(s.tooHigh)/float64(s.n)*100)
		}
		fmt.Printf("%-10s %-8s %6d %+9.3f %8.3f %8.0f%%\n",
			label, "ALL", overall.n, overall.bias(), overall.mae(), float64(overall.tooHigh)/float64(overall.n)*100)
		fmt.Println()
	}
	return nil
}

// sortedSeasons is a small helper for deterministic logging.
func sortedSeasons(m map[int]*seasonProfile) []int {
	out := make([]int, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Ints(out)
	return out
}
