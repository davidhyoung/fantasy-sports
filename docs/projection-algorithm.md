# NFL Player Projection Algorithm

## Overview

The projection system uses a comp-based (Marcel-style) approach: for each target player, it finds historical players with similar statistical profiles and physical/career attributes, then uses how those comparable players developed in subsequent seasons to project the target's future performance.

Unlike regression-to-the-mean systems that apply fixed aging curves, this approach lets the data speak — if a player's closest comps all declined sharply at age 29, that trajectory is baked into the projection. Comp count is itself a confidence signal: a player with many strong comps is a recognizable archetype, while one with few comps is genuinely unusual and carries wider uncertainty.

The system runs as an offline batch CLI (`cmd/projections/main.go`) and writes results to the database. There are no per-request computations.

---

## Data Sources

- **nflverse** weekly player stats (1999–present), imported via `cmd/import/`
- `nfl_players` — player metadata: position, physical profile, draft info, career stage
- `nfl_player_stats` — weekly box scores aggregated to seasonal totals during profile building
- Team-level context (pass attempts per game, team rushing share) derived from `nfl_player_stats` itself — no external team data feed required

---

## Player Profiles

Step 1 (`-profiles`) reads `nfl_player_stats` and `nfl_players`, computes one row per player per season in `nfl_player_season_profiles`.

Each profile captures:

| Dimension group | Examples |
|---|---|
| Per-game rates | pass_yards/g, rush_yards/g, targets/g, receptions/g, fg_pct |
| Efficiency metrics | yards_per_attempt, yards_per_carry, yards_per_reception, completion_pct, epa_per_play |
| Usage share | target_share, wopr, rush_share (within team) |
| Physical profile | height, weight, BMI (used as passive similarity dimensions) |
| Career stage | years_exp, age at season start |
| Team context | team_pass_volume (team pass attempts/g), team_rush_volume |
| Draft capital | draft_number (normalized within round), draft_round — only for players with < 3 years experience |
| Volume | games_played, games_started |

All rate dimensions are z-scored against the same position group and stored as JSONB in the `z_scores` column. Raw season totals are also stored for trajectory calculation.

---

## Similarity Matching

Step 2 (`-project`) compares each target player's profile against every historical profile in the same position group using **weighted Euclidean distance** on z-scored dimensions.

```
distance(a, b) = sqrt( Σ w_d × (z_a_d - z_b_d)² )
similarity     = 1 / (1 + distance)
```

A comp is accepted only if `similarity >= 0.60`. There is no fixed-N cutoff — all qualifying comps are used. This means a common archetype may produce 40+ comps while a unique player profile produces 0.

**Draft capital as a similarity dimension** is applied only when the target player has fewer than 3 years of NFL experience. For veterans, draft slot is dropped from the distance calculation entirely — a 5th-year player has proven (or failed to prove) themselves independent of where they were drafted.

**Position group matching** is used rather than strict position matching. For example, WR and slot WR are grouped together; RB profiles are compared only against other RBs. This prevents obviously irrelevant cross-position comparisons while allowing for positional flexibility within a group.

---

## Position-Specific Dimensions

Different positions emphasize different statistical dimensions. Weights below are relative (higher = more influence on distance).

### QB
| Dimension | Weight |
|---|---|
| completion_pct | 2.0 |
| yards_per_attempt | 2.0 |
| pass_yards_per_game | 1.5 |
| passing_tds_per_game | 1.5 |
| interceptions_per_game | 1.5 |
| rush_yards_per_game | 1.0 |
| epa_per_play | 2.0 |
| years_exp | 1.0 |

### RB
| Dimension | Weight |
|---|---|
| rush_yards_per_game | 2.0 |
| yards_per_carry | 2.0 |
| rush_share | 2.0 |
| targets_per_game | 1.5 |
| receptions_per_game | 1.0 |
| receiving_yards_per_game | 1.0 |
| rushing_epa_per_play | 1.5 |
| years_exp | 1.0 |
| age | 1.5 |

### WR
| Dimension | Weight |
|---|---|
| targets_per_game | 2.0 |
| target_share | 2.5 |
| receptions_per_game | 1.5 |
| receiving_yards_per_game | 2.0 |
| yards_per_reception | 1.5 |
| wopr | 2.0 |
| receiving_epa_per_play | 1.5 |
| years_exp | 1.0 |
| age | 1.0 |

### TE
| Dimension | Weight |
|---|---|
| targets_per_game | 2.0 |
| target_share | 2.0 |
| receptions_per_game | 2.0 |
| receiving_yards_per_game | 2.0 |
| yards_per_reception | 1.5 |
| receiving_epa_per_play | 1.5 |
| years_exp | 1.5 |
| age | 1.0 |

### K
| Dimension | Weight |
|---|---|
| fg_pct | 3.0 |
| fg_long | 2.0 |
| fg_made_per_game | 2.0 |
| pat_made_per_game | 1.0 |
| years_exp | 1.0 |

---

## Development Curve Projection

For each accepted comp, the algorithm looks up what that historical player did in the 1, 2, and 3 seasons following the matched season. This is the **development trajectory**.

Each trajectory is weighted by `similarity²` — squaring the similarity sharpens the influence of the closest comps and reduces noise from borderline matches.

The projected value for each stat in year N+1 is:

```
projected_stat = Σ (similarity² × comp_stat_in_year_N+1) / Σ similarity²
```

**Growth caps** are applied to prevent extreme outlier comps from distorting projections:

- Maximum growth: 3× the target's current season value
- Minimum floor: 0.1× the target's current season value (players can decline but not to zero)

**Retired player handling:** if a comp player retired after the matched season (no subsequent stats exist), they contribute a zero trajectory for that year. This naturally penalizes archetypes with high retirement rates — for example, older RBs with declining usage whose comps frequently did not play again.

---

## Confidence Score

Each projection row includes a `confidence` float (0–1). It is a weighted combination of five factors:

| Factor | Weight | Notes |
|---|---|---|
| Comp count | 30% | Normalized: 0 comps → 0, 20+ comps → 1.0 |
| Mean similarity of comps | 25% | Average `similarity` across all accepted comps |
| Data completeness | 20% | Fraction of dimensions that were non-null for the target |
| Seasons of history | 15% | More seasons = more stable profile; saturates at 5 years |
| Games played (current season) | 10% | Full 17-game season → 1.0; scaled proportionally |

```
confidence = 0.30 × comp_count_score
           + 0.25 × mean_similarity
           + 0.20 × completeness
           + 0.15 × history_score
           + 0.10 × games_played_score
```

---

## Profile Uniqueness

Comp count is surfaced as a human-readable `archetype_label` in the projection output:

| Comp count | Label |
|---|---|
| 20+ | common |
| 10–19 | moderate |
| 3–9 | rare |
| 1–2 | very rare |
| 0 | unique |

A "unique" label means no historical player clears the 0.60 similarity threshold. The projection falls back to population mean for the position group with minimum confidence. A "common" label suggests the player fits a well-understood mold and the projection is based on a robust sample.

---

## Limitations

- **Team context changes:** projections assume stable team context (pass volume, offensive scheme). A QB trade or coordinator change can invalidate a projection entirely.
- **Position changes:** a player switching from RB to full-time pass catcher mid-career has no valid comps in either position group until a new profile accumulates. The system will flag low comp count / low confidence in these cases.
- **Kicker volatility:** FG% is highly variable season to season. Kicker projections have structurally lower confidence and wider variance than skill positions.
- **No in-season weekly projections:** the system produces season-level projections only. Week-by-week projections accounting for opponent, weather, injury status, and game script are not yet implemented.
- **Historical coverage:** nflverse data starts at 1999. Players whose archetype only emerged after 2010 (e.g., pass-catching RBs in modern spread offenses) will have fewer valid comps from the early years of the dataset.

---

## Running

All commands run from the `backend/` directory:

```bash
# Step 1: build player season profiles (run after importing nflverse data)
make project-nfl ARGS="-profiles"

# Step 2: compute comp-based projections for a target season
make project-nfl ARGS="-project -season 2025"

# Both steps in sequence
make project-nfl ARGS="-all -season 2025"
```

Steps are idempotent — re-running upserts on conflict.

---

## Database Tables

### `nfl_player_season_profiles`

One row per player per season. Stores aggregated per-game rates, raw season totals, career stage metadata, and a `z_scores` JSONB column containing each dimension's z-score within the player's position group for that season.

Key columns: `gsis_id`, `season`, `position_group`, `games_played`, `age`, `years_exp`, `z_scores` (JSONB), `raw_stats` (JSONB), `team_context` (JSONB), `created_at`.

### `nfl_projections`

One row per player per projected season. Stores the projected stat line, confidence score, comp count, mean similarity, archetype label, and a `comp_details` JSONB array listing each comp's `gsis_id`, `matched_season`, and `similarity`.

Key columns: `gsis_id`, `season`, `position`, `projected_stats` (JSONB), `confidence`, `comp_count`, `mean_similarity`, `archetype_label`, `comp_details` (JSONB), `created_at`.
