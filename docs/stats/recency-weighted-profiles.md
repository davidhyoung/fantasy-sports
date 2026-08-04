# Recency-Weighted Target Profiles

> **Status: implemented behind a no-op default (August 2026)** in
> `backend/cmd/projections/recency_blend.go`, wired into both `computeProjections`
> (`main.go`) and `projectSeasonBacktest` (`backtest.go`). `TargetBlendDecay = 0`
> (seeded default) is exact — ships with zero behavior change until `-autotune`
> finds a nonzero value that beats the default on held-out validation.

## Problem it solves

The comp engine's "base season" is a single-season snapshot: whatever
`nfl_player_season_profiles` row exists for `targetSeason - 1`. Per-game rates
are stored, and `games_played` isn't a comp-matching dimension anywhere — so a
player who played 4 games at an elite rate (e.g. injury-shortened) and a player
who played 17 games at the same rate look *identical* to the comp engine, even
though one estimate rests on 4x less evidence. The larger, healthy season a
player had the year before contributes nothing, no matter how much more stable
a signal it is.

This is a different failure mode from the one `bayesian-shrinkage.md` solves.
That entry pulls a *small sample toward the population mean* (shrinking toward
other players). This one blends *two observations of the same player across
time* — closer to an exponentially-weighted moving average than to
Efron-Morris shrinkage, and worth keeping conceptually separate even though
both exist to make thin samples more trustworthy.

## Technique

For the target player only (never for comp candidates, which stay single-season
snapshots so the historical comp pool is unaffected), blend the base season
with the immediately preceding season, weighted by games played and discounted
by how many seasons old the observation is:

```
weight_i = games_i * decay^age_i         (age_i = 0 for base season, 1 for prior)
blended_field = (weight_base * field_base + weight_prior * field_prior) / (weight_base + weight_prior)
```

Applied to every per-game rate field and every z-score (safe to blend directly
— z-scores are globally normalized across all seasons, so a z-score of 1.0
means the same thing regardless of which season it came from). Point-in-time
facts (age, years_exp, draft_number, position_group, height, weight) are *not*
blended — they're copied from the base season unchanged.

`decay` is the one tuning parameter:
- `decay = 0` → prior season's weight is exactly zero → **exact no-op**,
  identical to today's single-season behavior, no conditional branching needed.
- `decay → 1` → prior season counted at full games-weight, same as the base
  season (a genuinely older, potentially stale signal weighted as if it were
  current).

## Assumptions

- The window is fixed at exactly 2 trailing seasons (base + immediately prior).
  Extending to N seasons with continued exponential decay is a natural future
  increment, not implemented here — this was scoped specifically to "last 2
  seasons."
- A player's role/talent is reasonably continuous across adjacent seasons. This
  breaks for a genuine, permanent role change (e.g. a backup who just became a
  starter) — blending in a materially different prior-season role would pull
  the profile toward a role that no longer applies. The situational-notes layer
  (`docs/stats/consensus-ensemble.md`) is the intended mitigation for cases like
  that, not this technique.
- Applied *universally* (every target, every run), not conditionally only when
  the base season is short — a deliberate choice over a narrower "only blend
  when games_played is low" patch, since the same logic (recent evidence should
  count in proportion to how much of it exists) is arguably correct for every
  player, not just injury-shortened ones.

## When it applies in this codebase

- `backend/cmd/projections/recency_blend.go` — `blendTargetProfile`, called
  from `computeProjections` (main.go) and `projectSeasonBacktest` (backtest.go)
  at target-selection time, before comp search.
- Does **not** touch `buildTrajectory`/`computeSimilarity`'s comp-candidate
  side — comps remain single-season snapshots, so this change is isolated to
  how the target's own input profile is constructed.

## Worked example

Malik Nabers, 2025 base season: 4 games, `fpts_ppr_pg=12.1`, `target_share=0.271`.
2024 season: 15 games, `fpts_ppr_pg=17.7`, `target_share=0.357`. With `decay=0.5`:

```
weight_2025 = 4 * 0.5^0 = 4
weight_2024 = 15 * 0.5^1 = 7.5
blended target_share = (4*0.271 + 7.5*0.357) / (4+7.5) = 0.327
```

vs. the raw `0.271` from 4 games alone — meaningfully more representative of
his actual role, without discarding the current season's signal entirely (it
still counts, just proportional to how much evidence it represents).

## How to validate it's working

1. **No-op check**: with `decay=0`, `nfl_projections` rows must be byte-identical
   to before this change existed — proves the default is truly inert.
2. `-backtest` with defaults should reproduce previously recorded metrics
   exactly (same reason).
3. `-autotune`'s existing accept/reject gate (tuned config must beat
   `defaultConfig()` on held-out validation seasons) is the real test — no new
   validation machinery needed, this plugs into what already exists.
4. Face-validity: re-run `-consensus-diff` after tuning and check whether
   players whose base season was injury-shortened (the original motivating
   case) show smaller divergence from consensus than before.

## Tradeoffs

- **One new global tuning parameter** (`decay`) that affects every player's
  projection, not just edge cases — higher blast radius than a narrower
  "only blend when games are low" patch would have had, in exchange for a more
  principled, uniformly-applied rule.
- **Can blur a genuine breakout or decline** for a player whose role changed
  for real between the two blended seasons (see Assumptions) — this is the
  same tradeoff any moving-average technique makes.
- **Adds one more thing to autotune**, lengthening the coordinate-ascent sweep
  and adding one more axis where overfitting-to-training-noise is possible
  (mitigated by the existing validation-must-beat-default guard, same as every
  other tunable).

## References

- Exponentially-weighted moving averages are standard in time-series forecasting
  for exactly this reason — recent observations should count more, but not to
  the total exclusion of a slightly older, still-relevant observation. No
  single canonical citation; the formula here is the simplest defensible form
  (linear games-weighting × exponential recency decay), not a novel derivation.
