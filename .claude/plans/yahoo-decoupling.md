# Decouple Rankings & Projections from Yahoo

## Context

Yahoo is currently doing two jobs in this codebase:

1. **Fantasy context** — league scoring categories, roster slots, who owns whom, FA status. Yahoo is the only source for this and that's fine.
2. **Stats source** — `GetLeagueRankings` and related flows fetch *raw NFL stat values* from Yahoo for every rostered player and every FA, even though we already have every stat in `nfl_player_stats` (nflverse). This is where the heavy coupling lives.

Goal: Yahoo provides *fantasy context only*. All player stats, projections, and grades come from our own DB. As a side effect, we get a public non-fantasy rankings experience that works without login or league.

NBA is out of scope for this initiative — we don't have a non-Yahoo NBA stats feed yet. Keep the NBA code path on Yahoo. Design the NFL refactor so NBA can follow later when we add a stats feed.

## Current coupling map

Yahoo-free today:
- `GET /api/projections` → `nfl_projections`
- `GET /api/nfl/players/{gsisId}` → `nfl_players` + `nfl_player_stats` + `nfl_projections`
- `GET /api/grades` → `nfl_player_grades`

Yahoo-coupled today:
- `GET /api/leagues/{id}/rankings` — pulls stat values via `GetLeagueRostersWithStats` + `GetAvailablePlayersWithStats`
- `GET /api/leagues/{id}/draft-values` — pulls roster positions + scoring modifiers (keep, this is fantasy context)
- Team/matchup/scoreboard — inherently live Yahoo data, not in scope

The ID bridge `nfl_players.yahoo_id` already exists and `ResolveAllYahooToGsis` / `ResolveBatchYahooToGsis` are in place.

---

## Phase 1 — Local NFL stats provider

Create `internal/services/nflstats/` exposing season-level stat lookups the ranking layer can consume in place of Yahoo roster stats:

```go
type PlayerSeasonStats struct {
    GsisID      string
    Season      int
    StatValues  map[string]float64 // keyed by our canonical stat IDs
    GamesPlayed int
}

func LoadSeasonStats(ctx, db, season int, gsisIDs []string) ([]PlayerSeasonStats, error)
func LoadSeasonStatsAllActive(ctx, db, season int) ([]PlayerSeasonStats, error)
```

Aggregates `nfl_player_stats` (weekly) → season totals per player. Cached in memory by (season, gsisIDs-hash) since it's static within a season.

**Canonical stat IDs**: introduce an internal stat vocabulary (e.g. `pass_yds`, `pass_td`, `rush_yds`, ...) that both the local provider and Yahoo mapping target. Add `internal/services/scoring/statids.go` with:
- The canonical list
- A `YahooToCanonical(statID string) string` map (expand the constants already in `draft_values.go`)
- A `CanonicalToNFLStatsColumn(id string) string` map for pulling from `nfl_player_stats`

This becomes the single place stat-ID mapping lives.

---

## Phase 2 — Ranking service takes pre-resolved stats

The ranking package (`internal/services/ranking/`) already takes `PlayerData` with `StatValues map[string]float64`. Nothing there is Yahoo-aware — good.

Refactor the **handler** `GetLeagueRankings`:

```
Before:
  Yahoo.GetLeagueRostersWithStats  ──► PlayerData (owner + stats)
  Yahoo.GetAvailablePlayersWithStats ──► PlayerData (FA + stats)

After:
  Yahoo.GetLeagueRosterOwnership     ──► (player_key, owner_team_key)  [new, lean]
  Yahoo.GetLeagueFreeAgents          ──► []player_key                  [new or trimmed]
  players.ResolveBatchYahooToGsis    ──► player_key → gsis_id
  nflstats.LoadSeasonStats           ──► gsis_id → canonical stats
  scoring.YahooCatsToCanonical       ──► rewrite categories to canonical IDs
```

Result: the existing `ranking.RankByPoints` / `ranking.RankByCategories` functions are unchanged. We only swap the data providers.

Yahoo calls that remain in `GetLeagueRankings`:
- `GetLeagueScoringStats` (league config)
- `GetLeagueRosterPositions` (league config)
- A lightweight roster-ownership call (just player_key + team_key, no stats)
- FA list (no stats)

If an ownership call that skips stats doesn't already exist on the Yahoo client, add it — it should be one `/teams;out=roster` request without the stat subresource.

**Fallback**: a Yahoo player with no `gsis_id` (rookie not yet in nflverse, DST, practice-squad) drops out of the ranking with a logged warning. Acceptable — happens only for a small set of edge-case players and is better than silently re-using Yahoo stats for some players and local stats for others.

**NBA path**: `isPointsLeague` already gates the NFL branch. Wrap the new local-stats path in the same gate; NBA keeps calling `GetLeagueRostersWithStats` until we have an NBA stats feed. Extract a small interface so the NBA branch can be migrated later without re-touching handlers.

---

## Phase 3 — Universal rankings endpoint

Add `GET /api/rankings` (public, no auth):

Query params:
- `season` — default current season
- `format` — `ppr` | `half` | `standard` | `superflex_ppr` (enum of a small set of standard formats)
- `position` — optional filter
- `scope` — `projection` (default, use `nfl_projections`) or `historical` (use `nfl_player_stats` of a past season)

Response: sorted list of `{ gsis_id, name, position, team, fpts, fpts_pg, grade, position_rank, overall_rank }` with no league-specific numbers.

Implementation: this is a thin handler over the data already in `nfl_projections` + `nfl_player_grades` + the canonical scoring in Phase 1. Zero Yahoo contact.

Frontend: new page at top-level `/rankings-public` (or fold into existing `/rankings` grades page as a new tab — cleaner since it's the same audience). Format toggle + position filter + clickable rows → `/players/:gsisId`.

---

## Phase 4 — Centralize scoring math

Today `draft_values.go` has `projToStatTotals` and `computeLeagueFpts` hard-wired to a handful of Yahoo stat IDs. Move both into `internal/services/scoring/`:

```go
func ProjectionToCanonicalTotals(p ProjectionRow, games float64) map[string]float64
func ScoreWithLeagueModifiers(totals map[string]float64, modifiers map[string]float64) float64
func ScoreWithStandardFormat(totals map[string]float64, format StandardFormat) float64
```

Both the league draft-values handler and the new public `/api/rankings` endpoint call into this. Removes the duplicate stat-ID constants in `draft_values.go`.

---

## Phase 5 — Coverage & cleanup

- Confirm `nfl_players.yahoo_id` coverage for active rostered players. Add a one-shot script that reports missing links after each Yahoo sync.
- Deprecate / delete `GetLeagueRostersWithStats` and `GetAvailablePlayersWithStats` usage from rankings code path once NBA is migrated (later).
- Docs: update `CLAUDE.md` route list + key patterns section, and the ranking algorithm doc to reflect the new data source.

---

## Tradeoffs

- **One extra DB aggregate per rankings request**. Cachable; small compared to the Yahoo round-trips removed.
- **Rookies without nflverse data**. Edge case; handle by falling back to projection-only stats (they'll have a row in `nfl_projections` once we run projections for that season).
- **Stat-ID mapping maintenance**. One centralized map is the price of removing the Yahoo stats dependency — worth it.
- **Staleness**. Our stats refresh on `make import-nfl` cadence, not live. For in-season rankings this means a ~daily lag vs Yahoo's live stats. Document this; offer a "refresh" button that triggers the import if latency matters.

## Not doing now

- NBA decoupling (needs a stats feed first)
- Live in-game stat pulls (Yahoo stays the authority for live scoreboards/matchups)
- Changing the `/api/leagues/{id}/draft-values` contract — it's already mostly local except for the league-config reads, which are correct uses of Yahoo.
