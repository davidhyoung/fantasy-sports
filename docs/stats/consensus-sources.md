# Consensus Sources & Calculations — Reference Catalog

> **Status: reference doc (August 2026).** This is the single place that names *which*
> external sources feed our consensus numbers and *exactly* how the two consensus
> calculations work. The reasoning for why we build it this way — median vs. mean,
> within-position ranking, format-matching — lives in
> [consensus-ensemble.md](consensus-ensemble.md) (the divergence layer) and
> [auction-values.md](auction-values.md) (the dollar layer). This doc doesn't repeat
> that reasoning; it's the lookup table + the formulas, kept current as sources are
> added or dropped.

## What "consensus" means here

Every external ranking site disagrees with every other one, and none of them is "the
market" by itself. We don't pick a favorite source — we pull rank/ADP/value from
several, resolve each row to a player (`gsis_id`), and take the **median** within
**position group** (`docs/stats/consensus-ensemble.md`). That single number feeds two
different consumers:

1. **Divergence** (`nfl_projection_divergences`) — our projection rank vs. the
   consensus rank, surfaced on `/divergences` and Home's Player Outlooks.
2. **Draft dollars** (`draft_consensus.go`) — the consensus rank read against *our own*
   auction-value curve, surfaced as `Cons $` / `Edge` on `/draft-prep`.

Both read the same underlying table (`nfl_consensus_rankings`), so adding a source
here improves both automatically — no separate wiring per consumer.

## Source catalog (as of 2026-08-04 snapshot)

All rows currently live in
`backend/cmd/projections/testdata/consensus/rankings_2026-08-04.json`, imported via
`make project-nfl ARGS="-import-consensus f.json -season 2026"`. Ingestion is
manual/file-based — see "Why hand-curated" below.

| `source` key | Outlet | Publishes | `metric_type` | `format` | Rows in snapshot | Notes |
|---|---|---|---|---|---|---|
| `espn` | ESPN Fantasy | Expert consensus rank | `rank` | `ppr` | 63 | Overall + positional expert rankings |
| `si_fabiano` | Sports Illustrated (Michael Fabiano) | Expert rank | `rank` | `ppr` | 63 | Named-columnist rankings, not SI's aggregate |
| `si_onsi` | Sports Illustrated / On3 (OnSI network) | Expert rank | `rank` | `ppr` | 59 | Distinct SI vertical from `si_fabiano` — kept separate since they're independently written |
| `bleacher_report` | Bleacher Report | Expert rank | `rank` | `ppr` | 59 | |
| `fantasypros_adp` | FantasyPros | Aggregated ADP | `adp` | `ppr` | 61 | FantasyPros' *own* ADP aggregate (they already average many draft sites); we treat their output as one source, not a shortcut to many |
| `sleeper_adp` | Sleeper | Live draft ADP | `adp` | `ppr` | 60 | Pulled from actual app drafts, not an expert opinion |
| `espn_adp` | ESPN | Live draft ADP | `adp` | `ppr` | 60 | |
| `underdog_adp` | Underdog Fantasy | Live draft ADP | `adp` | `half_ppr` | 60 | Underdog's default format is half-PPR, not PPR — the only non-PPR source we have |
| `keeptradecut` | KeepTradeCut | Crowdsourced dynasty trade value | `dynasty_value` | `dynasty` | 30 | Not diffed against our projections (dynasty has no comparable model output — see below); kept for future dynasty features |

**Coverage is intentionally shallow.** Every source publishes roughly their top ~60
per list (top-100 overall, thinning fast by position: QB 8 deep, TE 6, RB/WR ~24
each — measured in `docs/algorithm-review.md` §7.1). "Not in the list" means the
source didn't rank that far, not that they'd rank the player last.

**8 of 9 sources are PPR-only; `underdog_adp` is the sole half-PPR data point; there
is no `standard`-format source at all.** A `standard`-format divergence or consensus
$ column today will always come back empty (`—`), regardless of the league's actual
format.

**Dynasty is imported but not diffed.** `projectionRankColumn()`
(`cmd/projections/consensus.go`) explicitly rejects `dynasty`/`superflex` for the
divergence computation — our engine produces a one-season redraft projection with no
multi-year or QB-premium output to compare a dynasty/superflex rank against. The
`keeptradecut` rows exist for a future dynasty feature, not today's divergence table.

## Calculation 1 — Rank divergence

`computeConsensusDivergences` (`cmd/projections/consensus.go`), run via
`-consensus-diff -season N -format ppr|half_ppr|standard`:

```
our_rank(p)        = 1-based rank of p's proj_fpts[_ppr|_half] within p's position group
source_rank(p, s)  = RANK() OVER (PARTITION BY source, position ORDER BY value ASC)
                      -- recovers a position-relative rank from source s's list,
                      -- whether s published an overall or positional ranking
consensus_rank(p)  = median( source_rank(p, s) for every source s that ranked p )
rank_delta(p)      = our_rank(p) − consensus_rank(p)
source_count(p)    = count of sources that ranked p
```

Positive `rank_delta` = we rank the player worse than the market; negative = better.
Written to `nfl_projection_divergences`, one row per `(gsis_id, season, format)`,
re-upserted (not appended) each run.

## Calculation 2 — Consensus auction value

`loadConsensusValues` (`internal/handlers/draft_consensus.go`), called from
`GetDraftValues` for every `/draft-prep` and Draft-tab request:

```
# Preferred: an imported real market price, if one exists for this season+format
imported_price(p) = median(value for rows where metric_type='auction')
                       × (teams × budget) / (12 × 200)     -- rescale from the
                                                             -- standard market pool
                                                             -- these prices assume

# Fallback: derive a price from the consensus rank, read against OUR OWN value curve
consensus_rank(p) = median( RANK() OVER (source, position ORDER BY value ASC) )
                      -- same formula as Calculation 1, computed independently
                      -- here (this handler doesn't read nfl_projection_divergences)
cons_auction(p)    = our_auction_curve[ position(p) ][ round(consensus_rank(p)) ]
                      -- i.e. "what does OUR board pay for the player at the rank
                      -- slot the market assigns him"

edge(p) = auction(p) − cons_auction(p)
```

No source currently publishes `metric_type='auction'` rows, so every consensus dollar
value in the live app today is the derived (fallback) form — `Derived: true` on every
`consensusValue`. The imported-price branch is live code, just unfed.

`source_count(p) == 1` sets the single-source flag (`*` in the UI) on both the
divergence table and the draft board — a lone source is treated as uncorroborated,
never as a confident signal.

## Player identity resolution

`loadPlayerIndex` / `resolve()` (`cmd/projections/consensus.go`), tried in order,
first match wins:

1. `sleeper_id` exact match
2. `espn_id` exact match
3. normalized `name + team` (`normalizePlayerName`: lowercase, strip punctuation and
   suffixes `Jr./Sr./II/III/IV/V`, collapse whitespace)
4. normalized `name + position` (catches players traded since the source's data was
   captured, so team no longer matches)

Unmatched rows are kept with `gsis_id = NULL`, not dropped, specifically so they can
be audited (`SELECT * FROM nfl_consensus_rankings WHERE gsis_id IS NULL`) and the
resolver improved later rather than silently losing data.

## Why hand-curated, not scraped

Several major ranking sites render their tables client-side or actively block
scraping (checked directly, not assumed): FantasyPros' consensus rank page, Yahoo,
PFF, and CBS all failed a straightforward fetch during research for this feature.
There is no reliable automated refresh path today — snapshots go stale between manual
`-import-consensus` runs, and the `captured_date` on every row exists precisely so a
stale snapshot is visible rather than silently trusted.

## Candidate sources for future inclusion

Researched 2026-08-14 looking specifically for sources that would close the known
gaps above (a `standard`-format source, a `superflex` source, a second dynasty/expert
list, deeper positional coverage). Not yet imported — nothing below has a row in
`nfl_consensus_rankings` today.

<!-- CANDIDATE_SOURCES_TABLE -->

## When it applies in this codebase

- `backend/cmd/projections/testdata/consensus/*.json` — the current hand-curated
  snapshots; new sources are added as new rows here, then imported.
- `backend/cmd/projections/consensus.go` — `importConsensusRankings` (accepts any
  `source`/`format`/`metric_type` string — no source allowlist to update when adding
  one) and `computeConsensusDivergences`.
- `backend/internal/handlers/draft_consensus.go` — `loadConsensusValues`.
- `docs/stats/consensus-ensemble.md`, `docs/stats/auction-values.md` — the design
  rationale this catalog implements.

## References

- Source list and row counts: direct inspection of
  `rankings_2026-08-04.json` (515 rows, 9 sources) and
  `situational_notes_2026-08-04.json` (27 rows, 17 distinct outlet strings) in this
  repo, 2026-08-14.
- Coverage-depth figures: `docs/algorithm-review.md` §7.1 (62-player 2026 PPR
  coverage check).
