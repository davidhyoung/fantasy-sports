# Algorithm Review — Gaps, Assumptions, and Improvement Plan (June 2026)

A structured audit of the projection engine (`cmd/projections/`), the grading engine
(`cmd/projections/grades.go`), and the ranking engine (`internal/services/ranking/` +
`internal/handlers/analysis.go`), conducted against `docs/projection-algorithm.md`,
`docs/ranking-algorithm.md`, and the statistical techniques library at `docs/stats/`.

Each finding lists where it lives, why it matters, and its disposition.

---

## 1. Bugs and doc/code divergences

### 1.1 Survivorship bias in comp trajectories ⚠️ (fixed)

**Where:** `cmd/projections/main.go` — `buildTrajectory` / `computeWeightedProjection`.

The projection doc claimed retired comps "contribute a zero trajectory," which would
naturally penalize archetypes with high washout rates. The code did the opposite: a
comp with no next-season profile returned an empty trajectory and was **skipped** in
the weighted growth average. Only comps who kept playing influenced the projection —
the textbook survivorship bias. The bias is largest exactly where it's most dangerous:
aging RBs and marginal players whose comps frequently never played again.

**Fix:** comps whose match season is old enough that a following season *could* exist
in the data, but doesn't, now contribute growth = 0 (washed out). Comps matched against
the most recent data season (no chance to have a next season yet) are still skipped.

### 1.2 Zero-comp players repeat last season verbatim (fixed)

**Where:** `cmd/projections/main.go` — `applyGrowth` returned `currentVal` when no
comps qualified.

The doc claimed a population-mean fallback. In reality a unique-profile player was
projected to exactly repeat their base season — no regression to the mean at all,
despite these being the *least* certain projections in the system. Fixed: zero-comp
projections now regress toward the position-group mean.

### 1.2b Backtests measured the wrong model ⚠️ (fixed)

**Where:** `cmd/projections/backtest.go` — comp construction in `runBacktest` and the
autotuner's `scoreConfig`.

Backtest comps were built without `MatchProfile`, but `getGrowth` reads
`c.MatchProfile["fpts_ppr_pg"]` and returns growth = 1.0 when it's zero (nil-map
lookup). So **every comp's growth was 1.0** and all historical backtests measured a
"repeat last season per-game" persistence baseline — not the comp engine. The
production path was unaffected, but every stored backtest metric and every autotune
decision before June 2026 evaluated a different model than the one shipping. Fixed by
extracting a single `projectSeasonBacktest` helper that mirrors the production path
(and is shared by backtest + autotune so they can't drift again). Pre-June-2026 rows
in `nfl_backtest_results` are not comparable with new ones.

### 1.3 Backtest results inserted in duplicate (fixed)

**Where:** `cmd/projections/backtest.go` — `runBacktest` called
`storeBacktestResults(ctx, pool, allResults, …)` **inside** the per-season loop with
the accumulated slice, so season N's rows were re-inserted once for every subsequent
season. `nfl_backtest_results` accumulated quadratic duplicates, silently corrupting
any aggregate query over it.

### 1.4 Autotune never used its validation set (fixed)

**Where:** `cmd/projections/backtest.go` — `runAutotune`.

The comment said validation seasons "pick the best config (prevent overfitting)", but
coordinate-descent moves were accepted purely on training score and the final config
was saved unconditionally; validation RMSE was only logged. Fixed: the tuned config is
compared against the default config on the validation seasons and the winner is saved.

### 1.5 Stale documentation (fixed)

Both algorithm docs had drifted badly from the code:

- `projection-algorithm.md` described per-stat similarity weights; the code uses named
  **dimension groups** (`passing`, `rushing`, `value`, `physical`, `context`, `grade`)
  with one weight per group, averaged z-scores within a group, and a tuned config in
  `projection_config.json`.
- The documented confidence formula (comp count 30 / similarity 25 / completeness 20 /
  history 15 / games 10) didn't match the implemented one (similarity 25 / comp count
  20 / agreement 25 / sample depth 15 / data quality 15).
- The doc said each stat is projected from its own comp trajectories; the code projects
  fantasy points per game and scales every component stat by the same growth ratio.
- `ranking-algorithm.md` said all categories are weighted equally; the code applies
  CV × FA-scarcity weights. The doc's "future enhancements" list (position-relative
  scores, points-league support) had already shipped, and the VORP/replacement-level
  mode wasn't documented at all.

---

## 2. Statistical gaps (now addressed)

### 2.1 No shrinkage on small-sample rates → `docs/stats/bayesian-shrinkage.md`

Profiles qualify with only `minGames = 4`. Rate stats (YPC, YPA, comp %, YPR, FG%,
EPA/play) computed on tiny denominators produce inflated z-scores, which contaminate
similarity matching, grades (a 20-carry 7.0-YPC backup outgrading a 300-carry
workhorse), and tags. Empirical-Bayes shrinkage toward the position-group mean, with
pull inversely proportional to attempts, is applied before z-scoring in both the
production (`computeZScores`) and backtest (`recomputeZScores`) paths.

### 2.2 Point estimates with no outcome distribution → `docs/stats/uncertainty-quantification.md`

The comp set already *is* an empirical outcome distribution, but only its weighted mean
was kept. Two players with identical projections and confidence can have radically
different downside risk. Weighted stdev + P10/P50/P90 of comp year-1 outcomes are now
computed and stored alongside the point estimate, with a calibration check (do ~10% of
realized outcomes fall below P10?) in the backtest.

### 2.3 No opponent adjustment → `docs/stats/strength-of-schedule.md`

Raw per-game stats treat 100 yards against the league's worst defense as equal to 100
against its best — and the error **compounds** in comp matching because historical
comps' stats are also unadjusted. `nfl_player_stats.opponent_team` makes a
defense-vs-position yield table derivable with no new data feed. Per-game production is
multiplicatively adjusted (capped) before season aggregation.

---

## 3. Known assumptions (accepted, documented)

These are deliberate simplifications — fine as long as they're explicit:

- **Stable team context.** Projections assume the player's team situation (pass volume,
  scheme) persists. Trades and coordinator changes invalidate this; `team_context`
  dimensions partially mitigate by matching comps with similar contexts.
- **Component stats scale with total fantasy points.** A projected +10% in PPR/G is
  applied uniformly to yards, TDs, receptions. Cheap and stable, but can't express
  "fewer yards, more TDs" development shapes.
- **Era pooling.** Z-scores are computed per position group across *all* seasons
  (1999–present) so scores are comparable across eras, at the cost of passing-era
  inflation: a league-average 2024 passing line z-scores above a league-average 2005
  line. SOS adjustment narrows this; full era normalization (per-season z-scores with
  cross-season anchoring) is future work.
- **17-game projection for everyone.** Durability/games-missed risk is not modeled in
  totals; per-game numbers are the primary output and evaluation basis.
- **Aging multipliers are coarse phase-level nudges** (±2–7%) layered on top of comp
  trajectories that already embed aging. Kept deliberately small to avoid
  double-counting; tuned by autotune.
- **Yahoo league rankings are point-in-time.** League ranking z-scores use whatever
  stat period is requested with no shrinkage — acceptable for full-season stats, noisy
  for early-season periods (noted in the shrinkage entry as future work).

## 4. Evaluation methodology

- **Primary tuning objective: per-game Spearman rank correlation.** Draft decisions are
  ordinal; what matters is ordering players correctly. Evaluating per-game (≥4 games
  actually played) removes unpredictable injury noise from the skill signal.
- **Season-total metrics still reported** (`eval_basis = 'total'` rows) because totals
  are what a drafter experiences.
- Backtests preserve temporal integrity: for target season N, only profiles from
  seasons < N are visible, and z-score/shrinkage priors are recomputed from that
  restricted set.

## 5. Out of scope this round

- Weekly in-season projections (opponent/weather/game-script aware).
- NBA stats decoupling (NBA leagues still rank via Yahoo-period stats).
- Hierarchical/mixed-effects modeling of player-within-team effects.
- Era normalization beyond SOS (see §3).

## 6. Backlog from consensus-divergence investigation (August 2026)

Investigating the largest gaps between our projections and external consensus
(`docs/stats/consensus-ensemble.md`) surfaced real algorithmic questions, distinct
from the diagnostic-layer fixes (within-position ranking, note scoping) already
applied. Two of the four are now **implemented, behind no-op defaults**, per the
"introduce behind a flag, validate before replacing" principle this file
already follows — pending an `-autotune` run to find whether nonzero values
actually improve held-out validation accuracy. The other two remain backlog.

- ✅ **Injury-shortened base season as sole comp input** (Malik Nabers) —
  **implemented**: `blendTargetProfile` (`recency_blend.go`) blends the base
  season with the immediately preceding one, games-weighted × a decay factor,
  applied to the target only (comp candidates unaffected). `TargetBlendDecay=0`
  is an exact no-op. See `docs/stats/recency-weighted-profiles.md` — a distinct
  technique from position-group shrinkage (blends two observations of the same
  player over time, not toward a population mean).
- ✅ **Comp pool collapses for statistically rare profiles** (Derrick Henry,
  `comp_count=2`) — **implemented**: the comp-derived growth rate is now
  shrunk toward an age/position-conditioned baseline growth rate
  (`growth_baseline.go`, `computeGrowthBaselines`), weighted as a pseudo-comp
  with weight `GrowthShrinkageK` (0 = no-op) flowing through the same
  weighted-sum machinery as real comps, so the point estimate and the
  uncertainty band stay consistent. `k` is a fixed autotune-sweepable constant
  rather than the dynamic per-run median `shrinkage.go` uses for rate stats —
  a deliberate, documented deviation (see `docs/stats/bayesian-shrinkage.md`'s
  growth-rate extension for why).
- **Team-context dimensions are entirely backward-looking** (already noted in
  §3 as "Stable team context"). The divergence layer's situational notes now
  give this a partial, diagnostic-only mitigation — a note like "player signed
  with a new team" can surface next to their divergence line — but the
  *projection number itself* still has no representation of a player's new
  team at all when they change teams via trade/free agency. A real fix would
  mean overriding or reweighting the context dimension group for known
  offseason moves, which is a bigger design decision than this backlog note.
- **Some team-level production collapses are still unexplained.** Three
  Philadelphia skill players (Barkley, A.J. Brown, DeVonta Smith) and Terry
  McLaurin (Washington) all show a real 2024→2025 team-offense decline
  (`team_fpts_pg`/`team_rush_yds_pg` down double digits) with no situational
  note explaining why — unlike Justin Jefferson/Minnesota, where the cause
  (QB competition) was already researched and captured. This is a research gap,
  not an algorithm gap: a targeted follow-up pass on "why did PHI/WAS offenses
  decline in 2025" would let the situational-notes layer do its job here too.
