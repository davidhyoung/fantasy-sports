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

## 7. Auction-divergence analysis (August 2026)

Follow-up to §6, prompted by putting our auction value and a consensus value side
by side on `/draft-prep`. §6 investigated the largest *rank* gaps player by player;
this pass asked whether the gaps are systematic, and — crucially — whether the
market or the model is right where they disagree. Conclusion: **agreement is much
higher than the raw gaps suggest, the one systematic disagreement favours us, and
the real calibration error is somewhere the divergence layer cannot see.**

### 7.1 Agreement is high; the headline gaps are concentrated

62 players had 2026 PPR consensus coverage (sources publish ~top-100: QB 8 deep,
TE 6, RB/WR 24 each). Ranked within position on both sides:

| measure | value |
|---|---|
| median rank delta | +1.0 |
| |delta| > 10 | 8 of 62 |
| ordering agreement, full pipeline vs consensus (Spearman ρ) | +0.91 RB, +0.85 WR, +0.83 QB |
| same, last-season PPG only | +0.84 RB, +0.70 WR, **+0.07 QB** |

The comp machinery earns its keep: ranking QBs on last season alone has essentially
no relationship to consensus (ρ +0.07), while the pipeline reaches +0.83.

Two things inflate the *apparent* disagreement and are not model error:
- **Coverage depth.** Sources rank 8 QBs and 6 TEs. "Not on their list" is not a
  disagreement, it's the end of the list.
- **Population mismatch.** Our positional rank is over all ~471 projected players;
  theirs is over their top-N. Re-ranking both sides over the covered set drops the
  mean delta from +2.85 to **+0.33** (sd 7.0 → 3.5, outliers 8 → 2). The residual
  offset is real, though: it means our top-N contains players the market omits
  (RJ Harvey, Wan'Dale Robinson, Harold Fannin Jr. — the last on `comp_count = 1`).

### 7.2 The one systematic disagreement: down seasons

Correlating the gap with model features found three signals — base-season PPG
(r −0.52), comp-pool size (+0.39), player grade (−0.41) — which collapse into one:
`corr(base_ppg, comp_count) = −0.57`, `corr(base_ppg, grade) = +0.47`. They all
measure "how good was he last year". Split by direction:

| base season vs the one before | n | mean rank delta |
|---|---|---|
| fell ≥2 ppg | 17 | **+4.6** (we rank them well below the market) |
| flat ±2 | 20 | +2.1 |
| rose ≥2 ppg | 14 | +0.9 (we agree with the market) |

So we do **not** chase career years — we and the market agree on breakouts. We mark
down decliners much harder than the market does. Justin Jefferson (−7.4 ppg, our
WR17 vs consensus WR6), Chuba Hubbard and David Montgomery (both +12) are this
cohort, not three unrelated bugs.

### 7.3 Why the §6 lever doesn't fix it

The 2026 projections already ran with `target_blend_decay = 0.5` (config written
38 seconds before the projection run), so the recency blend §6 implemented is live.
Sweeping the decay 0 → 2.0 on a blended-PPG proxy moves agreement for the decliner
cohort by less than 0.02 ρ (0.798 → 0.804 → 0.786). **Reweighting the previous
season cannot close this gap**, because the disagreement is not about how much
weight last season deserves — it is about *why* the season was down, which no
weighting scheme can see. That information lives in the situational-notes layer.

### 7.4 Who is right? History says we are

Every player-season 2015–2024 with three consecutive measured seasons (n = 1328),
asking what happens the year after a ≥2 ppg drop (n = 394):

| | value |
|---|---|
| median share of the drop recovered next season | **−4%** |
| ended above their pre-decline level | 13% |
| **fell further** | **52%** |
| mirror case: share of a ≥2 ppg *gain* given back | **58%** |

The result holds across every age band (≤24: 6%, 25–26: −2%, 27–28: 1%, 29+: −12%)
and whether the down year was injury-shortened (5%) or a full season (−8%). The
most favourable slice anywhere — WRs aged ≤27 after a full down season — recovers
10%, nowhere near the market's implied restoration. Every row requires a following
season with ≥6 games, so players who washed out entirely are excluded: the finding
is **biased toward recovery** and still shows none.

Declines are sticky; gains are transient. The market's buy-low premium on decliners
is not supported by ten years of this dataset.

### 7.5 The real calibration error is where we and the market agree

Checking our shipped 2025 projections against what actually happened (n = 143,
players with 2023 + 2024 profiles and a measured 2025):

| 2024 vs 2023 | n | our bias (proj − actual) | MAE | verdict |
|---|---|---|---|---|
| fell ≥2 ppg | 41 | **−0.35** | 2.58 | well calibrated (21/41 too high, 20/41 too low) |
| flat ±2 | 57 | −0.11 | 3.05 | well calibrated |
| rose ≥2 ppg | 45 | **+1.26** | 2.75 | **we project breakouts too high** |

Our discount of decliners is a coin flip on direction — correctly calibrated, not
excessive. The measurable error is on **risers**, and it lines up with the 58%
give-back above: we under-regress breakout seasons by roughly a point per game.

The divergence layer could never have found this, because on risers the market
shares our bias (mean delta +0.9). **Consensus agreement and accuracy are different
targets, and this is the case that separates them.**

### 7.6 Recommendations

1. **Do not chase consensus on decliners.** Two independent tests say our lower
   ranking is better supported. Closing that gap would make the board agree with the
   market and get worse.
2. **Investigate asymmetric regression of breakout seasons.** [REFUTED by the sweep — see §7.7.] The one quantified
   error. The existing levers are symmetric — a blend that pulls a riser down also
   pulls a decliner up, and decliners are already right. The targeted change is a
   *directional* blend weight (regress gains harder than drops), validated by
   backtest bias per cohort across 2015–2024, not by one season. Target: riser bias
   +1.26 → ~0 with decliner bias unmoved. **Not implemented** — one season of
   evidence is not enough to justify touching the engine.
3. **Accept as criteria differences, not defects:** ADP is a market-clearing price
   (it carries name-brand demand and positional-run dynamics) while we produce a
   points projection; source list depth; and `proj_games = 17` for every player —
   we project points *if healthy* while the market discounts injury-prone players.
   The last is worth labelling in the UI, and is a candidate for the uncertainty
   band rather than the point estimate.

### 7.7 Sweep result (August 2026) — §7.6's recommendation is refuted

§7.5 inferred a breakout-specific over-projection from a single season (2025,
n = 143). Running it properly across 2015–2024 with temporal integrity
(`-cohort-bias`, which reports *signed* bias — the stored backtest metrics are
unsigned and cannot show skew) does not support it.

Startable players (base season ≥ 8 PPR/game), 1603 player-seasons:

| base season vs the one before | n | bias (proj − actual, per game) | MAE |
|---|---|---|---|
| rose ≥2 ppg | 501 | +1.47 | 3.15 |
| flat ±2 | 493 | **+1.73** | 3.18 |
| fell ≥2 ppg | 309 | +1.39 | 3.29 |
| **all** | 1603 | **+1.58** | 3.34 |

There is **no riser-specific bias**. Risers (+1.47) and decliners (+1.39) are
within noise of each other, and *flat* players are the worst cohort. The 2025
slice was too small; the asymmetry it appeared to show was not real.

**What is real is a global over-projection**, positive in all seven seasons
sampled (+0.48 to +1.90) and hitting 69% of players. Split by projected level:

| projected | n | bias | as % of projection |
|---|---|---|---|
| < 8 ppg | 62 | +0.61 | 8% |
| 8–12 | 584 | +1.22 | 12% |
| 12–16 | 525 | +1.54 | 11% |
| 16+ ppg | 432 | +2.27 | 12% |

The bias is **proportional, not additive** — a near-constant ~11–12% of whatever
we projected. That distinction decides whether it matters:

- **Ranks are unaffected.** Scaling every projection by the same factor preserves
  order.
- **Auction values are unaffected.** `VOR = k·points − k·replacement = k·(points −
  replacement)`, and each price is a *share* of the VOR pool, so a common factor
  cancels out of the whole draft-value pipeline.
- **Displayed point totals are inflated by ~12%** — "Proj Pts", season totals on
  the player page, anything absolute.

`TargetBlendDecayUp` (added to test the hypothesis) does reduce riser bias —
+1.47 → +1.09 at 0.7, → +0.79 at 0.9 — but it is the wrong instrument, and the
sweep shows why: decliners are equally over-projected and the knob leaves them
untouched by construction. It improves a metric by shrinking one cohort in a
model that is uniformly too high. **It stays at 0 (no-op).** The knob and the
`-cohort-bias` mode are kept so the next hypothesis can be tested the same way.

**Revised recommendation.** Not an engine change: a ~0.89 calibration factor on
*displayed* projected points, which by the algebra above moves no rank and no
auction value. Two caveats before applying it:

- Part of the 12% is definitional, not error. Our per-game number is an
  if-healthy, in-role rate; actuals include games played hurt, benchings and
  blowouts. Calibrating that away makes the display honest but the projection
  less interpretable as "his rate when right".
- Survivorship makes 12% a floor, not a ceiling: actuals require ≥4 games, so
  players who lost their job entirely are excluded from the comparison.

The honest summary of §7 as a whole: **the model's ordering is sound and agrees
with the market; its levels are ~12% hot; and the cohort story that looked
compelling in one season did not survive ten.**

### 7.8 Applied (August 2026) — level calibration

The §7.7 recommendation is implemented: `cmd/projections/calibration.go` scales
every projected quantity by a per-position factor, seeded **uniform at 0.884**
(`projection_calibration` in `projection_config.json`). Applied at the end of
`computeWeightedProjection`, so the production and backtest paths share it.

Measured effect across 2015–2024 (`-cohort-bias -min-base-ppg 8`, n = 1603):

| | before | after |
|---|---|---|
| bias (proj − actual, per game) | +1.583 | **−0.000** |
| MAE | 3.337 | **2.988** |
| projections too high | 69% | 51% |
| bias by level | +8% to +12% | −3% to +3% |

Verified on the live 2026 board (471 players) that the draft math is untouched:
**0 position-rank changes, 0 auction-value changes, 0 consensus-value changes**,
and projected points scaled by exactly 0.884×. Two overall ranks swapped — both
zero-VOR, $1 players at the bottom, where `sort.Slice` is unstable and ties
shuffle between runs regardless.

**Residual, and the open decision.** A uniform factor is right on aggregate and
nearly exact for WR (0.988) and TE (0.996), but the bias was never uniform across
positions. After calibration:

| position | residual factor | reading |
|---|---|---|
| QB | 1.078 | now ~8% *under*-projected |
| RB | 0.950 | still ~5% over |
| WR | 0.988 | ✓ |
| TE | 0.996 | ✓ |

Uniform was chosen deliberately: per-position factors (QB 0.953, RB 0.840,
TE 0.880, WR 0.873) would calibrate each position exactly, but they reprice
positions *against each other* — QBs would get materially more expensive at
auction. That is a different decision from correcting a display, so it is left as
a one-line config edit rather than folded into this change.

`nfl_projections` for target season 2026 was recomputed. The 2025 target rows are
uncalibrated and unused by the UI; re-run `-project -season 2025` to bring them in
line if they are ever needed.
