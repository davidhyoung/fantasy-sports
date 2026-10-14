# NBA Algorithm Review — Backtest-Driven Tuning (October 2026)

The NFL analog is [algorithm-review.md](algorithm-review.md) — a structured log of what
was tried, the numbers, and why each change was kept or discarded. This is NBA's first
entry, since NBA had no backtest harness at all before this pass. Same discipline NFL's
review follows throughout: "introduce behind a flag, validate before replacing," never
tune on the same range you validate against.

## Method

`cmd/project-nba -backtest`/`-autotune` against the full local history (31 seasons,
1996-2026, 1,059,309 `nba_player_stats` rows). Train range 2010-2020 (chosen so every
training season still has a reasonable comp pool behind it — seasons before 2010 have
few enough player-seasons on record that the candidate pool is thin and the backtest
metric itself gets noisy, visible in the 2010/2011 rows below), validation range
2021-2025, held out from every tuning decision.

Primary metric: mean Spearman rank correlation between projected and actual overall
fantasy-value rank, across players with ≥20 games actually played in the target season.

## 1. Baseline — current (pre-tuning) fixed weights

```
go run ./cmd/project-nba -backtest -from 2010 -to 2025
```

| season | n | rank_corr | rmse (pts/g) | mae (pts/g) |
|---|---|---|---|---|
| 2010 | 11 | 0.9091 | 4.56 | 3.42 |
| 2011 | 12 | 0.1329 | 8.42 | 5.59 |
| 2012 | 18 | 0.6533 | 5.78 | 4.16 |
| 2013 | 25 | 0.8100 | 3.89 | 2.36 |
| 2014 | 31 | 0.8145 | 3.35 | 2.71 |
| 2015 | 44 | 0.7790 | 4.24 | 2.93 |
| 2016 | 60 | 0.8806 | 3.15 | 2.43 |
| 2017 | 81 | 0.7484 | 4.45 | 2.96 |
| 2018 | 116 | 0.6599 | 6.05 | 4.04 |
| 2019 | 133 | 0.7796 | 3.79 | 2.82 |
| 2020 | 160 | 0.8097 | 3.37 | 2.57 |
| 2021 | 186 | 0.8031 | 3.17 | 2.48 |
| 2022 | 234 | 0.8212 | 3.23 | 2.47 |
| 2023 | 273 | 0.8347 | 3.19 | 2.43 |
| 2024 | 332 | 0.8302 | 3.09 | 2.36 |
| 2025 | 380 | 0.7436 | 3.82 | 2.84 |
| **mean** | | **0.7506** | | |

2011 (n=12, rank_corr=0.13) is a small-sample outlier — the candidate pool that early
in the backfill is thin enough that one or two mismatched comps can swing the whole
season's correlation. Not treated as evidence of anything beyond "small samples are
noisy," consistent with how every other season in the 2012-2025 range sits comfortably
in the 0.65-0.91 band.

## 2. Dimension-weight sweep (train 2010-2020)

Swept `WeightStatline`/`WeightTrajectory`/`WeightAge` (grid `{0.5, 1.0, 1.5, 2.0, 2.5,
3.0}`) alone first, before adding the shrinkage knobs, to see how much of any gain
traces to weighting vs. shrinkage.

| config | train score (2010-2020) | validation score (2021-2025) |
|---|---|---|
| default (2.0/1.0/1.0) | 0.7252 | 0.8066 |
| statline=0.5 (else default) | 0.7304 | 0.8078 |

**Kept provisionally** — beats default on both train and validation, though the
validation gain (+0.0012) is barely above noise on its own. Lowering the statline
weight means trajectory and age now dominate the comp search relative to raw statline
resemblance — a real, if modest, signal that "how a player is trending and where they
are in their career" predicts next-season growth better than "who has a similar
statline right now" alone.

## 3. Shrinkage knobs added (two documented gaps from the Wiki's "Not built yet" list)

Added `GrowthShrinkageK` (growth-baseline shrinkage, ports NFL's `growth_baseline.go`)
and `RateStatShrinkageK` (FG%/FT% shrinkage toward the position mean, ports
[bayesian-shrinkage.md](stats/bayesian-shrinkage.md)) — both config knobs, both
`0 = exact no-op`, both validated the same way before being kept.

Joint coordinate descent (weights + both shrinkage knobs, repeating the whole 5-knob
sweep until a full pass finds no further train-score improvement — it converged after
one pass):

| step | train score | what moved |
|---|---|---|
| baseline (default) | 0.7252 | — |
| weight_statline → 0.5 | 0.7304 | dimension weight |
| growth_shrinkage_k → 2.0 | 0.7409 | shrinkage |
| growth_shrinkage_k → 5.0 | 0.7454 | shrinkage |
| growth_shrinkage_k → 20.0 | 0.7549 | shrinkage |
| rate_stat_shrinkage_k → 50.0 | 0.7595 | shrinkage |
| *(second pass: no further improvement — converged)* | | |

**Final validation comparison:**

| config | validation score (2021-2025, held out) |
|---|---|
| default | 0.8066 |
| tuned (statline=0.5, growth_shrinkage_k=20, rate_stat_shrinkage_k=50) | **0.8151** |

Tuned beats default on held-out validation (+0.0085) — **kept**. Saved to
`nba_projection_config.json`.

## 4. Full-range backtest with the final tuned config

```
go run ./cmd/project-nba -backtest -from 2010 -to 2025
```

| season | rank_corr (default → tuned) | rmse (default → tuned) |
|---|---|---|
| 2010 | 0.9091 → 0.9091 | 4.56 → 4.01 |
| 2011 | 0.1329 → 0.3077 | 8.42 → 9.79 |
| 2012 | 0.6533 → 0.7276 | 5.78 → 6.54 |
| 2013 | 0.8100 → 0.8315 | 3.89 → 4.62 |
| 2014 | 0.8145 → 0.8375 | 3.35 → 4.78 |
| 2015 | 0.7790 → 0.8021 | 4.24 → 4.11 |
| 2016 | 0.8806 → 0.8880 | 3.15 → 3.76 |
| 2017 | 0.7484 → 0.7622 | 4.45 → 5.09 |
| 2018 | 0.6599 → 0.6584 | 6.05 → 5.68 |
| 2019 | 0.7796 → 0.7990 | 3.79 → 4.63 |
| 2020 | 0.8097 → 0.8311 | 3.37 → 4.02 |
| 2021 | 0.8031 → 0.8101 | 3.17 → 3.76 |
| 2022 | 0.8212 → 0.8496 | 3.23 → 3.41 |
| 2023 | 0.8347 → 0.8387 | 3.19 → 3.84 |
| 2024 | 0.8302 → 0.8311 | 3.09 → 3.64 |
| 2025 | 0.7436 → 0.7463 | 3.82 → 4.16 |
| **mean** | **0.7506 → 0.7769** | |

14 of 16 seasons improved or held steady on rank correlation; 2018 moved fractionally
backward (0.6599 → 0.6584, within noise). **RMSE got worse in most seasons** even as
rank correlation improved — shrinkage pulls point estimates toward a population mean,
which costs some per-player accuracy for stars and thin-sample players alike, while
improving the *ordering* that actually matters for a draft rank. The same distinction
NFL's §7.7-7.8 draws between "ranks unaffected" and "point totals" — here shrinkage
does trade off one for the other, which is the expected and accepted cost of the
technique, not a regression. No level-calibration pass exists yet for NBA
(see [nba-projection-algorithm.md](nba-projection-algorithm.md)'s Limitations) to
correct the point estimates back down the way NFL's `calibration.go` does; that's a
reasonable next-pass candidate if displayed point totals start to look inflated.

## Bottom line

Two previously-unimplemented, explicitly-documented gaps (growth-baseline shrinkage,
rate-stat shrinkage) are now implemented, validated by a real backtest across 16
seasons rather than guessed, and live in the current `nba_projection_config.json`.
The dimension-weight change is real but small on its own; the shrinkage knobs account
for most of the gain. Both knobs default to an exact no-op and were only kept because
they beat the untuned baseline on a held-out range they were never tuned against — the
same discipline NFL's review has followed throughout.

**Not pursued this pass** (candidates for a future iteration, per
[nba-projection-algorithm.md](nba-projection-algorithm.md)'s Limitations): level
calibration on displayed point totals, an outcome distribution (P10/P50/P90), and
re-investigating whether age-conditioning the growth baseline (not just position) finds
anything the coarser position-only baseline misses.
