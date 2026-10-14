# NBA Projection & Ranking Algorithm

> **Last synced with code: October 2026.** Covers the native NBA comp-based system:
> historical comparisons, next-season projection (both the live per-player path and the
> batch/backtestable path added this pass), and how a projection becomes an overall
> fantasy rank. The NFL analog is [projection-algorithm.md](projection-algorithm.md) —
> this doc follows its pipeline-stage structure, but NBA's version is deliberately
> smaller: no grades dimension, no rookie draft-slot path, and (until this pass) no
> backtest harness at all. Where NBA now has a real equivalent of an NFL technique, it
> cites the matching `docs/stats/*.md` entry rather than re-deriving the math.

## Overview

Native NBA leagues score 9 roto categories (points, rebounds, assists, steals, blocks,
turnovers, 3PM, FG%, FT%) over real NBA box scores — see the "Category Scoring" and
"Data Pipeline" sections of the in-app Wiki (`/wiki?sport=nba`) for the data-source
summary. This doc covers what sits on top of the raw box scores: a comp-based
next-season projection, and the rank that projection produces once it's z-scored
against a population of other projected players.

Two projection code paths exist side by side, deliberately:

1. **Live, on-demand** (`internal/handlers/nba_projections.go`'s `nbaPlayerProjection`)
   — computed per-request when a viewer opens a player's detail panel. No season
   cutoff: it always sees every real game in the database. Unchanged by this pass.
2. **Batch, backtestable** (`cmd/project-nba/projection.go`'s `projectAll`) — the same
   distance/growth math, run once for every eligible player at once, parameterized by
   an explicit `asOfSeason` cutoff so a historical backtest can restrict the candidate
   pool to "only seasons before the one being projected." New this pass.

These intentionally duplicate rather than share code — the live handler's types are
unexported, and a live per-request path has no reason to carry a temporal-integrity
cutoff it will never need. NFL's own backtest path makes the same tradeoff (see
`docs/projection-algorithm.md`'s "Backtesting & Tuning" section) for the same reason.

## Historical Comparisons

`nbaPlayerComps` (`internal/handlers/nba_players.go`) finds the historical
player-seasons that most resemble a target's own best-sample season — weighted
Euclidean distance over three dimension groups, each averaged to one z-score before
distance (so adding categories to the statline group doesn't dilute its weight):

```
statline  (9 per-game categories, weight 2.0)
physical  (height, weight, weight 1.0)
stage     (young/prime/post-prime ordinal from age, weight 1.0)
```

Restricted to the same ESPN position bucket (G/F/C). Turnovers is **not** inverted here
(this measures statistical resemblance, not fantasy value — a high-turnover playmaker
should read as similar to another high-turnover playmaker). Candidates missing
height/weight/birth_date are excluded entirely; if the target itself is missing one of
those, that dimension group is dropped from the formula rather than failing the whole
request. Unchanged by this pass.

## Comp-Based Projection

Both the live and batch paths share the same formula:

```
base season        = most recent season with >= 10 games, else the player's
                      best-sample season (most games played)
dimension groups    = statline (9 categories, avg z-score), trajectory
                      (season-over-season delta per category, avg z-score), age
distance(target, c) = sqrt( Σ w_g · (targetGroup_g - candGroup_g)² / Σ w_g )
similarity(c)        = 1 / (1 + distance)
growth(cat)          = next/match ratio, clamped [0.3, 2.5] for counting stats;
                       next-match delta, clamped [-0.3, 0.3] for FG%/FT%
projected(cat)        = base(cat) × Σ( w_c · growth_c(cat) )   where w_c = sim_c² / Σsim²
```

Only comps with an **observable next season** count (a candidate with no next-season
row is dropped, not scored as a zero-growth washout like NFL does) — the historical
backfill is still filling in, so "no row yet" doesn't reliably mean "left the league."
If every comp drops out, the projection flatly repeats the target's own rate.

**Dimension-group weights** (`projConfig.WeightStatline/WeightTrajectory/WeightAge`)
default to 2.0/1.0/1.0 — the values the live handler has always used — but are now a
tunable config the batch path's autotuner sweeps (see "Backtesting & Tuning" below).

### Two additions from this pass's backtest-driven tuning

Both are config knobs defaulting to an exact no-op, introduced and validated the same
"introduce behind a flag, sweep, keep only if validation improves" way NFL's analogous
knobs were (`docs/algorithm-review.md` §6-7). **Both are live in the current tuned
config** (`nba_projection_config.json`) as of this pass — see
[nba-algorithm-review.md](nba-algorithm-review.md) for the backtest evidence.

- **Growth-baseline shrinkage** (`GrowthShrinkageK`, ports
  [bayesian-shrinkage.md](stats/bayesian-shrinkage.md)'s growth-rate extension /
  NFL's `growth_baseline.go`): when a target's real comp pool is thin (small `Σsim²`),
  the comp-derived growth rate is shrunk toward a position-conditioned baseline growth
  — the plain mean growth per category across every candidate sharing the target's
  position, computed once per batch run. `k = 0` is an exact no-op; the current tuned
  value is **20**.
  ```
  effective_growth(cat) = (Σsim² · realGrowth(cat) + k · baseline(cat)) / (Σsim² + k)
  ```
- **Rate-stat shrinkage** (`RateStatShrinkageK`, ports
  [bayesian-shrinkage.md](stats/bayesian-shrinkage.md) directly): each season's FG%/FT%
  is pulled toward the position-group mean before any z-scoring or distance
  computation sees it, weighted by that season's own attempts vs. `k`. `k = 0` is an
  exact no-op; the current tuned value is **50**.
  ```
  shrunk_pct = (attempts · raw_pct + k · position_mean_pct) / (attempts + k)
  ```

### Ranking the projections

Once every eligible player has a projected stat line, `our_rank` is an **overall**
z-score fantasy-value rank (no position split — NBA category-league rankings are
published overall, and the metric is already standardized across positions):

```
z(p, cat) = (projected(p, cat) - populationMean(cat)) / populationStdev(cat)
          -- negated for turnovers, so higher always means better
score(p)  = Σ z(p, cat) over all 9 categories
rank(p)   = 1-based position of p in score-descending order
```

This is the same formula the frontend's `lib/nbaRanking.ts` (`nbaFantasyZScore`/
`nbaComputeRanks`) already used for actual stats — ported to Go
(`cmd/project-nba/projection.go`'s `fantasyZScoreSums`/`ranksFromScores`) so it can run
server-side over projected stats too.

## Backtesting & Tuning

New this pass — NBA had no backtest harness before. `cmd/project-nba -backtest -from
-to` treats a past season N as "the future": projects every player using only seasons
`< N` (temporal integrity — `loadSeasonAggregates`'s `asOfSeason` cutoff), then compares
the projected rank to the rank those same players actually achieved in season N (real
`nba_player_stats` totals, same z-score formula, same population restriction — players
with ≥20 games).

**Primary metric: Spearman rank correlation** between projected and actual overall
fantasy-value score, over players present in both — the same choice NFL's backtest
makes and for the same reason (draft decisions are ordinal; what matters is getting the
order right). RMSE/MAE on projected-vs-actual points/game are reported as a secondary
diagnostic, not the tuning objective.

`cmd/project-nba -autotune -from -to -train-to` does coordinate descent over all five
tunable knobs (the three dimension weights plus both shrinkage constants) in one
repeating pass — re-sweeping the whole set until a full pass finds no further
improvement, since the shrinkage knobs change the effective comp distances the weights
are balancing (NFL's autotune, by contrast, sweeps each dimension once except its own
per-position weight block). The winning config is then scored against the untuned
default on a **held-out validation range**; if the tuned config doesn't beat default
there, it's discarded and the default is kept — the same overfitting guard NFL's
`runAutotune` uses.

See [nba-algorithm-review.md](nba-algorithm-review.md) for the actual backtest numbers
this pass produced and the reasoning behind what was kept.

## Limitations

Explicit, not gaps to silently fill:

- **No outcome distribution.** NFL's P10/P50/P90 band
  ([uncertainty-quantification.md](stats/uncertainty-quantification.md)) has no NBA
  equivalent — a projection here is a single point estimate.
- **No level calibration.** NFL found and corrected a uniform ~12% over-projection bias
  ([algorithm-review.md](algorithm-review.md) §7.7-7.8); not investigated for NBA.
- **No grade dimension, no rookie/draft-slot path.** A true rookie with no prior NBA
  season has no profile to grow — they simply don't appear in `nba_projections` until
  they have one.
- **No opponent/strength-of-schedule adjustment**
  ([strength-of-schedule.md](stats/strength-of-schedule.md) has no NBA port).
- **No recency-weighted blending of the target's own recent seasons**
  ([recency-weighted-profiles.md](stats/recency-weighted-profiles.md) has no NBA port)
  — the base-season selection rule (most recent, or best-sample if thin) is a simpler,
  discrete choice, not a continuous blend.
- **Situational context (injuries, trades, depth-chart battles) never reaches the
  projection number** — same structural blind spot NFL's divergence layer documents;
  see the Damian Lillard worked example in
  [nba-consensus-rankings.md](stats/nba-consensus-rankings.md).

## Running

```bash
cd backend
go run ./cmd/project-nba -project -season 2026                         # batch-project every eligible player
go run ./cmd/project-nba -import-consensus f.json -season 2026          # import external rankings/ADP
go run ./cmd/project-nba -consensus-diff -season 2026                   # diff our rank vs. consensus
go run ./cmd/project-nba -backtest -from 2010 -to 2025                  # backtest across history
go run ./cmd/project-nba -autotune -from 2010 -to 2025 -train-to 2020   # tune + validate, saves nba_projection_config.json
```

## Database Tables

- `nba_projections` — one row per `(espn_id, target_season)`: the batch-computed
  projected stat line, `comp_count`, and `our_rank`. Read by `/api/nba/rankings` and
  the consensus-diff step. Independent of `nba_player_stats`/the live handler's
  per-request computation.
- `nba_consensus_rankings`, `nba_projection_divergences` — see
  [nba-consensus-rankings.md](stats/nba-consensus-rankings.md).
- `nba_backtest_results` — one row per backtested target season: `rank_correlation`,
  `rmse`, `mae`, `player_count`, `config_used` (full `projConfig` snapshot). Telemetry
  only, mirrors (trimmed) NFL's `nfl_backtest_results`.
- `nba_projection_config.json` — the tuned `projConfig`, read by both `-project` and
  `-backtest`/`-autotune`. All-defaults is an exact no-op relative to the pre-this-pass
  live projection behavior.
