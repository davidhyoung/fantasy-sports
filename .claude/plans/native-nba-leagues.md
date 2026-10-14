# Native NBA leagues

## Status (2026-10-10)

- **This pass shipped in full: data pipeline, league creation, rosters, schedule generation, weekly category scoring, standings.** `nba_players`/`nba_player_stats` (migration 000033), `league_rosters_nba` (000034), `league_matchups_nba`/`league_matchup_nba_categories`/`league_settings.season_start_date` (000035). `internal/services/espnnba` (client + mapping, unit-tested against a real captured box score) and `cmd/import-nba` (`-rosters`, `-stats -from -to`) — verified against real live data: 618 players, 944 real player-game rows across 25 completed games imported with 0 failures (after adding a player-stub fallback for box-score players not yet on a tracked roster — preseason squads run larger than the 15-man regular-season roster `-rosters` syncs). `CreateLeague` needed zero backend changes (already fully sport-generic — `slots`/`scoring` are opaque JSONB with no vocabulary enforcement beyond rejecting `DEF`). New handlers: `league_rosters_nba.go`, `schedule_nba.go`, `scoring_nba.go`. Frontend: sport picker in `CreateLeague.tsx`, `NativeRosterTabNBA`/`NativeScoreboardTabNBA`/`NativeStandingsTabNBA`/`NativePlayersTabNBA`, `league-detail/index.tsx` branches on `league.sport === 'nba'`. Verified end-to-end against real ESPN data (Giannis Antetokounmpo, Anthony Edwards, Alperen Sengun, etc.) in the browser: roster assign/drop, schedule generation, week scoring, the category grid (every `won` flag arithmetically checked, including turnovers' lower-is-better and FG%/FT% aggregated-not-averaged), and standings (match record + cumulative category record) all correct. One real bug caught and fixed during browser verification: adding a free agent didn't invalidate the free-agents query, so they lingered in that list after being rostered.
- **Key finding that reshaped the roster slot design:** ESPN's own NBA position taxonomy only has three buckets — Guard/Forward/Center — confirmed against real roster data. No PG/SG or SF/PF split exists to key a narrower slot off of, so the roster slot vocabulary is `G/F/C/UTIL/BN/IR`, not the originally-planned `PG/SG/SF/PF/C/G/F/UTIL`.
- **Deferred to a later phase, deliberately** (see "Deferred" section below): free agency/waivers, trades, draft, dynasty contracts/salary cap, live in-game score polling, message board, draft prep, comp-based projections, playoffs/bracket seeding.

## Context

The app's native (non-Yahoo) league system — rosters, contracts, cap, draft,
free agency, weekly scoring, message board — was built entirely for football
over two prior multi-phase efforts (`.claude/plans/native-leagues.md`,
`.claude/plans/dynasty-transactions.md`). NBA today is Yahoo-only: `leagues.sport`
has existed since the first migration but is only ever read to pick
points-vs-categories *ranking display* mode for Yahoo leagues
(`analysis.go:95-98` — `sport == "nfl"` → points, anything else → Yahoo
categories). `CreateLeague` hardcodes `sport: 'nfl'` on the frontend
(`CreateLeague.tsx:130`) and every native-league table/handler/component is
built directly against `gsis_id`/`nfl_players`. `.claude/plans/native-leagues.md:270`
already flagged this exact gap: *"NBA native leagues (no non-Yahoo NBA stats
feed...)"* — true until now. ESPN's same hidden API family already proven
reliable in this app for NFL live scores also covers NBA scoreboards,
boxscores, and full team rosters (verified directly against real games this
session), which removes that blocker without needing a new data source.

Per your direction: **category/roto scoring** (points, rebounds, assists,
steals, blocks, turnovers, 3PM, FG%, FT%), **weekly head-to-head** matchups
(not daily lineups), and an **MVP-first phased build**. This plan covers
everything through working category scoring + standings — league creation,
rosters, a real NBA stats pipeline, schedule generation, and weekly scoring.
Free agency, trades, draft, dynasty contracts/cap, live in-game polling, and
comp-based projections are explicitly deferred (see "Deferred" at the end) —
mirroring how the NFL side itself shipped settings/rosters before weekly
play, and weekly play before cap/draft/free agency.

## Key design decisions

- **New parallel tables, not a unified schema.** The explore pass confirmed
  `leagues`/`teams`/`league_draft_picks`/`league_transactions` are already
  schema-generic and get reused as-is (discriminated by `leagues.sport`).
  But `league_rosters` has a hard `gsis_id REFERENCES nfl_players` FK and a
  football-only slot CHECK constraint, and every stat-fetching function
  (`ScoreLeagueWeek`, `relevantPlayerStats`, `weeklyTrend`) calls
  `nflstats.*`/raw `nfl_player_stats` SQL directly with no sport-dispatch
  seam. Retrofitting a union-typed player reference into the existing NFL
  tables would be riskier than this codebase's own established pattern
  (native vs. Yahoo already live side-by-side as parallel verticals, not a
  forced unification). So: new `nba_players`, `nba_player_stats`,
  `league_rosters_nba`, `league_matchups_nba` — same shapes/conventions as
  their NFL counterparts, FK'd to `nba_players` instead.
- **ESPN athlete ID *is* the player identity**, not a crosswalk column.
  Unlike NFL (where `nfl_players.gsis_id` is primary and `espn_id` is a
  secondary crosswalk column populated from nflverse), NBA has no
  nflverse-equivalent bulk source with a maintained ID crosswalk (confirmed
  via research — `nba_api`/balldontlie/Basketball-Reference none ship one).
  Since ESPN is the sole real-stats source for this phase anyway,
  `nba_players.espn_id` is just the primary key directly — no crosswalk step
  needed at all.
- **Stats stored per-game, not per-week.** NFL stores one row per
  (player, week) since there's exactly one game a week. NBA teams play
  2-4x/week, so `nba_player_stats` is one row per (player, real NBA game);
  weekly category totals are summed across however many games fell in that
  fantasy week at scoring time.
- **FG%/FT% are computed from aggregated makes/attempts, never averaged.**
  Storing raw `fg_made`/`fg_att`/`ft_made`/`ft_att` per game and summing
  before dividing is the only correct way to combine percentage stats across
  multiple games — averaging per-game percentages would badly distort
  low-volume games.
- **A category matchup's winner is "most categories won," with per-category
  results also kept** (not just a single winner flag) — both because that's
  how real roto leagues work (ties on individual categories are possible)
  and because a league's standings page needs the cumulative category
  record, not just a match win/loss tally.

## Design

### Data pipeline (new, foundational)

**Migration** — `nba_players` (mirrors `nfl_players`, keyed by `espn_id`
directly: name, position, team, birth_date, height, weight, college,
years_exp, jersey_number, headshot_url, updated_at) and `nba_player_stats`
(one row per real game: `espn_id`, `espn_event_id` dedup key, `game_date`,
`season`, `team`, `opponent`, `minutes`, `points`, `rebounds`
[+`offensive_rebounds`/`defensive_rebounds`], `assists`, `steals`, `blocks`,
`turnovers`, `fouls`, `fg_made`/`fg_att`, `fg3_made`/`fg3_att`,
`ft_made`/`ft_att`; `UNIQUE (espn_id, espn_event_id)`).

**`internal/services/espnnba/`** (new package, mirrors
`internal/services/espnlive`'s already-proven client/mapping pattern):
- `client.go` — scoreboard (`site.api.espn.com/.../basketball/nba/scoreboard?dates=YYYYMMDD`),
  boxscore (`.../summary?event=`), and roster
  (`.../basketball/nba/teams/{id}/roster`, ids 1-30) fetchers. Same
  browser-UA `http.Client` approach already confirmed to get past Akamai's
  block on plain `curl`/`WebFetch`.
- `mapping.go` — parses the boxscore's one flat stat group (`keys`: minutes,
  points, `fieldGoalsMade-fieldGoalsAttempted`,
  `threePointFieldGoalsMade-threePointFieldGoalsAttempted`,
  `freeThrowsMade-freeThrowsAttempted`, rebounds, assists, turnovers,
  steals, blocks, ...) into `nba_player_stats` columns — reuses the exact
  composite "made-attempted" string-splitting helper already written for
  NFL's completions/attempts (`espnlive/mapping.go`'s `firstOfComposite`/
  `secondOfComposite`), just ported into this package since it's a tiny,
  source-agnostic string util.

**`backend/cmd/import-nba/main.go`** (new CLI, mirrors `cmd/import`'s
manual-batch-import philosophy): `-rosters` upserts `nba_players` from all
30 teams' roster endpoints; `-stats -from YYYY-MM-DD -to YYYY-MM-DD`
walks each date, pulls that day's completed games from the scoreboard, and
upserts their boxscores into `nba_player_stats` (idempotent — same
`ON CONFLICT (espn_id, espn_event_id)` upsert pattern as every other
importer in this codebase). A `Makefile` target `import-nba` matching
`import-nfl`'s calling convention.

### League creation, settings, rosters

- `leagues`/`league_settings`/`teams` — reused as-is with `sport = 'nba'`.
  `league_settings.scoring` JSONB gets a parallel meaning for NBA leagues: a
  flat `{category: 1}` inclusion map (which of the 9 categories count) rather
  than NFL's per-stat point weight — same column, sport-dependent
  interpretation, consistent with how `slots` already means something
  different per sport today.
- **NBA slot vocabulary**: `G, F, C, UTIL, BN, IR` (dedicated G/F/C, UTIL =
  any position) — ESPN's own NBA position taxonomy only has these three
  buckets (confirmed against real roster data, see Status above), so there's
  no PG/SG or SF/PF split to key a narrower slot off of. A new
  `league_rosters_nba.slot` CHECK constraint, plus an NBA eligibility map
  mirroring `league_rosters.go`'s `slotEligibleForPosition` (a parallel Go
  map, not a rewrite of that function's logic shape).
- **`league_rosters_nba`** (migration): `(league_id, team_id, espn_id, slot,
  acquired_via ['draft','trade','waiver','fa'], acquired_at)`, PK
  `(league_id, espn_id)` — same shape as `league_rosters` minus the
  contracts/cap columns (deferred — see below).
- **`internal/handlers/league_rosters_nba.go`** (new, mirrors
  `league_rosters.go`): `GetLeagueRostersNBA`, `AssignLeagueRosterNBA`,
  `DropLeagueRosterNBA`, `GetLeagueFreeAgentsNBA` (nba_players minus
  league_rosters_nba) — no contract/cap logic at all this phase, just slot
  assignment + eligibility + the same per-category stat-breakdown pattern
  `relevantPlayerStats` already established (reusable *shape*, new
  NBA-specific implementation since the stat source differs entirely).
- **`CreateLeague`** (`leagues.go`) gains an NBA branch: when
  `req.Sport == "nba"`, validate against the NBA slot vocabulary instead of
  `validateNativeSlots`'s football one, and write default `scoring` as the
  9-category inclusion map instead of point weights.
- **Frontend**: `CreateLeague.tsx` gets a sport picker (NFL/NBA) — when NBA
  is selected, the slots editor shows NBA positions and the scoring editor
  becomes category checkboxes instead of point-value inputs, not a football
  form with basketball labels bolted on.

### Schedule + weekly category scoring

- `roundRobinRounds`/`GenerateLeagueSchedule`'s round-robin logic
  (`schedule.go:112-231`) is confirmed sport-generic — reused directly, just
  writing into a new `league_matchups_nba` table (same shape as
  `league_matchups`: `league_id, season, week, team1_id, team2_id,
  is_playoffs, computed_at`) instead of pointing-mode's.
  `league_settings` gains a `season_start_date` column (NBA has no existing
  "what calendar date is week N" concept, unlike NFL where week numbers
  already map onto real NFL weeks) — week N's games are `season_start_date +
  7*(N-1)` through `+7*N-1`.
- **`league_matchup_nba_categories`** (migration, child table): `(matchup_id,
  team_id, category, value, won)` — one row per team per category per
  matchup, `category ∈ {points, rebounds, assists, steals, blocks,
  turnovers, three_pm, fg_pct, ft_pct}`.
- **`ScoreLeagueWeekNBA`** (`internal/handlers/schedule_nba.go`, new,
  mirrors `ScoreLeagueWeek`'s explicit-re-runnable-commissioner-action
  philosophy): for each matchup, for each team, sum its starter-slotted
  players' `nba_player_stats` rows falling within that week's date range;
  aggregate into the 9 categories (FG%/FT% computed from summed
  makes/attempts, not averaged); write one `league_matchup_nba_categories`
  row per team per category with `won` set (ties → both `false` or a
  separate `tied` flag); stamp `league_matchups_nba.computed_at`.
- **`GetLeagueScoreboardNBA`/`GetLeagueStandingsNBA`**
  (`internal/handlers/scoring_nba.go`, new, mirror `nativeScoreboard`/
  `nativeStandings`): scoreboard returns each matchup's full 9-category
  breakdown per team (so the UI can render a real roto grid, not just a
  single score); standings aggregate **matchup** win/loss/tie (majority of
  categories) *and* the cumulative category win/loss record across the
  season (the standard roto-league tiebreaker), both pure reads over
  `league_matchup_nba_categories` — no separate standings table, same
  convention as the NFL side.
- **Frontend**: new `league-detail/nba/` components —
  `NativeScoreboardTabNBA` (a 9-row category grid per matchup, winning side
  bolded per category, "6-3" summary), `NativeStandingsTabNBA` (match
  record + category record), `NativeRosterTabNBA`/`NativeRosterTableNBA`
  (same slot-picking UX as the NFL version, NBA slot vocabulary).
  `league-detail/index.tsx` gains a `league.sport === 'nba'` branch
  alongside its existing `source` (yahoo/native) branch, rendering the NBA
  tab set instead of the NFL native one.

## Deferred (explicitly out of scope this pass)

- Free agency/waivers, trades, draft, dynasty contracts/salary cap — the
  entire `dynasty-transactions.md` apparatus. A redraft-style roster
  (assign/drop only, no salary) is enough for category scoring to mean
  something; cap/contracts are a layered addition later, exactly like the
  NFL side's own history.
- Live in-game score polling (an NBA `espnnba` poller mirroring
  `espnlive`) — this phase's stats are the same manual/periodic
  `make import-nba` batch model the NFL side used before live polling
  existed.
- Message board, draft prep, comp-based projections — `nba_api`-sourced
  historical backfill would be needed for projections specifically (ESPN
  alone doesn't have the multi-season depth); not needed until a projection
  engine is actually wanted.
- Playoffs/bracket seeding — same scope cut the NFL side already carries.

> **Status addendum (October 2026): most of the above "deferred" list is no
> longer deferred.** Draft prep (`NativeDraftPrepTabNBA.tsx`), comp-based
> projections (`internal/handlers/nba_projections.go`, live/on-demand) and
> historical comparisons (`nbaPlayerComps`) shipped in a later pass, using the
> ESPN-only backfill after all (now 31 seasons, 1996-2026, confirmed complete
> locally) rather than waiting on `nba_api`. A further pass (also October
> 2026) added a *second*, batch/backtestable projection pipeline
> (`cmd/project-nba`), a backtest/autotune harness, and external consensus
> rankings (`nba_consensus_rankings`, the Draft Prep "Consensus" view) — see
> `docs/nba-projection-algorithm.md`, `docs/nba-algorithm-review.md`, and
> `docs/stats/nba-consensus-rankings.md` for what actually shipped. Message
> board and playoffs/bracket seeding remain genuinely deferred.

## Files touched (this pass)

- New migration(s): `nba_players`, `nba_player_stats`,
  `league_rosters_nba`, `league_matchups_nba`,
  `league_matchup_nba_categories`, `league_settings.season_start_date`
- `backend/internal/services/espnnba/{client,mapping}.go` (new)
- `backend/cmd/import-nba/main.go` (new), `backend/Makefile` (new target)
- `backend/internal/handlers/league_rosters_nba.go`,
  `schedule_nba.go`, `scoring_nba.go` (new)
- `backend/internal/handlers/leagues.go` (`CreateLeague` NBA branch)
- `backend/cmd/api/main.go` (new routes)
- `frontend/src/pages/CreateLeague.tsx` (sport picker)
- `frontend/src/pages/league-detail/nba/` (new: scoreboard/standings/roster
  tab components), `frontend/src/pages/league-detail/index.tsx` (sport
  branch), `frontend/src/api/client.ts`/`queryKeys.ts` (new types/calls)
- `CLAUDE.md` (document the new NBA vertical alongside the NFL one)

## Verification

1. `make import-nba ARGS="-rosters"` then `-stats -from <a recent date> -to
   <today>` against local Postgres — confirm real players/games land (spot
   check a known player's real recent box score against espn.com).
2. `go build ./... && go vet ./... && go test ./...`.
3. Create a test NBA league via the API, assign a full roster, generate a
   schedule, run `ScoreLeagueWeekNBA` for a week with real imported games —
   confirm category values and per-category winners are arithmetically
   correct (spot check FG%/FT% specifically, since that's the easiest place
   for an averaging-vs-aggregating bug to hide).
4. `tsc`/`vite build` clean; browser check (Chrome MCP) creating an NBA
   league end-to-end and viewing the category scoreboard grid.
