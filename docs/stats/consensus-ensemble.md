# Consensus Divergence (External Signal Cross-Check)

> **Status: divergence-flag only (August 2026)**, non-mutating. `nfl_consensus_rankings`
> and `nfl_player_situational_notes` store external data; `nfl_projection_divergences`
> stores the computed gap between our projection and the outside world. Nothing here
> changes `nfl_projections` — this is a review/diagnostic layer, not an ensemble blend,
> until it has a season of retrospective validation behind it. A full investigation of
> every large divergence in the first real run found six overlapping root-cause
> clusters (below); the diagnostic-layer fixes are applied, the genuine algorithm
> questions are backlogged in `docs/algorithm-review.md` §6 pending a backtest.

## Problem it solves

The comp-based engine (`docs/projection-algorithm.md`) only ever sees *last completed
season's* stats — it has no way to know a player got traded in June, tore an ACL in
camp, lost a snap-share battle to a rookie, or that the market (expert rankings, ADP,
dynasty trade value) has already priced in information the model structurally cannot
see. Two different failure modes follow from this blind spot:

1. **Silent bias** — the model's rank for a player is quietly wrong relative to
   everyone else who *does* have the current information, and nobody notices until
   the season is underway.
2. **No outside check** — the model has no way to say "I am unusually confident /
   unusually out of step with consensus here," which is exactly the kind of signal
   that should trigger a human look before draft season.

## Technique

Two independent mechanisms — deliberately *not* merged into one blended number yet:

**1. Rank divergence.** Convert our own projection to a rank *within position group*
(QB vs QB, RB vs RB, ...) using `proj_fpts`, `proj_fpts_ppr`, or `proj_fpts_half`
(already computed per player). Ranking overall across all positions was tried first
and discarded — QBs structurally score more raw fantasy points than any other
position, so an overall ranking clusters nearly every startable QB in the top 30-40
regardless of projection quality, while consensus/ADP reflects scarcity-adjusted
*draft value* (a 1-QB league starts one QB but 2-3 RB/WR). That mismatch alone
produced rank deltas of 40-94 for QBs in the first run — a methodology artifact, not
a projection error. Within-position ranking is comparable on both sides: for the
consensus side, `RANK() OVER (PARTITION BY source, position ORDER BY value ASC)`
recovers a position-relative rank from each source's list regardless of whether that
source published an overall or position-specific ranking, since only the relative
order within the partition matters. Then take the **median** rank/ADP across all
external sources matched to that player for the same format:

```
consensus_rank_median = median(source_1_rank, source_2_rank, ..., source_n_rank)
rank_delta = our_rank - consensus_rank_median
```

Median (not mean) because individual outlets disagree by a wide margin on specific
players — e.g. one national outlet had Christian McCaffrey 24th overall in August
2026 while five other sources and three ADP feeds all had him inside the top 10. A
mean would let that one outlier drag the comparison; the median doesn't.

**2. Situational tagging.** Store discrete events (injury, depth-chart battle, scheme
change, holdout, suspension, rookie usage, trade) as rows tagged to a player, each
with a direction (positive/negative/neutral), a magnitude (major/moderate/minor), and
a confidence level (multi-source vs. single-source report). These are surfaced
alongside a player's projection, not folded into it — a human decides what a
"season-ending injury" tag should do to a number, the model doesn't guess.

Some events aren't really about one player — a QB competition or a scheme change is
team-wide context that matters to every pass-catcher on the roster, but *how much*
and in *which direction* differs per player (a QB upgrade doesn't help every receiver
equally; a run-heavy scheme change can help RBs while hurting WRs). Rather than have
the system guess that allocation, notes carry an explicit `scope` (`player` | `team`):
team-scoped notes are surfaced to every player on that team+season without any
inference about impact, same as player-scoped ones — the report just shows more
context, it never decides which way it cuts.

## Assumptions

- **Sources must be format-matched.** Redraft season-long value and dynasty
  trade value diverge enormously for the same player (Derrick Henry: top-11–24
  redraft, ~83rd on KeepTradeCut dynasty, because dynasty prices in age-driven
  decline redraft doesn't care about). Mixing formats in one median is meaningless.
- **Player identity resolution is inherently fuzzy.** Almost no external source
  shares a stable ID with `nfl_players`. `sleeper_id`/`espn_id`/`rotowire_id`
  already exist on that table for the sources that do provide one; everything else
  falls back to normalized name + position + team, with team treated as a soft
  signal (traded players won't match on team).
- **Cross-source agreement is a confidence signal, not a correctness guarantee.**
  Multiple outlets can share the same bias (e.g. all drafting off the same ADP
  data). `source_count` is stored per divergence row precisely so low-agreement
  rows can be down-weighted or excluded from review.
- **Consensus itself can be wrong.** This layer exists to flag disagreement, not to
  assume the outside world is right and our model is wrong — a large divergence is
  a prompt to look, not a correction to apply.

## When it applies in this codebase

- `backend/cmd/projections/consensus.go` — `importConsensusRankings`,
  `importSituationalNotes`, and `computeConsensusDivergences` (new, this feature).
- `nfl_projections` (`backend/migrations/000007_projections.up.sql`) — the source of
  "our rank"; read-only from this layer's perspective.
- `docs/algorithm-review.md` — should reference this entry once the layer has a
  season of retrospective results to discuss.

## Worked example

First real run, 2026 preseason PPR (`base_season=2025`), from the actual pipeline:

**Justin Jefferson (WR): our=32, consensus=6 (delta +26 within the WR pool, n=7
sources).** Investigating the divergence (not just trusting either side) found a
real, non-buggy cause: Jefferson's
2025 `fpts_ppr_pg` fell from 19.0 (2024) to 11.6, even though his own role didn't
shrink — `target_share` (0.307 vs 0.301) and `wopr` (0.741 vs 0.717) both held or
improved. What collapsed was Minnesota's whole passing offense: `team_pass_yds_pg`
fell 27% (257.6 → 188.7), consistent with a rough rookie season from a first-year
starting QB. The comp engine correctly found historical matches for "elite
target-share receiver stuck with bad QB play" (Demaryius Thomas 2011 with Tim Tebow,
T.Y. Hilton 2017 without Andrew Luck) and those comps' next-season growth is mixed
(0.14x–1.7x), producing a muted 2026 projection. Consensus ranks him top-11 because
the market already knows what the comp engine structurally can't: Minnesota's 2026 QB
job is expected to go to a proven veteran, not the struggling rookie. This is exactly
the pattern the divergence layer exists to surface — not a bug, a real blind spot,
now visible instead of silent. The team-scoped situational note for the Vikings QB
competition (see "Situational tagging" above) prints inline with Jefferson's
divergence line for exactly this reason.

**Full investigation, first run (36 outliers with |delta| ≥ 20 before the
within-position fix):** rather than investigate each in isolation, grouping them
found six overlapping root-cause clusters — the QB ranking-methodology artifact
above (7 players), team-level offensive collapse with the cause already researched
(Jefferson) vs. not yet researched (3 Eagles players + Terry McLaurin), a player who
switched teams via free agency with no forward-looking context at all (Kenneth
Walker III — also caught a note-scoping bug in the process), an injury-shortened
base season standing in as a player's entire comp-matching input (Malik Nabers), a
comp pool nearly empty for a statistically rare veteran profile (Derrick Henry,
`comp_count=2`), and rookies/sophomores where consensus is pricing in camp buzz the
model structurally can't see (7 players). Full breakdown and the resulting backlog:
`docs/algorithm-review.md` §6.

**Cross-source noise, for contrast:** Christian McCaffrey's Bleacher Report rank (24)
sat well below five other sources clustered at 3–9 (`source_count=7`). The median
correctly absorbed that single outlier rather than treating it as signal — a
divergence only gets interesting when *most* sources disagree with the model, not
when one does.

## How to validate it's working

No historical backtest is possible — we don't have historical consensus-ranking
snapshots the way we have historical `nfl_player_stats`. Validation plan instead:

1. **Zero mutation risk.** Because this never touches `nfl_projections`, it cannot
   regress the existing backtested accuracy (`nfl_backtest_results`) by construction.
2. **Retrospective check at end of 2026 season.** Compare players with large
   `rank_delta` against actual season-ending fantasy output — did the divergence
   predict which side (our model or consensus) was closer to right? This is the
   real test of whether the layer should ever graduate to influencing projections.
3. **Sanity check on import.** `source_count` should be ≥2 for any divergence taken
   seriously; single-source divergences are noise until corroborated.

## Tradeoffs

- **Doesn't improve the model yet.** This is a diagnostic/review layer; it costs
  engineering effort now for a payoff that only materializes after a season of
  retrospective data and a deliberate decision to blend.
- **Player-matching is lossy.** Name-based matching will misfire on suffixes,
  duplicate names, and mid-season trades; unmatched rows are kept (not dropped) with
  `gsis_id = NULL` specifically so this can be audited rather than silently losing data.
- **Ingestion is manual/file-based, not live.** Several major sources (FantasyPros
  consensus rank, Yahoo, PFF, CBS) render their tables client-side and blocked
  scraping outright during research — there is no reliable automated refresh path
  today, so snapshots go stale between manual imports.

## References

- [consensus-sources.md](consensus-sources.md) — the concrete source catalog (which
  outlets, what they publish, coverage depth) and the exact divergence/auction
  formulas this entry motivates.
- Wisdom-of-crowds / forecast-aggregation literature generally supports **median
  over mean** for aggregating independent-ish rankings when a minority of sources
  may be extreme outliers (median breakdown point 50% vs. mean's 0%).
- Surowiecki, *The Wisdom of Crowds* (2004) — the general case for aggregating
  independent judgments, with the caveat (also true here) that the judgments must be
  reasonably independent, not copied from the same upstream ADP feed.
