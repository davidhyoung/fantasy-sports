# Bayesian Shrinkage (Empirical Bayes)

> **Status: implemented (June 2026)** in `backend/cmd/projections/shrinkage.go`,
> applied in both z-score paths (`computeZScores` in `main.go`, `recomputeZScores`
> in `backtest.go`). Shrunk rates: YPA, comp%, YPC, YPR, FG%, and the three
> EPA/play fields. Usage shares (target_share, wopr) are deliberately NOT shrunk —
> role is real even in short samples. Chosen policy: prior = attempt-weighted
> position-group mean (Σ rate·n / Σ n); `k` = median sample size in the pool, so a
> median-volume player keeps ~50% weight. Priors are computed from whatever profile
> pool is passed in, so backtests stay temporally clean. The live league-ranking
> path (`internal/handlers/analysis.go`) still does NOT shrink — relevant mainly
> for lastweek/early-season periods. See also the **growth-rate extension**
> below (August 2026) — same formula, different target, behind its own no-op default.

## Problem it solves

Raw rate statistics are unstable on small samples. A kicker who goes 3-for-3 has a 100% FG rate; a hitter who goes 4-for-10 has a .400 average. Ranking these players head-to-head against veterans with thousands of attempts gives nonsensical results — their z-scores explode precisely because their sample sizes are tiny. The same failure mode hits early-season fantasy rankings, injured players returning mid-year, and rookies in Weeks 1–4.

## Technique

Pull ("shrink") each player's observed rate toward the population mean, with the pull strength inversely proportional to sample size. A player with lots of data barely moves; a player with very little data collapses toward the prior.

The simplest form — the **James–Stein / Efron–Morris estimator** for rates:

```
shrunk_rate = (n * observed_rate + k * population_mean) / (n + k)
```

Where:
- `observed_rate` = player's raw rate (e.g. FG%, target share, YPC)
- `n` = player's sample size (attempts, targets, carries)
- `population_mean` = mean rate across all players in the same position group
- `k` = shrinkage constant; larger `k` pulls harder toward the mean

Choosing `k`: a principled default is `k = population_variance / between_player_variance`, estimated empirically from the data. A practical shortcut is to pick `k` so that a "median-sample" player (e.g. league-median attempts) has their observed rate weighted ~50%.

## Assumptions

- Players in the pool are roughly exchangeable — i.e. reasonable to group them under a single prior. Position groups usually satisfy this; mixing QBs and RBs does not.
- The prior (population mean) is itself estimated from enough data to be stable.
- You care more about predictive accuracy on the next game/season than about preserving extreme point estimates.

## When it applies in this codebase

- `internal/handlers/analysis.go` — per-category z-scores in the ranking engine. Currently a player with one huge week can dominate their category. Shrink the player's rate toward position-group mean before computing the z-score.
- `cmd/projections/profiles.go` — z-scored dimensions stored in `nfl_player_season_profiles.z_scores`. Rate stats (YPC, target share, completion %) should be shrunk before z-scoring, especially for partial seasons (`games_played < 8`).
- `cmd/projections/grades.go` — efficiency sub-scores for rookies and injured players. A RB with 20 carries and a 7.0 YPC is not actually a better rusher than a 300-carry workhorse at 4.8.
- **In-season rankings once we add them** — Weeks 1–4 are exactly when shrinkage matters most; by Week 12 the shrinkage is effectively invisible for regulars.

## Worked example

NFL RB yards-per-carry, early in the 2025 season:

| Player | Carries | Raw YPC | Shrunk YPC (k=80) |
|---|---|---|---|
| Veteran A | 180 | 4.2 | 4.24 |
| Rookie B | 25 | 6.8 | 4.92 |
| Backup C | 8 | 2.1 | 4.28 |

Population mean YPC ≈ 4.3. With `k=80`:
- Veteran A barely moves (4.2 → 4.24) — we trust his 180-carry sample.
- Rookie B drops hard (6.8 → 4.92) — still above average, but the difference against Veteran A is now believable, not absurd.
- Backup C climbs (2.1 → 4.28) — 8 carries tells us almost nothing, so he collapses to the prior.

The ranking is now defensible; without shrinkage, Rookie B would have a z-score roughly 3× Veteran A's despite contributing far less fantasy value.

## How to validate it's working

- Add a backtest mode in `cmd/projections/backtest.go` that compares raw-z vs shrunk-z projections on out-of-sample seasons. Track RMSE, MAE, and correlation in `nfl_backtest_results`.
- Sanity check: in Weeks 1–3, the top-10 shrunk-z rankings should look more like preseason consensus than the top-10 raw-z rankings do. If they don't, `k` is probably too small.
- Unit test: a player with zero attempts should come out at exactly the population mean.

## Extension: shrinking derived growth rates (August 2026)

> **Status: implemented behind a no-op default** in `growth_baseline.go` +
> `computeWeightedProjection` (`main.go`). `GrowthShrinkageK = 0` (seeded
> default) is exact — ships with zero behavior change until `-autotune` finds a
> nonzero value that beats the default on held-out validation.

The same problem this entry solves for raw rate stats (small samples give
unstable estimates) also hits the *comp-derived growth rate* itself: a player
with `comp_count=2` (e.g. a 31-year-old RB still at elite volume — almost no
historical comps exist at that age) has their entire projection swing on
whatever those 1-2 comps happened to do, with no mechanism pulling back toward
a broader prior. Applying the exact same formula here:

```
shrunk_growth = (totalWeight * growthRate + k * baselineGrowth) / (totalWeight + k)
```

Where `totalWeight` is the comp-similarity²-weighted sum (the natural analog of
`n` — an "effective comp count," not a raw count) and `baselineGrowth` is the
mean historical year-over-year `fpts_ppr_pg` ratio for players in the same
`(position_group, career_phase)` bucket (`internal/aging.Phase` — reusing the
existing 4-phase bucketing rather than inventing new ones). Computed by
`computeGrowthBaselines`, a shared pure function (mirrors `computeGroupMeans`'s
existing precedent) called identically from production and the
temporally-restricted backtest path.

**`k` selection deliberately deviates from this entry's own policy.** Above,
`k` is the *median sample size in the current pool* — not available here
without restructuring the per-target projection loop into two passes (every
target's comp search would need to finish before a cross-player median could be
known). Instead `GrowthShrinkageK` is a fixed, `-autotune`-sweepable scalar,
seeded from the codebase's existing `commonArchetype = 10` constant (already
the established notion of "a well-supported comp count" — see
`ConfCompCount = min(1, count/10)`). Simpler, avoids restructuring the
projection loop, still tunable — but a real methodological difference from the
dynamic-median policy above, noted here rather than silently diverging.

Implementation detail worth knowing: rather than a separate post-hoc
adjustment, the shrinkage baseline is added as one extra weighted
pseudo-observation into the *same* weighted-sum loops `applyGrowth` and the
outcome-distribution block already use — so the point estimate and the
P10/P50/P90 uncertainty band stay consistent by construction, and a thin-comp
player's distribution correctly *narrows* toward the baseline (not widens),
since one large weighted point mass pulls both the mean and the quantiles.

## Tradeoffs

- **Adds one tuning parameter** (`k`) per stat group. Wrong `k` values either over-correct (everyone looks average) or under-correct (small samples still blow up).
- **Obscures extreme but real performers.** A genuinely elite rookie will be dragged toward the mean early — accurate in expectation, but frustrating to a user who sees him downranked.
- **Needs a sensible prior.** If the position-group mean is itself unstable (e.g. TE target share in a year with lots of injuries), shrinkage just pulls toward a noisy target.

## References

- Efron & Morris, *Stein's Paradox in Statistics* (Scientific American, 1977) — the canonical intuition piece.
- Tango, Lichtman & Dolphin, *The Book: Playing the Percentages in Baseball* (2007) — ch. on regression to the mean; batting-average shrinkage is the textbook application.
- Carl Morris, *Parametric Empirical Bayes Inference: Theory and Applications* (JASA, 1983) — formal derivation.
