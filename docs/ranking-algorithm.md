# Player Ranking Algorithm

> Last synced with code: June 2026. Implementation lives in
> `internal/services/ranking/` (pure logic) and `internal/handlers/analysis.go`
> (Yahoo/local data assembly). Update this doc when either changes.

## Overview

The ranking engine scores every rostered player in a league relative to all other
rostered players, using the league's own scoring configuration. It has two modes,
selected by sport:

- **Categories mode** (NBA): weighted z-score rankings (`RankByCategories`)
- **Points mode** (NFL): VORP — value over replacement player (`RankByPoints`)

Rankings are computed on the fly per request. For NFL season rankings, stat values
come from the local `nfl_player_stats` table (via `services/nflstats`); Yahoo supplies
only league context (rosters, scoring categories, roster slots, FA list). Other stat
periods and NBA leagues still read stats from Yahoo.

---

## Categories mode (NBA)

### 1. Per-category z-scores

For each scoring category, mean and stdev are computed across all rostered players,
then each player gets `z = (value − mean) / stdev`. Yahoo's `sort_order` is respected:
for "lower is better" categories (TO), the z-score is flipped so positive always
means good.

### 2. Category weights — NOT equal

Each category's weight is `CV × scarcity`, normalised so the mean weight is 1.0:

- **CV** (coefficient of variation) = `stdev / |mean|` — categories where players
  differ more are worth more.
- **Scarcity** = `1 / (1 + max(0, avg_FA_z))` — computed from the top ~100 free
  agents' z-scores against the rostered baseline. If strong replacements are freely
  available in a category, rostered production in it is discounted.

### 3. Overall and position scores

```
overall_score  = Σ weight_c × z_c            (across scoring categories)
position_score = z-score of overall_score within the player's position group
```

Players are ranked overall and within position. Percentiles per category are
rank-based among rostered players.

### Edge cases

| Scenario | Behavior |
|----------|----------|
| Player has no value for a category | z = 0 (neutral) |
| stdev = 0 for a category | z = 0 for everyone, weight = 0 |
| FA fetch fails | scarcity = 1.0 (logged, rankings still served) |
| < 2 players at a position | position_score = 0, rank 1 |

## Points mode (NFL)

### 1. Total points

`total_points = Σ stat_value × league_stat_modifier` per player. For season stat type,
values come from `nfl_player_stats` translated through the canonical stat-ID vocabulary
(`services/scoring`).

### 2. Replacement levels

Starter demand per position is derived from the league's actual roster settings
(`ranking.ComputeReplacementLevels`). Dedicated (single-position) slots claim
starters directly: `count × num_teams`. Flex-type slots (`W/R/T`, `Q/W/R/T`)
are **not** split evenly across eligible positions — that only holds up when
the eligible positions have comparable value (true for RB/WR/TE in an ordinary
FLEX, false for SFLEX, where QB dwarfs the other three per game in points
formats). Instead each flex slot pools its still-unclaimed eligible candidates
by value and lets the actual best players claim the spots, position-blind —
so a superflex slot resolves to whichever position the numbers say, typically
almost entirely QB, without a hand-tuned weight.

```
threshold   = starters already claimed at this position (dedicated + won from flex pools)
replacement = total_points of the (threshold+1)-th best rostered player
VORP        = player_total_points − replacement
```

Players (rostered + top FAs) are ranked by VORP; per-stat z-scores against the
rostered baseline are attached for cell coloring.

---

## Visual indicators (frontend)

Stat cells are color-coded by z-score (±0.5 light, ±1.5 strong). The Value column
shows overall score/VORP with rank badges; points-mode leagues also display the
replacement-level legend.

## Known gaps

- **No shrinkage on short stat periods** — `stat_type=lastweek` or early-season
  rankings z-score tiny samples directly (see `docs/stats/bayesian-shrinkage.md`;
  the projection pipeline shrinks, the live ranking path does not yet).
- **No schedule adjustment** — live rankings use raw stats, unlike the projection
  profiles which are SOS-adjusted.
- **NBA leagues depend on Yahoo period stats** — no local NBA stats feed yet.

## Future enhancements

- Custom category weights (user-tunable, replacing CV × scarcity)
- Trend analysis (week-over-week z-score changes)
- Trade analyzer (net value across player sets)
