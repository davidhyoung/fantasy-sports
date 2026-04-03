# Player Ranking Algorithm

## Overview

The ranking engine scores every rostered player in a league relative to all other rostered players, using the league's own scoring categories. It produces an **overall value score**, **per-category z-scores**, **percentiles**, and **position-relative rankings**.

Rankings are computed on-the-fly from Yahoo Fantasy API data — no local database storage needed.

## How It Works

### 1. Data Collection

The endpoint `GET /api/leagues/{id}/rankings?stat_type=season` fetches two things concurrently:

- **All team rosters with stats** — every player on every team, with their season (or specified period) stats
- **League scoring categories** — the stat categories that determine matchup wins (e.g., PTS, REB, AST for NBA; Pass Yds, Rush TD for NFL). Display-only stats are excluded.

### 2. Z-Score Computation

For each scoring category:

1. **Collect** all rostered players' values for that category
2. **Compute the mean** (average) across all players who have a value
3. **Compute the standard deviation** (spread) across those same players
4. **For each player**, compute the z-score:

```
z = (player_value - mean) / stdev
```

A z-score tells you how many standard deviations a player is above or below the league average:
- `z = +1.0` means the player is 1 standard deviation above average
- `z = -0.5` means the player is half a standard deviation below average
- `z = 0` means the player is exactly average

### 3. Sort Order Handling

Yahoo provides a `sort_order` field for each category:
- `"1"` = **higher is better** (e.g., Points, Rebounds, Touchdowns)
- `"0"` = **lower is better** (e.g., Turnovers, Interceptions)

For "lower is better" categories, the z-score is **flipped**: `z = -z`. This ensures a positive z-score always means "good" regardless of the category direction.

### 4. Overall Score

A player's overall score is the **sum of all their z-scores** across every scoring category:

```
overall_score = z_PTS + z_REB + z_AST + z_STL + z_BLK + z_FG% + z_FT% + z_3PM + z_TO
```

All categories are weighted equally. A higher overall score means the player contributes more value across all categories.

### 5. Percentile

For each category, players are ranked and assigned a percentile:

```
percentile = (number_of_players_beaten / total_players) * 100
```

A player at the 90th percentile in PTS is scoring more points than 90% of rostered players.

### 6. Ranking

- **Overall rank**: Players sorted by overall score descending. #1 is the most valuable.
- **Position rank**: Players grouped by position (QB, RB, PG, C, etc.), then ranked by overall score within each group. This answers "how good is this player relative to others at their position?"

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| Player has no stats for a category | z-score = 0 (neutral) |
| All players have the same value (stdev = 0) | z-score = 0 for everyone |
| Player has no stats at all | Overall score = 0, ranked last |
| League has no scoring categories | Endpoint returns empty response |
| No rosters loaded | Endpoint returns empty response |

## Visual Indicators (Frontend)

Stat cells in roster tables are color-coded by z-score:

| Z-Score Range | Color | Meaning |
|---------------|-------|---------|
| z >= 1.5 | Strong green | Elite (top ~7%) |
| z >= 0.5 | Light green | Above average |
| -0.5 < z < 0.5 | No color | Average |
| z <= -0.5 | Light red | Below average |
| z <= -1.5 | Strong red | Poor (bottom ~7%) |

The "Value" column shows the overall score with color (green = positive, red = negative) and the overall rank as a badge.

## Example

NBA H2H Categories league with 10 teams, ~130 rostered players:

| Player | PTS (z) | REB (z) | AST (z) | TO (z) | Overall | Rank |
|--------|---------|---------|---------|--------|---------|------|
| Jokic | +1.82 | +2.10 | +1.93 | -0.31 | +8.24 | #1 |
| Curry | +2.05 | -0.48 | +0.82 | +0.53 | +5.12 | #5 |
| Bench player | -0.90 | -0.40 | -1.20 | -0.80 | -4.50 | #98 |

Jokic's PTS z-score of +1.82 means he scores 1.82 standard deviations more points than the average rostered player. His TO z-score of -0.31 means he turns the ball over slightly more than average (negative because lower turnovers is better).

## Future Enhancements

- **Custom category weights** — let users weight categories differently (e.g., 2x for steals)
- **Position-relative z-scores** — compare players only against others at their position
- **Points league support** — use Yahoo's `stat_modifiers` for weighted scoring
- **Trend analysis** — compare this week vs last week vs season z-scores
- **Free agent rankings** — rank available players to find top pickups
- **Trade analyzer** — compare net value when swapping sets of players
