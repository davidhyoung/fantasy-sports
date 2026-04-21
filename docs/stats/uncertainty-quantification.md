# Uncertainty Quantification in Projections

## Problem it solves

The current projection system returns a single point estimate per stat (e.g. "1,240 rushing yards") plus a scalar `confidence` ∈ [0, 1]. But `confidence = 0.75` doesn't tell a drafter the **range** of plausible outcomes — is this player's 90th-percentile season 1,400 yards or 1,900? Two players with identical projections and identical confidence scores can have radically different downside risk. Draft decisions, trade evaluations, and keeper decisions all hinge on that spread, not just the mean.

## Technique

Replace point estimates with a **distribution** — at minimum, a mean and a standard deviation; better, a set of quantiles (P10, P50, P90) derived from the comp set itself.

For the comp-based projection already in use, the machinery is almost free because we already have weighted comp trajectories:

```
weights_i = similarity_i² / Σ similarity²
μ        = Σ weights_i × stat_i           # weighted mean (current projection)
σ²       = Σ weights_i × (stat_i − μ)²    # weighted variance
σ        = sqrt(σ²)
```

For quantiles, sort the comp outcomes, walk the cumulative weight, and interpolate:

```
P_q = stat value where cumulative weight first exceeds q
```

So with 15 comps, a target's **projected rushing yards** becomes `{P10: 780, P50: 1220, P90: 1680}` rather than `1220 ± handwave`.

## Assumptions

- The comp set is a reasonable sample from the target player's outcome distribution. (Same assumption that underpins the existing comp-based point estimate.)
- Comp outcomes are independent enough that the empirical distribution is meaningful. (Loosely true; tight era clusters can violate this.)
- Similarity² weighting is appropriate for variance as well as mean — it is, because the same "these players are more relevant" logic applies.

## When it applies in this codebase

- `cmd/projections/main.go` — in the projection step, alongside each `projected_stats` entry, also emit `projected_stats_p10`, `projected_stats_p50`, `projected_stats_p90` (and/or `projected_stats_stdev`).
- `migrations/` — new migration to add these columns to `nfl_projections` (JSONB is fine; they mirror the shape of `projected_stats`).
- `internal/handlers/projections.go` — `GetProjectionDetail` returns the quantiles; `ListProjections` can still return just the median + stdev to keep payload small.
- `frontend/src/pages/player-detail/` — render a range bar or small sparkline for projection uncertainty. A player whose P10–P90 spans "WR2 to bust" is visibly riskier than one whose range is "WR1 to high-end WR2."
- `internal/handlers/draft_values.go` — downstream, auction values should factor in variance (risk-adjusted VOR), not just the mean projection.

## Worked example

Two WRs projected to identical 1,050 receiving yards (current point estimate):

| Player | Comp count | Mean | Stdev | P10 | P50 | P90 |
|---|---|---|---|---|---|---|
| WR X (consistent archetype) | 22 | 1050 | 140 | 880 | 1050 | 1230 |
| WR Y (unique, volatile) | 6 | 1050 | 310 | 640 | 1050 | 1470 |

Both have `confidence ≈ 0.6`, but WR Y's 10th-percentile outcome (640 yards) is a fantasy-irrelevant season, while WR X's floor (880) is still a useful starter. A drafter picking in Round 3 should weight WR X higher than the point estimates alone would suggest.

## How to validate it's working

- **Calibration check in `cmd/projections/backtest.go`**: for historical seasons, does P10 actually capture ~10% of realized outcomes below it and P90 ~10% above? If the quantiles are systematically wide or narrow, the variance estimator is biased.
- **Proper scoring rule (CRPS — Continuous Ranked Probability Score)**: compare CRPS of the distributional projection against the point-estimate baseline on held-out seasons. Lower is better.
- **UI sanity**: for a well-known "consistent" player (e.g. a bellcow RB) the range should visibly be tighter than for a "boom/bust" player (deep-threat WR with few targets).

## Tradeoffs

- **Bandwidth**: the payload grows (3–5× more numbers per projection). Mitigate by returning quantiles only in detail endpoints.
- **Small comp sets give unstable variance.** A player with 3 comps has a noisy σ; report it but flag it (e.g. greyed out when `comp_count < 8`).
- **User interpretation**: fantasy players are used to point estimates and ranks. Introducing ranges requires UI design thought — badges, error bars, or "floor/ceiling" labels that are easier to read than raw P10/P90.
- **Doesn't replace `confidence`**. Confidence captures "how much we trust the projection process at all"; uncertainty captures "how wide the outcome distribution is." A player can have high confidence in a high-variance projection (we're confident he's a boom/bust).

## References

- Gneiting & Raftery, *Strictly Proper Scoring Rules, Prediction, and Estimation* (JASA, 2007) — foundational for CRPS and calibration.
- FiveThirtyEight's NFL QB Elo and CARMELo projections — public examples of surfacing P25/P75 alongside point estimates.
- PECOTA (Nate Silver, *Baseball Prospectus*) — the canonical fantasy-sports example of comp-based quantile projections.
