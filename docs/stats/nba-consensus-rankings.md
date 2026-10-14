# NBA Consensus Rankings

> **Status: implemented (October 2026)** in `backend/cmd/project-nba/consensus.go` +
> `backend/internal/handlers/nba_rankings.go`. Unlike the NFL equivalent
> ([consensus-ensemble.md](consensus-ensemble.md)), this is not diagnostic-only — it's a
> live, user-facing comparison surfaced on the native NBA league's Draft Prep tab
> (Consensus view), and it shows **every individual source's rank**, not just the
> aggregated median.

## Problem it solves

The native NBA comp-based projection engine ([nba-projection-algorithm.md](../nba-projection-algorithm.md))
only ever sees real box-score history — it has no way to know a player signed
elsewhere in free agency, is coming back from a major injury, or that the market
(ESPN, CBS, RotoWire, etc.) has already priced in information the model structurally
can't see. Two things follow:

1. **No outside check.** Without a comparison, there's no way to say "the model is
   unusually confident / unusually out of step with the market here" — exactly the
   signal that should prompt a human look before a draft.
2. **A single "consensus" number hides disagreement.** Averaging five outlets into one
   median is useful, but a viewer comparing our rank to "the market" can't tell whether
   that market is five outlets in lockstep or four that disagree sharply with a fifth.

## Technique

Two numbers, computed from the same underlying `nba_consensus_rankings` table,
surfaced side by side on `GET /api/nba/rankings`:

```
our_rank(p)        = overall z-score fantasy-value rank of p's batch-computed
                      projection (nba_projections.our_rank — see
                      nba-projection-algorithm.md's "Ranking the projections" section)
source_rank(p, s)   = RANK() OVER (PARTITION BY source ORDER BY value ASC)
                      -- recovers a comparable rank from source s's list regardless
                      -- of how many players it actually covers
consensus_rank(p)   = median( source_rank(p, s) for every source s that ranked p )
```

No within-position split (unlike NFL): category-league rankings are published overall
(ESPN, CBS, RotoWire, NBA.com all publish one combined top-N list), and NBA's z-score
fantasy-value metric is already standardized across positions — there's no raw-volume
skew the way NFL's points structurally favor QBs.

**Every individual `source_rank(p, s)` is returned**, not just the median — the API
response's `sources` array is `[{"source": "espn", "rank": 12}, {"source": "cbs", "rank":
15}, ...]`. This is the one deliberate divergence from the NFL pattern: NFL's
`/divergences` only ever exposes `consensus_rank_median` + `source_count`, discarding
the per-source ranks after computing the median (see
[consensus-ensemble.md](consensus-ensemble.md)'s "When it applies" section). Here, the
per-source ranks are computed live on every request (cheap at ~300-600 rows) rather than
persisted in a denormalized table, so adding or dropping a source needs no schema change.

Median (not mean) for the same reason NFL uses it: a single outlier source shouldn't
drag the comparison — see consensus-ensemble.md's wisdom-of-crowds reasoning, which
applies identically here.

## Assumptions

- **Single format, no position split.** Simpler than NFL: this app has exactly one NBA
  scoring format (9-category roto), so there's no `format` dimension in
  `nba_consensus_rankings` at all, and no within-position ranking to reconcile.
- **Player identity resolution is simpler than NFL's.** `nba_players.espn_id` is the
  row's own primary key (not a crosswalk column), so the resolver
  (`cmd/project-nba/consensus.go`'s `resolve()`) only needs: an explicit `espn_id` on the
  input row (rare), then normalized `name+team`, then normalized `name+position`. No
  `sleeper_id` tier exists because nothing upstream of `nba_players` provides one.
- **Team abbreviations need normalizing.** Different outlets spell team abbreviations
  differently (`GSW` vs. `GS`, `NYK` vs. `NY`, `WAS` vs. `WSH`, `UTA` vs. `UTAH`, `SAS`
  vs. `SA`, `NOP` vs. `NO`, `BRK` vs. `BKN`, `PHO` vs. `PHX`) from the ESPN-derived
  convention `nba_players.team` uses. `teamAliases` in `consensus.go` maps them before
  resolution — the same purpose as NFL's own `teamAliases`, a different alias set.
- **Consensus itself can be wrong.** Same caveat as NFL: this is a comparison to look
  at, not a correction to apply — nothing here feeds back into `nba_projections`.
- **Rookies have the same cold-start gap as NFL's divergence layer found** — a rookie's
  consensus coverage depends on how late a source's snapshot was captured relative to
  training camp/preseason buzz; not specifically investigated for NBA yet.

## When it applies in this codebase

- `backend/cmd/project-nba/consensus.go` — `importConsensusRankings`,
  `computeConsensusDivergences`, `resolve()`/`normalizePlayerName`/`teamAliases`.
- `backend/migrations/000038_nba_consensus.up.sql` — `nba_consensus_rankings`,
  `nba_projections`, `nba_projection_divergences`.
- `backend/internal/handlers/nba_rankings.go` — `ListNBARankings`
  (`GET /api/nba/rankings`), the live per-source-rank + median computation.
- `frontend/src/pages/league-detail/components/NBAConsensusRankings.tsx` — the
  Draft Prep "Consensus" view (dynamic per-source columns, driven entirely by whichever
  sources the API returns).
- `backend/cmd/project-nba/testdata/consensus/*.json` — hand-curated snapshots, same
  manual/file-based ingestion philosophy as NFL's (see "Why hand-curated" in
  [consensus-sources.md](consensus-sources.md) — the same unscrapeable-JS-table and
  paywall problems apply to NBA sites).

## Source catalog (first snapshot, captured 2026-10-14)

| `source` key | Outlet | Publishes | `metric_type` | Rows | Notes |
|---|---|---|---|---|---|
| `espn` | ESPN Fantasy | Roto/category-league top 150 | `rank` | 150 | Full top-150 fetched cleanly |
| `rotowire` | RotoWire | Category-league top 150 | `rank` | 150 | Full top-150 fetched cleanly |
| `nba_com` | NBA.com | Category-league top 250 | `rank` | 250 | Deepest single source; full top-250 fetched cleanly |
| `hashtag_basketball` | HashtagBasketball | Projections + ADP | `rank` | 30 | Free tier gates past the top 30 |
| `cbs` | CBS Sports (SportsLine model) | ADP | `adp` | 12 | Article names only the top 12; full board is paywalled behind SportsLine |

Same "coverage is intentionally shallow" caveat NFL's catalog makes: a player absent
from a source's list means the source didn't rank them that far, not that they'd be
ranked last. `source_count` on every ranking item tells a viewer how many outlets
actually weighed in — a single-source divergence is noise until corroborated, same
threshold NFL's consensus-ensemble.md sets.

**Yahoo was deliberately not pursued as a source** — its public consensus-rankings
article is JS-rendered, the same dead end NFL's research hit (see
consensus-sources.md's "Why hand-curated, not scraped"). A real Yahoo signal would mean
building a Yahoo Fantasy API `draft_analysis` integration (average pick/percent-drafted
for a synced league) — scoped out of this pass.

## Worked example

From the first real run, 2026 preseason (`cmd/project-nba -consensus-diff -season
2026`): **Damian Lillard, our=504 (last place) vs. consensus=70, delta=+434** — the
single largest gap in the dataset. Investigating (not just trusting either side): our
model has no way to know Lillard tore his Achilles in the 2025 playoffs and is expected
to miss most or all of the 2026-27 season — it saw his last healthy per-game rate (an
excellent one) and, with no injury signal to discount it, grew that rate forward nearly
unchanged. The market has priced in the injury; the model structurally can't see it —
the same shape of blind spot NFL's consensus-ensemble.md documents for Justin
Jefferson's QB-competition case, just NBA's version of "the model only sees box
scores."

**Cross-source agreement, for contrast:** Victor Wembanyama's five sources were 2, 2, 2,
2, 2 (ESPN, CBS, HashtagBasketball, NBA.com, RotoWire) — unanimous, and our own rank (1)
sits right alongside. A tight cluster like this is the opposite end of the signal from
Lillard's case: nothing to investigate, high confidence all around.

## How to validate it's working

- **Zero mutation risk**, same as NFL: `-consensus-diff` only ever writes to
  `nba_projection_divergences`, never to `nba_projections`.
- **Sanity check on import**: `source_count` should be ≥2 for a divergence taken
  seriously — single-source gaps are noise until corroborated (same threshold NFL's
  consensus-ensemble.md sets).
- **Retrospective check at end of 2026-27 season**: compare players with the largest
  `rank_delta` (Lillard, and others surfaced by `printDivergenceReport`) against their
  actual 2026-27 output — did the gap predict which side was closer to right?

## Tradeoffs

- **Ingestion is manual/file-based**, same as NFL — no reliable automated refresh path;
  `captured_date` on every row exists so a stale snapshot is visible, not silently
  trusted.
- **Shallower than NFL's 13-source catalog.** Five sources is enough for a genuine
  median, but thinner than NFL's; HashtagBasketball/CBS's free-tier gates mean many
  mid-to-late-round players only have 2-3 sources, not 5.
- **Per-source ranks computed live, not persisted** — a deliberate simplicity choice
  given the row counts involved (a few hundred players × up to 5 sources), revisit if
  the source catalog grows enough to make the live `RANK() OVER` query a real cost.

## References

- [consensus-ensemble.md](consensus-ensemble.md) — the NFL divergence layer this ports
  the player-identity-resolution and median-of-ranks technique from; the "median over
  mean" reasoning applies identically here.
- [consensus-sources.md](consensus-sources.md) — NFL's source catalog, including the
  "why hand-curated, not scraped" research (the same JS-rendering/paywall obstacles
  apply to the NBA outlets evaluated for this catalog).
