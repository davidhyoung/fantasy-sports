# Strength of Schedule (Opponent Adjustment)

## Problem it solves

A WR with 1,200 receiving yards against the league's 3 worst pass defenses is not equivalent to one with 1,200 yards against top-10 defenses. Neither the ranking engine (`analysis.go`) nor the projection engine (`cmd/projections/`) adjusts for opponent quality today. This systematically overrates players on easy schedules and underrates those on hard ones — and worse, it **compounds** in comp-based projections because the historical comps' own stats were also unadjusted.

## Technique

Two common forms, from simplest to most rigorous:

### 1. Schedule-adjusted rate (simple)

For each player-game, divide raw production by the opponent defense's per-game yield to the relevant position:

```
adjusted_game = raw_game × (league_avg_allowed / opp_allowed_to_position)
season_adj    = Σ adjusted_games
```

Where `opp_allowed_to_position` is e.g. "yards allowed to WRs per game" for that defense that season. Simple, cheap, and sufficient for ranking adjustments.

### 2. Iterative / SRS-style (rigorous)

Borrow from the Simple Rating System used in college sports rankings. Let `r_i` be player `i`'s rating and `d_j` be defense `j`'s rating. For each game:

```
raw_i_vs_j = r_i − d_j + ε
```

Solve iteratively: each player's rating is their average game output minus the average opponent defense rating, and each defense's rating is symmetric. Converges in a few iterations. The output is both a schedule-adjusted player rating **and** a defense rating — the defense ratings can then feed back into the next season's projections as a prior on opponent strength.

## Assumptions

- Defensive performance against a position is measurable and reasonably stable within a season. (Mostly true; injuries to key defenders violate it.)
- Opponent schedules overlap enough to be comparable. (True in the NFL — 17 games with common opponents.)
- The adjustment is linear/multiplicative. (A useful simplification; reality is closer to multiplicative with caps.)

## When it applies in this codebase

- `cmd/projections/profiles.go` — when aggregating weekly `nfl_player_stats` into season profiles, apply a schedule adjustment per-game before summing. Store both raw and adjusted versions in `nfl_player_season_profiles.raw_stats` (add an `adjusted_stats` sibling).
- `cmd/projections/main.go` — similarity matching should use adjusted z-scores. Two players with the same raw target share but facing very different schedules are less similar than they look.
- `internal/handlers/analysis.go` — rankings could optionally expose a "schedule-adjusted" toggle, especially useful mid-season when schedules have diverged.
- `internal/handlers/grades.go` — real-life grades should absolutely be SOS-adjusted; a "good" RB is one who performs well against average opposition, not one who feasted on bad defenses.
- **New data dependency**: requires opponent-per-game context. We already have every game in `nfl_player_stats`; we need to aggregate it into `nfl_team_defense_stats` by season + position faced. This is a derivable table, no external feed needed.

## Worked example

Two RBs, 2024 season:

| RB | Raw rush YPG | Avg opp defense rank vs RB | Adj rush YPG |
|---|---|---|---|
| RB A | 92 | 26 (weak) | 78 |
| RB B | 85 | 8 (strong) | 97 |

RB A's raw number looked better, but he got there against soft defenses. After adjustment, RB B is the more valuable rusher — and his comp matches in the projection engine should now be drawn from historically strong rushers, not average ones.

## How to validate it's working

- **Year-over-year stability**: schedule-adjusted stats should be more stable YoY than raw stats (because schedule noise is removed). Compute YoY correlation for raw vs adjusted on historical seasons; adjusted should correlate higher with itself across years.
- **Backtest delta in `nfl_backtest_results`**: add an `sos_adjusted` variant; compare RMSE against unadjusted baseline.
- **Face-validity check**: after adjustment, elite-schedule outliers (think: old Matt Forte seasons, or any RB in a historically bad division) should move in the expected direction.

## Tradeoffs

- **Added data pipeline**: we need to build and maintain a defense-vs-position table. It's derivable from data we have, but it's an extra offline step.
- **Adjustment goes wrong at edges**: a defense that plays no one but elite WRs looks "strong" only because of who they faced — iterative SRS mitigates this, simple adjustment does not.
- **Introduces a circularity**: defense ratings are themselves computed from (partly) the same players we're rating. SRS resolves this with iteration; naïve approaches can double-count.
- **Complicates comp matching**: once stats are adjusted, the historical comps' own stats must be adjusted on the same basis. Consistency across eras matters more than the specific adjustment formula.

## References

- Simple Rating System, *sports-reference.com* glossary — explains the iterative college-football/basketball rating approach.
- Football Outsiders / DVOA methodology papers — the canonical NFL opponent-adjusted efficiency framework.
- Massey, *Statistical Models Applied to the Rating of Sports Teams* (1997) — academic treatment of SRS-style systems.
