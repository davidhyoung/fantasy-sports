# Tiering (1-D Clustering of Player Value)

> **Status: implemented (August 2026)** in `internal/services/tiers/` and attached
> per player by `GetDraftValues`. Shown on the draft board and the prep shortlist.

## Problem it solves

A ranked list implies a precision that projections don't have. "RB6" and "RB7"
are different rows, but if they're projected within two points of each other the
distinction is noise — and acting on it costs you. The question a drafter
actually asks is *"if I wait a round, do I still get someone like this?"*, which
a rank cannot answer and a tier can.

The naive approaches both fail:

- **Fixed-size groups** (tiers of five) draw breaks where there is no gap and hide
  breaks where there is one. A cliff between RB3 and RB4 gets buried mid-tier.
- **A fixed point threshold** ("every 20 points is a tier") doesn't transfer
  between positions or scoring formats: 20 points separates the top three QBs and
  a third of the WR pool.

## Technique

Sort a position's players by value descending and walk the list once, keeping a
running tier. A player joins the current tier while the tier's **total spread**
stays inside a width budget, and starts a new tier when it wouldn't:

```
budget = (value[0] − value[draftable−1]) / targetTiers

tier ← 1;  tierTop ← value[0]
for each player p in descending value:
    if tierTop − p.value > budget:
        tier ← tier + 1
        tierTop ← p.value
    assign tier to p
```

Symbols:

- `value` — league-scored projected points (`proj_league_fpts`), so tiers follow
  the same numbers the auction prices do.
- `draftable` — how many at this position get drafted; `ceil(starterSlots ×
  teams × 1.5)`. The ×1.5 covers the bench/injury fringe.
- `targetTiers` — how many tiers should span the draftable range. **Default 8.**

Two properties fall out of the single rule, and they're the reason it's the rule:

1. **The tier's promise is literal.** Everyone inside a tier is within one budget
   of everyone else in it. That is what "interchangeable" has to mean for the
   drafter's question to have an answer.
2. **Cliffs break tiers automatically.** A cliff is precisely a gap that blows the
   budget, so no separate gap rule is needed. A separate rule would also
   misbehave: gaps are large at the top of a position and near zero in the
   middle, so any single gap threshold either makes the elite all singletons or
   the middle one undifferentiated mass.

The budget is derived from the draftable range rather than being an absolute
number of points, so it scales with position, scoring format and league size on
its own.

## Assumptions

- **Value is one-dimensional and already comparable.** Tiering happens *within*
  position; across positions the raw point totals aren't comparable (QBs score
  more), which is the same trap documented in `consensus-ensemble.md`.
- **The draftable range carries the signal.** Including the replacement-level
  tail — hundreds of players within a few points of zero — inflates the range and
  collapses the top of the board into one or two tiers. `TestDraftableRangeSetsTheWidth`
  pins this down.
- **Projections are accurate enough for the gaps to be real.** Given the ~12%
  level bias found in `docs/algorithm-review.md` §7.7 is *proportional*, it
  scales the budget along with the values and leaves tier membership unchanged.
- **Equal values belong together.** Ties never split.

## When it applies in this codebase

- `backend/internal/services/tiers/tiers.go` — `Assign`, the whole algorithm.
- `backend/internal/handlers/draft_values.go` — step 9 tiers each position group
  and sets `draftPlayer.Tier`.
- `frontend/src/pages/draft-prep/components/DraftBoardTable.tsx` — the Tier
  column (sortable; sorting by tier deliberately interleaves positions, which
  answers "who's at the top of their own position?").
- `frontend/src/pages/draft-prep/components/Shortlist.tsx` — tier beside position
  on each rated player.

## Worked example

The 2026 board, 12-team PPR, top of each position:

| Pos | Tier | n | span | players |
|---|---|---|---|---|
| RB | 1 | 2 | 2.8 | Robinson (340), Gibbs (337) |
| RB | 2 | 3 | 16.1 | Achane (310), McCaffrey (301), Taylor (294) |
| RB | 3 | 5 | 25.2 | Skattebo (279) … Hampton (254) |
| WR | 1 | 1 | 0.0 | Nacua (350) |
| WR | 2 | 1 | 0.0 | Chase (316) |
| WR | 3 | 4 | 13.9 | Smith-Njigba (291) … Rice (277) |
| QB | 1 | 1 | 0.0 | Allen (350) |
| QB | 2 | 2 | 6.8 | Maye (322), Dart (315) |

Read the WR column: Nacua and Chase are each alone because the drop to the next
player (34 and 25 points) exceeds the budget — they are genuinely not
interchangeable with anyone. Then four receivers sit inside 14 points, so if you
miss London you can take Lamb and lose almost nothing. That is the decision the
tier exists to support, and a rank column cannot express it.

## How to validate it's working

- **Unit tests** (`tiers_test.go`): cliffs split, the spread promise holds for
  every tier, the draftable window controls the width, ties stay together, tier
  numbers follow value order.
- **Eyeball the elites.** If the top of a position is one big tier, the budget is
  too wide (check `draftable`); if the flat middle is all singletons, it's too
  narrow.
- **Tier counts.** Roughly `targetTiers` tiers should cover the draftable range,
  with more beyond it in the tail. Wildly more or fewer means the range is being
  set by the wrong pool.

## Tradeoffs

- **`targetTiers` is a judgment, not a discovery.** Eight is chosen because it
  puts a 12-team starter pool into groups of two or three — actionable without
  being arbitrary. It is not derived from the data, and nothing here estimates
  "the right number of tiers".
- **Breaks can land mid-plateau.** On a long smooth run with no cliff, the budget
  still forces a break somewhere, and that boundary is arbitrary — two players a
  point apart can sit either side of it. A gap-seeking pass (Jenks / 1-D k-means)
  would place those boundaries better at the cost of a much less explainable
  algorithm and a per-position `k` to choose. Worth revisiting only if the
  arbitrary boundaries prove misleading in use.
- **No uncertainty.** Two players can share a tier while having very different
  projection confidence or outcome spread (`uncertainty-quantification.md`). The
  tier says their point estimates are close, not that they're equally safe.

## References

- Jenks, George F. — "The Data Model Concept in Statistical Mapping" (1967), the
  natural-breaks classification this deliberately simplifies.
- Fishman, Mike — tier-based drafting, the standard framing of "draft the tier,
  not the player".
- See also: [auction-values.md](auction-values.md) for the value these tiers group.
