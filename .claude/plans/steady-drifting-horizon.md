# Separate Real-Life Player Grades from Fantasy League Value

## Context

The app currently has two systems that blur together:
- **Projections** (`cmd/projections/`) — projects raw NFL stats forward using historical comps. Scoring-agnostic but outputs fantasy point totals as a convenience.
- **Rankings** (`internal/services/ranking/`) — computes league-specific relative value (VORP for NFL, z-scores for NBA) from live Yahoo data. Displayed in a separate Rankings tab in league detail.

The user wants a clear three-layer separation:

1. **Player Grade** — "How good is this player at actual football?" A position-relative composite score (0-100 percentile) derived from efficiency, volume, usage, and context metrics already in our season profiles. Independent of any fantasy league. Displayed at top-level `/rankings` and on player detail pages.

2. **Stat Projections** (existing) — "What stats will this player produce?" The comp-based system stays as-is but gains Player Grade as an additional similarity dimension.

3. **Fantasy League Value** (existing) — "How valuable is this player in MY league?" VORP/z-scores + draft values. The current standalone Rankings tab gets **removed** — its data gets integrated into the Players tab instead (fantasy value column + sortability). Draft tab stays as-is but gains a Grade column with delta indicator.

Build for NFL first, designed so NBA can follow the same pattern.

---

## Phase 1: Player Grade computation (backend)

### Migration `000011_player_grades.up.sql`

```sql
CREATE TABLE nfl_player_grades (
    gsis_id         TEXT NOT NULL REFERENCES nfl_players(gsis_id),
    season          INT NOT NULL,
    position_group  TEXT NOT NULL,

    overall         NUMERIC(5,2) NOT NULL,   -- 0-100 percentile
    production      NUMERIC(5,2) NOT NULL,   -- raw statistical output
    efficiency      NUMERIC(5,2) NOT NULL,   -- EPA, comp%, YAC, etc.
    usage           NUMERIC(5,2) NOT NULL,   -- target share, snap proxy, volume
    durability      NUMERIC(5,2) NOT NULL,   -- games played / expected

    career_phase    TEXT NOT NULL,            -- developing/prime/post-prime/late-career
    yoy_trend       NUMERIC(5,3),            -- overall change vs prior season

    dimension_details JSONB NOT NULL DEFAULT '{}',
    computed_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (gsis_id, season)
);

CREATE INDEX idx_pg_ranking ON nfl_player_grades (season, position_group, overall DESC);
```

### New file: `backend/cmd/projections/grades.go`

Computes grades for all seasons with profile data (2020-2025). Reads from `nfl_player_season_profiles`, writes to `nfl_player_grades`.

**Position-specific sub-score weights and dimensions:**

| Position | Production | Efficiency | Usage | Durability |
|----------|-----------|------------|-------|------------|
| QB | 30% | 35% | 20% | 15% |
| RB | 25% | 25% | 25% | 25% |
| WR | 25% | 30% | 30% | 15% |
| TE | 30% | 30% | 25% | 15% |
| K | 40% | 40% | 5% | 15% |

**Per-position sub-score dimensions:**

| Sub-score | QB dims | RB dims | WR dims | TE dims |
|-----------|---------|---------|---------|---------|
| Production | pass_yds_pg, pass_td_pg, rush_yds_pg | rush_yds_pg, rush_td_pg, rec_yds_pg, rec_td_pg | rec_yds_pg, rec_td_pg, rush_yds_pg | rec_yds_pg, rec_td_pg |
| Efficiency | pass_epa_play, comp_pct, pass_ypa, int_pg (inv), sacks_pg (inv) | rush_epa_play, rush_ypc, rec_epa_play, receiving_yac_pg | rec_epa_play, rec_ypr, receiving_yac_pg, receiving_air_yards_pg | rec_epa_play, rec_ypr, receiving_yac_pg |
| Usage | pass_yds_pg (volume proxy) | rush_yard_share, target_share, targets_pg | target_share, wopr, targets_pg | target_share, targets_pg |
| Durability | games / 17 | games / 17 | games / 17 | games / 17 |

**Rationale for position-specific weights:**
- **RB**: Higher durability weight (25%) — RB careers are short and injury-prone, availability is a major differentiator
- **WR**: Higher usage weight (30%) — target share and WOPR are the strongest predictors of WR value
- **QB**: Higher efficiency weight (35%) — EPA/play and completion % separate elite QBs from volume passers
- **K**: No meaningful usage metric, so production and efficiency dominate

**Algorithm:**
1. Load all profiles for a given season + position group
2. For each sub-score: average the z-scores of its dimensions (already computed in `z_scores` JSONB)
3. Convert sub-score z-scores to percentiles (empirical rank within the cohort)
4. `overall = Σ(position_weight[sub] × sub_percentile)` using position-specific weights above
5. Convert overall to percentile
6. `yoy_trend = this_season_overall - prev_season_overall` (normalized to -1..+1 scale)
7. `career_phase` from existing `aging.Phase()` function

**Invocation:** `make project-nfl ARGS="-grades"` (new flag alongside existing `-profiles`, `-project`)

**Files to modify:**
- `backend/cmd/projections/main.go` — add `-grades` flag, wire to `computeGrades()`
- `backend/Makefile` — already has `project-nfl` target, no change needed

### Projected grades for target season

After computing historical grades, also compute a "projected grade" for the target season using the projected stats from `nfl_projections`. Run the same formula on the projected per-game rates to produce a forward-looking grade. Store with `season = target_season`.

---

## Phase 2: API endpoints

### New file: `backend/internal/handlers/grades.go`

Two endpoints:

**`GET /api/grades?season=2025&position=QB&limit=200&offset=0`**
- Returns ranked list of players with grades
- Response: `{ season, players: [{ gsis_id, name, position, team, headshot_url, age, overall, production, efficiency, usage, durability, career_phase, yoy_trend, overall_rank, position_rank }], total }`

**`GET /api/grades/{gsisId}`**
- Returns all seasons of grades for a player
- Response: `{ gsis_id, name, position, team, headshot_url, seasons: [{ season, overall, production, efficiency, usage, durability, career_phase }] }`

**Files to modify:**
- `backend/cmd/api/main.go` (or wherever routes are registered) — add routes
- `frontend/src/api/client.ts` — add types + API functions
- `frontend/src/api/queryKeys.ts` — add cache keys

### Extend existing endpoints

- `GET /api/projections` and `GET /api/projections/{gsisId}` — JOIN to `nfl_player_grades` to include `player_grade` and `grade_rank` in response
- `GET /api/nfl/players/{gsisId}` — include grade history in response
- `GET /api/leagues/{id}/draft-values` — include `player_grade` in each player row
- `GET /api/leagues/{id}/rankings` — include `player_grade` and `yoy_trend` for each player (for PlayersTab integration)

**Files to modify:**
- `backend/internal/handlers/projections.go` — add grade fields to list/detail response structs, JOIN query
- `backend/internal/handlers/nfl_players.go` — add grades to player detail response
- `backend/internal/handlers/draft_values.go` — add grade field to draft player response
- `backend/internal/handlers/analysis.go` — add grade fields to ranked player response

---

## Phase 3: Frontend — Player Grade on existing pages

### Player detail page (`/players/:gsisId`)
- Add a "Player Grade" card between the header and stats table
- Show overall grade prominently (large number with color: 90+ elite green, 70-89 good blue, 50-69 average, <50 below average)
- Show sub-score bars (production, efficiency, usage, durability)
- Show YoY trend sparkline from grade history
- **File:** `frontend/src/pages/player-detail/index.tsx` + new `components/GradeCard.tsx`

### Projections table (`/projections`)
- Add "Grade" column (sortable) showing overall grade with color
- **File:** `frontend/src/pages/projections/components/ProjectionTable.tsx`

### Draft tab (league detail)
- Add "Grade" column + delta indicator alongside existing VOR/auction value
- Delta indicator: when fantasy rank differs significantly from grade rank, show badge (e.g. grade rank is much better than fantasy rank = "undervalued", or vice versa)
- **File:** `frontend/src/pages/league-detail/DraftTab.tsx`

### Players tab — absorb Rankings tab data
- The current Rankings tab (`RankingsTab.tsx`) gets **removed** as a standalone tab
- Its fantasy value data (VORP, z-scores, position rank) gets integrated into the Players tab columns
- Players tab gains: Grade column (sortable), Trend indicator (up/down arrow + magnitude), Fantasy Value column (already there as VORP)
- Users can sort by Grade or by Fantasy Value to see different perspectives
- **Files to modify:**
  - `frontend/src/pages/league-detail/PlayersTab.tsx` — add Grade + Trend columns
  - `frontend/src/pages/league-detail/hooks/usePlayers.ts` — ensure grade data flows through
  - `frontend/src/pages/league-detail/index.tsx` — remove Rankings tab trigger + content
  - `frontend/src/pages/league-detail/RankingsTab.tsx` — delete (or keep for reference during migration)

---

## Phase 4: Frontend — Standalone `/rankings` page

New top-level page at `/rankings` in the nav (alongside Projections). This shows **real-life Player Grades**, not fantasy value.

- Position filter tabs (All, QB, RB, WR, TE, K)
- Season selector
- Table: Rank, Player, Team, Pos, Overall Grade, Production, Efficiency, Usage, Durability, Phase, Trend
- All columns sortable
- Rows clickable → `/players/:gsisId`
- Grade cells color-coded by percentile tier

**New files:**
- `frontend/src/pages/rankings/index.tsx`
- `frontend/src/pages/rankings/hooks/useRankings.ts`
- Route added in `frontend/src/App.tsx`

---

## Phase 5: Grade as projection input

### Similarity dimension
Add `overall_grade_z` to the z-scores JSONB in season profiles. Include it as a new dimension group in comp matching with moderate weight (1.0-1.5). This makes the comp system prefer players who were similarly "good at football overall," not just stat-line twins.

**Files to modify:**
- `backend/cmd/projections/main.go` — add grade z-score to profile building, add dimension group to `positionGroups()`

### Trend adjustment
Small bounded adjustment to projected stats based on grade trajectory:
- Player trending up in grade but stats haven't caught up → slight upward nudge
- Player trending down → slight downward nudge
- Capped at +/- 5%

---

## Key decisions

- **Percentile scale (0-100)** not raw z-scores for user-facing values. Z-scores remain in `dimension_details` JSONB.
- **Position-specific sub-score weights** from day 1 (RB: higher durability, WR: higher usage, QB: higher efficiency).
- **Rankings tab removed** — fantasy value data absorbed into Players tab. `/rankings` top-level page is for real-life grades.
- **Grade + delta indicator** on Draft tab — shows grade rank vs fantasy rank discrepancy.
- **Batch computed** like projections, not on-the-fly. Run via CLI flag.
- **Separate table** from season profiles (input vs derived output, avoids circular dependency).
- **NFL-first** but table naming (`nfl_player_grades`) and handler structure make it straightforward to add `nba_player_grades` later with sport-specific dimension definitions.

## Verification

1. Run `make project-nfl ARGS="-grades"` and verify `nfl_player_grades` has data for 2020-2025
2. Spot-check: top-graded QBs should be Mahomes, Allen, etc. Top RBs should be Henry, McCaffrey, etc.
3. Hit `GET /api/grades?season=2024&position=QB` and verify response
4. Check player detail page shows grade card
5. Check `/rankings` page renders correctly with sortable columns and grade colors
6. Check projections table has grade column
7. Check Draft tab shows grade + delta indicator
8. Check Players tab shows grade + trend + fantasy value (no separate Rankings tab)
9. After Phase 5: re-run projections, verify comp matching uses grade dimension
