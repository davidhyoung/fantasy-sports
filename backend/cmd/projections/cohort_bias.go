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
	n       int
	sumErr  float64 // signed: projected − actual, per game
	sumAbs  float64
	tooHigh int
	projSum float64
	actSum  float64
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

// levelBands split players by how good we said they'd be, so a uniform level
// bias can be told apart from one that scales with the projection.
var levelBands = []struct {
	label    string
	min, max float64
}{
	{"< 8 ppg", 0, 8},
	{"8-12", 8, 12},
	{"12-16", 12, 16},
	{"16+ ppg", 16, 1e9},
}

func bandOf(projPG float64) string {
	for _, b := range levelBands {
		if projPG >= b.min && projPG < b.max {
			return b.label
		}
	}
	return levelBands[len(levelBands)-1].label
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
// minBasePPG optionally restricts the report to players who were already
// fantasy-relevant in their base season. It filters on an INPUT (the base
// season), never on the outcome, so it can't select for our own errors.
func runCohortBias(ctx context.Context, pool *pgxpool.Pool, fromYear, toYear int, cfg projConfig, sweep []float64, minBasePPG float64) error {
	allProfiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	actuals, err := loadActuals(ctx, pool)
	if err != nil {
		return fmt.Errorf("load actuals: %w", err)
	}
	log.Printf("=== Cohort bias, seasons %d–%d (%d profiles, min base ppg %.1f) ===",
		fromYear, toYear, len(allProfiles), minBasePPG)

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
		levels := map[string]*cohortStat{}
		for _, b := range levelBands {
			levels[b.label] = &cohortStat{}
		}

		for target := fromYear; target <= toYear; target++ {
			outcomes := projectSeasonBacktest(runCfg, allProfiles, target)
			seasonActuals := actualsForSeason(actuals, target)
			for gsisID, out := range outcomes {
				act, ok := seasonActuals[gsisID]
				if !ok || act.Games < minEvalGames {
					continue
				}
				if base := byPlayer[gsisID][target-1]; minBasePPG > 0 && (base == nil || base.FptsPPRPG < minBasePPG) {
					continue
				}
				actualPG := act.Total / float64(act.Games)
				cohort := cohortOf(byPlayer[gsisID], target-1)
				stats[cohort].add(out.PerGame, actualPG)
				overall.add(out.PerGame, actualPG)
				levels[bandOf(out.PerGame)].add(out.PerGame, actualPG)
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

		// A uniform level bias cancels out of ranks and out of VOR, so it barely
		// touches a draft board. A bias that grows with projected level does not —
		// it stretches the top of the board away from the field. Splitting by
		// projected level is what tells those two apart.
		fmt.Printf("\n%-10s %-12s %6s %9s %8s %9s\n", label, "proj level", "n", "bias", "MAE", "bias/proj")
		for _, b := range levelBands {
			s := levels[b.label]
			if s == nil || s.n == 0 {
				continue
			}
			meanProj := s.projSum / float64(s.n)
			fmt.Printf("%-10s %-12s %6d %+9.3f %8.3f %8.0f%%\n",
				label, b.label, s.n, s.bias(), s.mae(), s.bias()/meanProj*100)
		}
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
