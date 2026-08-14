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

### Our value: VOR → compressed, budget-reserved proportional share

```
replacement[pos]  = points of the player at index ceil(starterSlots[pos] × teams)
                    in that position, sorted by points descending
VOR(p)            = max(0, points(p) − replacement[pos(p)]) × ageMultiplier(p)
rosterSpots        = teams × Σ(all roster slot counts, including bench)
surplus            = teams × budget − rosterSpots         // $1 reserved per spot up front
compressedVOR(p)   = VOR(p) ^ 0.75                          // see "Why compress" below
auction(p)         = max(1, round( 1 + compressedVOR(p) / ΣcompressedVOR × surplus ))
```

`starterSlots` comes from the league's roster with flex spots split evenly across
their eligible positions (`ranking.ComputeStarterSlots`). The `max(0, …)` floor means
sub-replacement players contribute nothing to `ΣVOR`.

**Fixed 2026-08-14: the $1 reservation.** Every rostered player costs at least a
dollar, VOR-positive or not — standard VBD practice reserves that dollar for *every*
roster spot (bench included) before splitting what's left by VOR share. The original
implementation skipped the reservation: it split the *full* nominal budget across only
the VOR-positive players, then floored everyone else at $1 on top, so the league's
true spendable total (`teams × budget`) was smaller than what the formula divided among
the players who actually earned a share — the sum of all auction values didn't just
"slightly exceed" the pool, the whole board ran hot, worst at the top. Restricting to
the players who'd actually get drafted (down to the last roster spot), the sum now
lands within $1 of `teams × budget`, as it should.

**Why compress.** Even with the reservation fixed, a shallow single-QB roster (Yahoo's
common default: RB2/WR2/FLEX1, no superflex) leaves very few flex-eligible starter
slots, so few players clear replacement level — and linear VOR-proportional sharing
concentrates the *entire* pool onto that small group. That pushed the consensus #1
overall past $100 in a 12-team/$200 league, well above what real bidders actually pay
for one player (observed ceiling: roughly $70-80) — real drafters don't execute pure
linear VOR math; budget-diversification instincts cap how much of $200 anyone puts on
one player even when the math says they "should" pay more. Raising VOR to the 0.75
power before sharing compresses that spread (the top gives up relatively more than the
middle) while leaving rankings and the zero/non-zero VOR boundary untouched, since
`x^p` is monotonic for `x > 0`. The exponent was picked empirically against this app's
own board to land the top price in the observed real-world range — it is a single
global constant, not tuned per league.

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

A real synced league's board (12 teams, $200, PPR, Yahoo default roster), post the
2026-08-14 $1-reservation and compression fixes:

| Player | Pos | Our rank | Cons rank | Our $ | Cons $ | Edge |
|---|---|---|---|---|---|---|
| Kyle Pitts | TE | 13 | 5.0 | $1 | $34 | **−$33** |
| Justin Jefferson | WR | 16 | 6.0 | $26 | $46 | **−$20** |
| Malik Nabers | WR | 5 | 13.5 | $55 | $29 | **+$26** |
| Cam Skattebo | RB | 17 | 18.0 | $23 | $21 | **+$2** |

Read the Jefferson row: sources call him the WR6; our board pays $46 for its own WR6;
our board ranks him WR16 and pays $26. We are $20 cheaper than the market on a player
the market considers elite. The dollar figure is what makes it actionable: it's a $20
mistake if we're wrong, 10% of a $200 budget. Skattebo (once the largest gap in the
dataset — RB6 on a $66 valuation against a consensus RB18 read) is now within a
rounding error of the market after the short-season shrinkage fix (`docs/algorithm-review.md`
§6, CLAUDE.md's "Short-season shrinkage" entry) — the auction-value gap tracking the
rank gap down is itself a sign the two fixes are consistent with each other.

## How to validate it's working

- **Identity check:** with no consensus rows for a season, every consensus column is
  `—` and the board is unchanged. With rows present, coverage should be ~60–100
  players (the published depth), not the whole pool.
- **Scale check:** doubling the budget roughly-but-not-exactly doubles the derived
  column post-compression (verified: Puka Nacua $77 → $160, a 2.08× not 2.00× ratio —
  the `1 + share × surplus` shape isn't scale-invariant the way plain proportional
  share was). The imported-price path is untouched and still scales exactly (`price(p)
  = median × teams×budget / (12×200)` is a plain linear rescale).
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
- **The compression exponent (0.75) is a single global constant, not tuned per
  league.** It was picked to land the top price in the observed real-world range for
  a standard 12-team/$200/single-QB board; a very different league shape (huge budget,
  deep superflex, 30-team dynasty) has no guarantee it lands in the "right" place, and
  there's no principled derivation for 0.75 beyond that empirical match — a different
  real-market anchor could argue for a different exponent.
- **Auction values now sum to within ~$1 of the true budget pool** (`teams × budget`),
  restricted to the players who'd actually fill a roster spot — the $1-per-slot
  reservation fixed 2026-08-14 makes that identity hold rather than merely
  approximately hold. Summing the *whole* evaluated player pool (including everyone
  past the last roster spot, each shown at the $1 floor) will still overshoot, since
  those aren't real draft picks.

## References

- Zola, Joe — "Value Over Replacement and Auction Values", FantasyPros methodology
  writeups (2015–present), the standard formulation of proportional VOR pricing.
- Harstad, Doug — "Value Based Drafting", Footballguys (1999), the original VBD
  framing that replacement-level pricing descends from.
- See also: [consensus-ensemble.md](consensus-ensemble.md) for source handling,
  median choice, and within-position ranking; [consensus-sources.md](consensus-sources.md)
  for the concrete source catalog and formulas.
