# NBA Auction Values and Consensus Pricing

> **Status: implemented (October 2026)** in `internal/handlers/nba_draft_values.go`
> (`GetNBADraftValues`/`attachNBAConsensusValues`). The consensus side is a *derived*
> value today, same caveat as [auction-values.md](auction-values.md)'s NFL version.

## Problem it solves

Same two problems [auction-values.md](auction-values.md) describes for NFL — turning a
projection into a dollar price, and comparing that price to what the external market
implies — but native NBA leagues had **no auction economics at all** until this: no
"points" analog to price off of, and no way to say "what should I pay for X" in a
12-team $200 category league.

## Why this isn't a straight port of the NFL formula

NFL prices off fantasy *points*, a single number every league agrees on once the
pointing system is fixed. NBA category leagues don't have that — value is a **z-score
sum across up to 9 roto categories**, and *which categories count is a per-league
setting* (`league_settings.scoring` is a `{category: 0|1}` inclusion map — see
`CLAUDE.md`'s "Native NBA leagues" section). A league that turns off `turnovers` or
`ft_pct` genuinely values players differently, so the dollar board has to be recomputed
per-league, not read off a single universal number the way NFL's PPR/half/standard
totals are. This is why `GetNBADraftValues` can't just read the stored
`nba_projections.our_rank` (always all 9 categories, equally weighted) — it recomputes
the z-score sum itself, restricted to the categories `league_settings.scoring` actually
turns on.

Roster positions are the other structural difference: NFL's roster slots are a rich,
configurable vocabulary (`QB/RB/WR/TE/FLEX/SFLEX/K/DEF`) parsed through Yahoo-style
flex-eligibility strings (`ranking.ParseFlexEligible`). NBA's are a fixed 3-bucket
vocabulary — `G/F/C` plus a position-blind `UTIL` — ESPN's own taxonomy, the sole data
source (verified against real roster data: `nba_players.position` only ever holds
`G`, `F`, `C`, nothing finer). Forcing that through NFL's flex parser (which maps
`Q`/`W`/`R`/`T` abbreviations, not `G`/`F`/`C`) would have meant touching shared NFL
ranking code for an unrelated sport, so NBA's replacement-level math
(`nbaComputeReplacementLevels`) is a small bespoke function instead — same
dedicated-slots-claim-directly, pooled-slot-claims-by-value algorithm, just written
for exactly three positions.

## Technique

### Our value: league-category z-score sum → VOR → compressed, budget-reserved share

```
included(cat)      = league_settings.scoring[cat] != 0
z(p, cat)           = (stat(p, cat) − poolMean[cat]) / poolStdev[cat]   // turnovers negated
fantasyValue(p)      = Σ z(p, cat) over cat where included(cat)
```

`fantasyValue` is NBA's "points" analog — everything downstream is the same math
[auction-values.md](auction-values.md) documents for NFL, with `fantasyValue` standing
in for `points`:

```
replacement[pos]  = fantasyValue of the player at index (dedicated_slots[pos] + util_claims[pos]) × teams
                    in that position, sorted by fantasyValue descending
VOR(p)            = max(0, fantasyValue(p) − replacement[pos(p)])
rosterSpots        = teams × Σ(G, F, C, UTIL, BN, IR slot counts)
surplus            = teams × budget − rosterSpots
compressedVOR(p)   = VOR(p) ^ 0.75
auction(p)         = max(1, round( 1 + compressedVOR(p) / ΣcompressedVOR × surplus ))
```

**UTIL pooling.** Dedicated `G`/`F`/`C` slots claim their top `count × teams` players
directly. `UTIL` slots then pool whichever `G`/`F`/`C` candidates are still unclaimed,
greedily claiming whichever position's next-best player has the higher `fantasyValue`
— the same "let the actual best players claim shared spots" idea NFL's flex pooling
uses (`ranking.ComputeReplacementLevels`), just fixed to exactly three positions
instead of parsed from a roster-slot string. Verified on a real 12-team/$200/9-category
league (`UTIL: 2`): the two UTIL slots per team (24 league-wide) split 16 to `G`, 4 to
`F`, 4 to `C` based on that league's actual player values — a different league's talent
distribution would split differently, which is the point.

**No age multiplier.** NFL's auction value applies an age-based draft multiplier
(`internal/aging`, NFL-specific prime-years curves). There's no NBA equivalent in this
codebase, so NBA auction values skip this step entirely rather than force-fit an
NFL-shaped aging curve onto a different sport.

**Compression exponent.** Reuses NFL's empirically-tuned `0.75` as a starting point —
unvalidated against real NBA auction-room behavior (no equivalent "observed ceiling"
research has been done for NBA). The computed top price in a real local league (12
teams, $200, all 9 categories) landed at $101 for the consensus #1 overall, which is in
the range real NBA auction drafts actually pay for a top player in that shape of
league — a reasonable first anchor, but the constant itself has no NBA-specific
derivation yet.

### Consensus value: our curve, the market's rank — joined through our own position

```
pos_rank(p, source) = RANK() OVER (PARTITION BY source, nba_players.position ORDER BY value ASC)
consensus_rank(p)    = median over sources of pos_rank(p, source)
consensus_auction(p) = auction(our player whose position_rank == round(consensus_rank(p)))
```

This looks like NFL's `loadConsensusValues`, but with one deliberate difference: the
`RANK()` partitions on **`nba_players.position` joined in at query time**, not the raw
`position` text column stored on `nba_consensus_rankings` itself. NFL's version
partitions on the stored column directly because NFL's position vocabulary
(QB/RB/WR/TE) is standard across every outlet that publishes rankings. NBA outlets
publish `PG/SG/SF/PF/C` — a 5-way split — while this app's own vocabulary is the
ESPN-taxonomy 3-bucket `G/F/C` scheme. Partitioning on the raw text would fragment
ranks into incompatible buckets (and in practice the stored column is empty on every
imported row today — none of the five source JSON snapshots populated it). Joining to
our own canonical position fixes both problems at once.

Imported real $ values (`metric_type = 'auction'`) take precedence when present, same
generic path and `12-team/$200` reference-pool rescale NFL's `marketBudgetPool`
convention uses (`nbaMarketBudgetPool`) — nothing publishes these yet (no real external
NBA $-auction data has been imported, matching NFL's own current state), but the schema
and import path (`nba_consensus_rankings.metric_type` is plain `TEXT`) already accept
them without a migration.

## Worked example

A real local native NBA league (EuroLeagueAllStars, 12 teams, $200, all 9 categories
on), read live from `GET /api/leagues/{id}/draft-values-nba?season=2026`:

| Player | Pos | Our rank | Our $ | Cons $ | Edge |
|---|---|---|---|---|---|
| Victor Wembanyama | F | 1 | $101 | $101 | $0 |
| Nikola Jokic | C | 2 | $84 | $84 | $0 |
| Shai Gilgeous-Alexander | G | 3 | $68 | $68 | $0 |
| Luka Doncic | G | 4 | $67 | $67 | $0 |
| Tyrese Maxey | G | 5 | $66 | $49 | **+$17** |
| Kawhi Leonard | F | 6 | $63 | $35 | **+$28** |

The top four land exactly on the market's own number — our board and the external
consensus agree both on who's elite and on how much elite costs here. Maxey and
Leonard are where the two disagree: we'd pay $17–28 more than the market's ranking
implies, a bargain if our model is right about their category profile, a mistake if
the market's caution (age/injury-history priced in for Leonard, a step below superstar
for Maxey) is the one to trust.

**Category sensitivity, same league, `?scoring=points:1,rebounds:1,assists:1,` (all
else `0`):** the #1 overall player changes from Wembanyama to Jokic — his elite
points/rebounds/assists profile outranks Wembanyama's blocks-heavy one once steals,
blocks, turnovers, threes, and both shooting percentages stop counting. This is the
concrete proof the league-category requirement actually works, not just the inclusion
flags being read and ignored.

## Assumptions

- Same "the market's ranking and ours measure the same thing" assumption
  [auction-values.md](auction-values.md) states for NFL — both this league's category
  set and the market's (whatever it is) are assumed roto/category-shaped, not points.
- A position rank maps to a price, same caveat as NFL — reading our curve at the
  market's *derived* position rank assumes we and the market disagree about *who* is
  the best center, not about what the best center is worth.
- **The derived position rank is doubly indirect for NBA**: it comes from an *overall*
  consensus number (NBA sources publish overall category-league rankings, not
  position-split ones — see [nba-consensus-rankings.md](nba-consensus-rankings.md)),
  turned into a position-relative ordinal only by sorting players of the same position
  by that overall number. This is a reasonable adaptation, not the real thing an outlet
  that actually published position ranks would give.
- Consensus coverage is partial (the 592-row source snapshot covers a few hundred
  players, not the full pool) — uncovered players show `null`/`—`, never `$0`.
- Auction value is a valuation, not a bid — same caveat as NFL.

## When it applies in this codebase

- `backend/internal/handlers/nba_draft_values.go` — `GetNBADraftValues` computes our
  value and attaches consensus; `attachNBAConsensusValues` implements both the
  imported and derived consensus paths; `nbaComputeReplacementLevels` is the bespoke
  G/F/C/UTIL replacement-level function.
- `backend/cmd/project-nba/consensus.go` — `importConsensusRankings` is where
  `metric_type = 'auction'` rows would arrive (schema-ready, not populated).
- `frontend/src/pages/league-detail/components/NBAAuctionValues.tsx` — the Auction $
  view in `NativeDraftPrepTabNBA.tsx`.

## How to validate it's working

- **Identity check:** with no consensus rows for a season, every consensus column is
  `null` and `auction_value` is unaffected.
- **Budget-pool check:** summing `auction_value` over the top `Σslots × teams` players
  (the ones who'd actually fill a roster spot) lands within a few dollars of
  `teams × budget` — $2,394 vs. a $2,400 pool in the worked-example league above.
  Summing the *whole* evaluated pool (hundreds of $1-floor players who'd never
  actually be drafted) overshoots, same documented NFL caveat.
- **Category-sensitivity check:** changing `?scoring=` measurably reorders the board
  (see the worked example above) — if it didn't, the league-category requirement
  wouldn't actually be wired in.

## Tradeoffs

Same list [auction-values.md](auction-values.md) gives for NFL applies here —
not a market-clearing price, `edge` only as good as the (doubly-derived, for NBA)
rank matching, single-source coverage uncorroborated, the 0.75 compression exponent
unvalidated for this sport specifically. Additionally:

- **No age multiplier** — NFL's draft-value age curve doesn't exist for NBA in this
  codebase; a 38-year-old and a 23-year-old with the same projected stat line price
  identically here, which a real drafter wouldn't do.
- **The derived consensus position rank is an extra step removed from reality** for
  NBA specifically (see Assumptions) — NFL's version reads a rank the source actually
  published at that position; NBA's is inferred from an overall number.

## References

- [auction-values.md](auction-values.md) — the NFL original this adapts; most of the
  underlying VOR/compression/consensus reasoning is shared and not repeated here.
- [nba-consensus-rankings.md](nba-consensus-rankings.md) — the external source catalog
  and why NBA rankings come overall, not position-split.
- [nba-projection-algorithm.md](nba-projection-algorithm.md) — where `nba_projections`'
  per-category values (the input to this doc's z-score sum) come from.


## Projection source: DARKO (October 2026)

Auction values no longer price off the homegrown comp-based `nba_projections`; they use
[DARKO](https://darko.app) per-player forward projections (`nba_darko_projections`).
`fantasyValue` and everything downstream (replacement levels, VOR, compression, tiers,
consensus) is unchanged — only the per-game stat line feeding the z-scores differs.

- **Per-game conversion:** `possessions = pace × minutes / 48`; each per-100 rate × possessions / 100.
  3PM = `fg3a_100 × fg3_pct`; rebounds = `orb_100 + drb_100`.
- **Percentage categories:** valued as makes above a league-average shooter on the same
  attempts (`fgm − leagueFG% × fga`), not the raw percentage — a 60% shooter on 2 attempts
  must not outrank a 52% shooter on 15. The league rate is pooled over the draftable pool.
- **Z-score population:** the draftable pool (top `teams × (G+F+C+UTIL+BN)` by projected
  minutes), not every projected player, so deep-bench bodies don't inflate each stdev.
- **Playing-time guards:** DARKO's snapshot can carry end-of-season rest/tanking minutes
  (a deep-bench player projected at 35 mpg). `x_minutes` is averaged with last season's actual
  mpg when the player has ≥20 games, capped at 34 (28 without a sample); pace is clamped to
  [92, 98]. These are judgment calls — not backtested.
- **Known gaps:** no games-played/injury projection (rates only); the snapshot predates the
  2026-27 season (dataset as of 2026-06-03) so offseason moves aren't reflected; rookies and
  call-ups have no blend partner.
- **Refresh:** re-pull the snapshot via the DARKO tools, write the `{columns, rows}` JSON,
  `make project-nba ARGS="-import-darko f.json -season 2026"`.
