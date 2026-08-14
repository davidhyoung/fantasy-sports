# Auction Values and Consensus Pricing

> **Status: implemented (August 2026)** in `internal/handlers/draft_values.go` (our own
> value) and `internal/handlers/draft_consensus.go` (the consensus column). The
> consensus side is a *derived* value today — see the honesty note under Tradeoffs.

## Problem it solves

Two distinct problems, easy to conflate:

**1. Turning projections into prices.** A projection says a player scores 280 points.
An auction draft asks "what fraction of $200 is he worth?" Points don't answer that:
in a 12-team league starting 1 QB, the QB1 and QB13 might be 60 points apart while
the RB1 and RB13 are 130 apart, and the money has to follow the *scarcity*, not the
raw total.

**2. Comparing our price to the market's.** Our board can be internally consistent and
still be badly wrong about a player the market has repriced for reasons the comp
engine can't see (a June trade, a camp injury). We already flag *rank* disagreement
(`consensus-ensemble.md`), but a rank gap doesn't tell you what it costs you: being
three spots off at RB4 is a $12 mistake, being three spots off at RB40 is a $1 one.
Dollars are the unit a drafter actually spends.

## Technique

### Our value: VOR → proportional budget share

```
replacement[pos] = points of the player at index ceil(starterSlots[pos] × teams)
                   in that position, sorted by points descending
VOR(p)           = max(0, points(p) − replacement[pos(p)]) × ageMultiplier(p)
auction(p)       = max(1, round( VOR(p) / ΣVOR × (teams × budget) ))
```

`starterSlots` comes from the league's roster with flex spots split evenly across
their eligible positions (`ranking.ComputeStarterSlots`). The `max(0, …)` floor means
sub-replacement players contribute nothing to `ΣVOR`, and the `max(1, …)` floor means
every rostered player costs at least a dollar. Those two floors are why the sum of
all auction values slightly exceeds the true money pool — the $1 minimums are real
dollars the VOR share never allocated.

### Consensus value: our curve, the market's ranking

External sources publish ranks and ADP, not dollars. Rather than invent a second
dollar model, we read our own price curve at the position rank the market assigns:

```
consensus_rank(p)    = median over sources of RANK() within (source, position)
consensus_auction(p) = auction( our player whose position_rank == round(consensus_rank(p)) )
```

So if seven outlets call a player the RB5 and our board pays $44 for *its* RB5, his
consensus value is $44. The comparison this enables is then exactly one thing:

```
edge(p) = auction(p) − consensus_auction(p)
```

A positive edge means we'd pay more than the market's ranking implies — a bargain
*if* our ranking is the right one. Because both numbers come from the same curve,
`edge` isolates the ranking disagreement and carries no artifact of a second pricing
model. It also inherits the league's settings for free: change the budget, the teams,
or the roster, and both columns move together.

Median (not mean) across sources, and rank *within position* rather than overall,
both for the reasons established in [consensus-ensemble.md](consensus-ensemble.md):
one outlet's outlier shouldn't drag the comparison, and overall ranks conflate raw
point volume (which favours QBs) with scarcity-adjusted draft value.

### Imported market prices, when available

If `nfl_consensus_rankings` carries `metric_type = 'auction'` rows for the season and
format, those win over the derived value. They're published against a standard
12-team/$200 pool, so they're rescaled:

```
price(p) = median(imported prices) × (teams × budget) / (12 × 200)
```

This is a linear rescale, which assumes the *shape* of the market's value curve
doesn't change with budget — true enough between $200 and $300 leagues, increasingly
wrong at extremes (a $50 budget compresses everything toward the $1 floor).

## Assumptions

- **The market's ranking and ours are measuring the same thing.** Both must be
  redraft, same scoring format. Dynasty value prices age-driven decline that redraft
  ignores, so mixing them is meaningless (`consensus-ensemble.md`).
- **A position rank maps to a price.** Reading our curve at the market's rank assumes
  the *k*-th best player at a position is worth what our *k*-th best is worth — i.e.
  that we and the market disagree about *who* is RB5, not about what RB5 is worth.
  Where the market's whole positional shape differs from ours (they think this year's
  RB class is flat, we think it's top-heavy), the derived value inherits our shape.
- **Consensus coverage is partial and top-heavy.** Sources publish roughly the top
  100 picks. Everyone outside that is uncovered, not cheap — the UI shows `—`, never
  `$0`, and uncovered players sort last in either direction.
- **Auction value is not draft strategy.** These are valuations, not bids. Actual
  bidding depends on roster construction, money already spent, and what the room does.

## When it applies in this codebase

- `backend/internal/handlers/draft_values.go` — `GetDraftValues` computes our value
  (steps 5–8) and attaches the consensus columns (step 9).
- `backend/internal/handlers/draft_consensus.go` — `loadConsensusValues` implements
  both the imported and derived paths.
- `backend/cmd/projections/consensus.go` — `importConsensusRankings` is where
  `metric_type = 'auction'` rows would arrive; the importer already accepts them
  since `metric_type` is generic.
- `frontend/src/pages/draft-prep/components/DraftBoardTable.tsx` — the `Cons $` and
  `Edge` columns, shown on `/draft-prep` only.

## Worked example

The 2026 mock league board (12 teams, $200, PPR), taking the four largest gaps:

| Player | Pos | Our rank | Cons rank | Our $ | Cons $ | Edge |
|---|---|---|---|---|---|---|
| Cam Skattebo | RB | 6 | 18.0 | $66 | $26 | **+$40** |
| Justin Jefferson | WR | 17 | 6.0 | $17 | $52 | **−$35** |
| Malik Nabers | WR | 5 | 14.0 | $57 | $23 | **+$34** |
| Kyle Pitts | TE | 14 | 5.0 | $4 | $37 | **−$33** |

Read the Jefferson row: seven sources call him the WR6; our board pays $52 for its own
WR6; our board ranks him WR17 and pays $17. We are $35 cheaper than the market on a
player the market considers elite — the exact case `docs/algorithm-review.md` §6 flags
as an injury-shortened base season feeding a thin comp pool. The dollar figure is what
makes it actionable: it's a $35 mistake if we're wrong, which is 17% of a $200 budget.

## How to validate it's working

- **Identity check:** with no consensus rows for a season, every consensus column is
  `—` and the board is unchanged. With rows present, coverage should be ~60–100
  players (the published depth), not the whole pool.
- **Scale check:** doubling the budget doubles both columns (verified: Jefferson
  $52 → $104 derived, $63 → $126 imported).
- **Precedence check:** inserting `metric_type = 'auction'` rows flips
  `consensus_derived` to false and the value to the rescaled median.
- **Retrospective (not yet run):** after a season, correlate `edge` at draft time with
  end-of-season points per dollar. If large positive edges systematically underperform,
  our ranking — not the market's — is the one to fix.

## Tradeoffs

- **The derived consensus value is not a market clearing price.** It is "what our own
  model would pay for the market's ranking". It cannot capture cases where the market
  and our model agree on *rank* but disagree on *how steep* the curve is — if the room
  bids RB1 to $70 and we say $60, a rank-derived consensus will never show that gap.
  Only imported auction prices fix this, which is why that path exists and takes
  precedence.
- **`edge` is only as good as the rank matching.** Player identity resolution across
  sources is fuzzy (`consensus-ensemble.md`), and a mismatched player produces a
  confident, wrong dollar gap.
- **Single-source coverage is uncorroborated** and marked with `*` in the UI, matching
  the divergence table's convention.
- **The $1 floor and the sub-replacement clamp mean auction values don't sum to the
  budget.** This is inherent to proportional VOR allocation, not a bug, but it means
  the column can't be read as "spend exactly this".

## References

- Zola, Joe — "Value Over Replacement and Auction Values", FantasyPros methodology
  writeups (2015–present), the standard formulation of proportional VOR pricing.
- Harstad, Doug — "Value Based Drafting", Footballguys (1999), the original VBD
  framing that replacement-level pricing descends from.
- See also: [consensus-ensemble.md](consensus-ensemble.md) for source handling,
  median choice, and within-position ranking; [consensus-sources.md](consensus-sources.md)
  for the concrete source catalog and formulas.
