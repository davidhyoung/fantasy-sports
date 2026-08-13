# NFL Player Projection Algorithm

> Last synced with code: June 2026. If you change `cmd/projections/`, update this doc
> (and `docs/algorithm-review.md` if assumptions change).

## Overview

The projection system uses a comp-based (PECOTA-style) approach: for each target player,
it finds historical players with similar statistical profiles and physical/career
attributes, then uses how those comparable players developed in subsequent seasons to
project the target's future performance.

Unlike regression-to-the-mean systems that apply fixed aging curves, this approach lets
the data speak — if a player's closest comps all declined sharply at age 29, that
trajectory is baked into the projection. Comps that **washed out of the league** after
their match season count as zero-growth outcomes, so archetypes with high washout rates
(aging RBs, marginal producers) are projected down accordingly.

The system runs as an offline batch CLI (`cmd/projections/main.go`) and writes results
to the database. There are no per-request computations.

---

## Data Sources

- **nflverse** weekly player stats (1999–present), imported via `cmd/import/`
- `nfl_players` — player metadata: position, physical profile, draft info, career stage
- `nfl_player_stats` — weekly box scores aggregated to seasonal totals during profile building
- Team-level context (pass/rush yards per game, fantasy points per game) derived from
  `nfl_player_stats` itself — no external team data feed required
- Defense-vs-position strength, also derived from `nfl_player_stats` (`opponent_team`)

---

## Player Profiles

Step 1 (`-profiles`) reads `nfl_player_stats` and `nfl_players` and computes one row per
player per season (min 4 active games) in `nfl_player_season_profiles`.

**Strength-of-schedule adjustment** (`docs/stats/strength-of-schedule.md`): each weekly
production stat (yards, TDs, fantasy points, air yards, YAC, first downs) is scaled by
`league_avg_PPR_allowed / opponent_PPR_allowed` for the player's position group that
season, capped to [0.75, 1.25], before summing into season totals. Volume (attempts,
targets), turnovers, kicking, and EPA averages stay raw. Profiles therefore hold
*schedule-neutral* production.

Each profile captures per-game rates, efficiency metrics (YPA, YPC, YPR, comp%, EPA/play),
usage shares (target share, WOPR, rush yard share), physical profile, career stage,
team context, and volume.

**Z-scores with shrinkage** (`docs/stats/bayesian-shrinkage.md`): before z-scoring, each
rate stat (YPA, comp%, YPC, YPR, FG%, EPA/play) is shrunk toward the position-group's
attempt-weighted mean: `shrunk = (n·observed + k·mean) / (n + k)`, where `n` is the
player's attempts/carries/receptions and `k` is the median sample size in the pool.
A 20-carry 7.0-YPC season no longer z-scores like an elite rusher. Usage shares are
*not* shrunk — role is real even in short samples. All dimensions are then z-scored
within the position group **globally across seasons** (so a z of +1.0 means the same
thing in 2005 and 2024) and stored as JSONB in `z_scores`.

---

## Similarity Matching

Step 2 (`-project`) compares each target player's base-season profile against every
historical profile in the same position group (age within ±2 years) using weighted
Euclidean distance over **dimension groups** — not individual stats. Each group's
z-scores are averaged first, so adding more stats to a group doesn't dilute the
group's weight:

```
group_diff_g = avg_z(target, g) − avg_z(cand, g)
distance     = sqrt( Σ w_g × group_diff_g² / Σ w_g )
similarity   = 1 / (1 + distance)
```

A comp is accepted only if `similarity ≥ 0.60` (tunable). There is no fixed-N cutoff —
a common archetype may produce 40+ comps while a unique player produces 0.

### Dimension groups and default weights

| Position | Groups (weight) |
|---|---|
| QB | passing (3.0), rushing (2.0), value (2.0), physical (0.75), context (0.75), grade (1.25) |
| RB | rushing (3.0), receiving (2.0), value (2.0), physical (1.25), context (0.75), grade (1.25) |
| WR | receiving (3.0), rushing (0.75), value (2.0), physical (1.0), context (0.75), grade (1.25) |
| TE | receiving (3.0), rushing (0.5), value (2.0), physical (0.75), context (0.75), grade (1.25) |
| K | kicking (3.0), value (3.0), physical (1.0), grade (1.25) |

- The **grade** group is `overall_grade_z` from `nfl_player_grades` — prefers comps who
  were similarly good at football overall, not just stat-line twins.
- **Draft capital** is added as an extra group (weight 1.0) only for players with
  < 3 years experience.
- Weights live in `projection_config.json` (keyed by group name) and are tuned by
  `-autotune`. The field lists per group are fixed in `positionGroups()` in `main.go`.

---

## Development Curve Projection

For each accepted comp, the algorithm looks up the comp's next qualifying season and
computes a growth rate `next_season_PPR/G ÷ match_season_PPR/G`, clamped to
[0.1, 3.0]. The target's projection is the similarity²-weighted average growth applied
to their base-season value:

```
weight_i        = similarity_i² / Σ similarity²
projected_PPR/G = base_PPR/G × Σ weight_i × growth_i
```

- **Washed-out comps** (no qualifying season after the match, despite the data extending
  past it) contribute `growth = 0`, bypassing the clamp floor — washing out is a real
  outcome, not a measurement outlier. Skipping them (the old behavior) was survivorship
  bias that inflated projections for risky archetypes.
- **Zero-comp targets** regress halfway to the position-group mean
  (`0.5 × base + 0.5 × group_mean`) instead of repeating their season verbatim.
- A position-phase **aging multiplier** (developing ×1.02 … late-career ×0.93, tunable)
  is applied on top; it's deliberately small because the age-matched comps already
  embed most aging signal.
- A **grade-trend nudge** (±5% max) adjusts for players whose real-life grade is
  trending up/down faster than their stat line.
- Component stats (yards, TDs, receptions, FG) are scaled by the same overall growth
  ratio — the system projects fantasy production, not independent stat shapes.

## Rookie Projections

Everything above requires a target's own base-season profile to grow forward — a
player with zero NFL games has none, so incoming rookies are invisible to it.
`rookies.go` covers that gap as an additional step inside `-project`:

- **Targets:** `nfl_players` rows with `entry_year == targetSeason` that the main
  loop didn't already project.
- **Comp pool:** historical rookie-season profiles (`years_exp == 0`, which is
  derived per-profile from `season - entry_year`, not "earliest season we've
  imported"). Profiles only exist for player-seasons with ≥ `minGames` games, so
  redshirt/zero-snap rookie years are already excluded.
- **Similarity is draft slot only** — a Gaussian kernel (σ = 40 picks, ~1.5
  rounds) on overall draft number. Undrafted players (target or comp) are pinned
  to a synthetic slot past the last real pick, so UDFAs comp against UDFAs. There's
  no stat-line to match on, since the target has never played.
- **Projection = the comps' own rookie-season output**, similarity²-weighted —
  not a growth rate applied to anything, since there's no prior value of the
  target's to grow. At least `rookieMinComps` (5) comps are kept even if none
  clear the similarity threshold (thin pools — e.g. rookie kickers — are rare in
  the historical data but shouldn't return nothing).
- **Confidence** reuses the same weighted formula as veteran projections, but
  `ConfDataQuality` is pinned to 0 (the target has zero NFL seasons of their own),
  which caps rookie confidence below a comparable veteran's.
- Shares `nfl_projections` (same upsert, same calibration) with veteran
  projections, so every consumer sees rookies with no query changes.
  `base_season` is stored as `targetSeason - 1` for schema consistency only — no
  profile backs it for rookies.
- **Depends on current-season rosters being imported** (`make import-nfl
  ARGS="-from N -to N -rosters-only"`) so `entry_year`/`draft_number` are
  populated. nflverse publishes each year's roster — including that year's draft
  class — well before Week 1, independent of stats availability.

## Outcome Distribution (uncertainty)

Per `docs/stats/uncertainty-quantification.md`: the comp set is itself an empirical
outcome distribution. Alongside the point estimate, the engine stores the
similarity²-weighted standard deviation (per-game) and **P10 / P50 / P90** season-total
PPR quantiles, computed by walking cumulative comp weight over sorted implied outcomes
(washouts contribute zeros, which is what gives risky archetypes their low floors).
Fewer than 2 usable comps → quantiles collapse to the point estimate.

Backtests check calibration: the fraction of realized per-game outcomes below P10 /
above P90 is stored in `nfl_backtest_results.p10_coverage / p90_coverage`
(well-calibrated ≈ 0.10 each). As of June 2026, P10 coverage runs ~2–6% (floor slightly
too pessimistic — washout zeros widen it) and P90 ~7–18%.

---

## Level Calibration

The last step before a projection is stored. Measured across 2015–2024 with
temporal integrity (`-cohort-bias`), the engine projects startable players about
**12% high** — in every season sampled, and at every projection level, so the bias
is proportional rather than additive. `projection_calibration` in
`projection_config.json` scales every projected quantity by a per-position factor
(seeded uniform at **0.884**); `calibration.go` applies it at the end of
`computeWeightedProjection`, so the production and backtest paths share it.

A *uniform* factor is invisible to the draft board and visible only in the
numbers you read:

```
VOR = k·points − k·replacement = k·(points − replacement)
auction = share of the VOR pool  →  k cancels
```

Applying it moved 0 position ranks, 0 auction values and 0 consensus values on the
2026 board, while bias went +1.583 → −0.000 ppg and MAE 3.337 → 2.988.

Per-stat rates are scaled alongside the point totals, so a player's projected
yardage still reconciles with his projected points, and kickers — whose league
points are computed from rates rather than a generic total — don't drift relative
to skill positions.

Two caveats worth carrying: part of the 12% is definitional rather than error (our
per-game number is an if-healthy, in-role rate, while actuals include games played
hurt and benchings), and the estimate is a floor, since players who lost their job
entirely are excluded from the comparison. The residual after a uniform factor is
not uniform — QB ends ~8% under, RB ~5% over — and per-position factors would fix
that at the cost of repricing positions against each other. See
`docs/algorithm-review.md` §7.7–7.8.

---

## Confidence Score

Five factors, computed in `computeProjections`:

```
confidence = 0.25 × avg_similarity
           + 0.20 × min(1, comp_count / 10)
           + 0.25 × comp_agreement        (1 / (1 + stdev of comp growths))
           + 0.15 × sample_depth          (fraction of comps with a usable outcome)
           + 0.15 × min(1, seasons_of_history / 3)
```

Confidence captures "how much to trust the projection process"; the P10–P90 spread
captures "how wide the outcome range is." A boom/bust player can have high confidence
in a wide distribution.

## Profile Uniqueness

`uniqueness` label from comp count: 0 → `unique`, ≤3 → `rare`, <10 → `moderate`,
≥10 with avg similarity ≥ 0.70 → `common`.

---

## Backtesting & Tuning

`-backtest` replays each target season using **only prior-season data** — profiles are
filtered, then z-scores *and shrinkage priors* are recomputed from the restricted pool.
Metrics are stored in `nfl_backtest_results` on two bases:

- `eval_basis = 'total'` — projected vs actual season PPR totals (what a drafter
  experiences, but dominated by unpredictable injuries)
- `eval_basis = 'per_game'` — projected vs actual PPR/G (≥4 games played); the cleaner
  skill signal and the **tuning objective**

`-autotune` runs coordinate ascent (similarity threshold, age window, aging multipliers,
per-position group weights) maximizing **mean per-game Spearman rank correlation** on
training seasons — draft decisions are ordinal, so ordering players correctly beats
minimizing point error. The tuned config must beat the default config on held-out
validation seasons or the default is kept. Winner is saved to `projection_config.json`.

Benchmark (June 2026, train 2015–2021, validate 2022–2024): per-game ρ ≈ 0.71 train,
**0.75 validation**; season-total RMSE ~68–81 pts (vs ~82–92 before the survivorship
fix + shrinkage + SOS). Note: pre-June-2026 backtest rows measured a "repeat last
season" baseline due to a missing `MatchProfile` bug and are not comparable.

---

## Limitations

- **Team context changes:** projections assume stable team context. A trade or
  coordinator change can invalidate a projection; `context` dimensions only partially
  mitigate.
- **Uniform component scaling:** can't express "fewer yards, more TDs" development.
- **Era pooling:** global z-scores mean passing-era inflation leaks into cross-era
  comps; SOS narrows but doesn't eliminate it.
- **Grade dimension absent in backtests:** backtests recompute z-scores without
  `overall_grade_z` (grades aren't recomputed per historical cutoff), so backtest
  similarity slightly differs from production similarity.
- **17-game projection for everyone:** durability risk is not modeled in totals.
- **Kicker volatility:** FG% shrinkage helps, but kicker projections remain
  structurally low-confidence.

---

## Running

All commands run from the `backend/` directory:

```bash
make project-nfl ARGS="-profiles"               # step 1: build season profiles (SOS + shrinkage)
make project-nfl ARGS="-grades"                 # step 2: player grades + grade z enrichment
make project-nfl ARGS="-project -season 2026"   # step 3: comp-based projections
make project-nfl ARGS="-all -season 2026"       # all steps
make backtest-nfl ARGS="-from 2015 -to 2024"    # dual-basis backtest
make autotune-nfl ARGS="-from 2015 -to 2024 -train-to 2021"  # tune on per-game rank corr
```

Steps are idempotent — re-running upserts on conflict.

---

## Database Tables

### `nfl_player_season_profiles`
One row per player per season: schedule-adjusted per-game rates, raw totals metadata,
career stage, team context, `z_scores` JSONB (shrunk rates, grade z), `tags` TEXT[].

### `nfl_projections`
One row per (player, base_season, target_season): projected per-game stat line, season
totals (std/half/PPR), `proj_fpts_ppr_stdev` + `proj_fpts_ppr_p10/p50/p90`, confidence
breakdown, comp count/avg similarity/uniqueness, `comps` JSONB (top 10, each with
similarity, weight, `washed_out`, match profile, pre/post trajectories, matching and
divergent dimension groups).

### `nfl_backtest_results`
Accuracy metrics per (target_season, position_group, eval_basis): RMSE, MAE, Pearson r,
Spearman ρ, tier accuracy (top-12 QB/TE, top-24 RB/WR), quantile coverage, and the full
config JSON used.
