# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

> **Keep this file and `memory/project_fantasy_sports.md` up to date** whenever you add routes, handlers, pages, or schema changes. This file documents *current state only* — implementation history, bug-fix narratives, and iteration-by-iteration UI changes belong in git log / `docs/HANDOFF.md` / `.claude/plans/`, not here.

## Project Overview

Full-stack fantasy sports web app (multi-sport: NFL, NBA). Go backend + React/Vite/TypeScript frontend + PostgreSQL. Users authenticate via Yahoo OAuth, sync their Yahoo fantasy leagues, and view live rosters, scoreboards, standings, and matchup details. Also supports fully **native** (non-Yahoo) leagues — see "Native leagues" below.

## Architecture

```
backend/   Go API server (Chi router, pgx/pgxpool, golang-migrate, gorilla/sessions, Yahoo OAuth2)
frontend/  React 18 + Vite 5 + TypeScript + Tailwind CSS + shadcn/ui + TanStack Query v5
```

**Backend layout:**
- `cmd/api/main.go` — entry point; wires router, DB pool, sessions, oauth config, handlers
- `cmd/import/main.go` — CLI tool to download nflverse CSV data and upsert into `nfl_players` / `nfl_player_stats`
- `internal/handlers/` — one file per resource; all handlers are methods on `Handler{db, sessions, oauthConfig, config}`
  - `handlers.go` / `respond.go` — Handler struct + constructor; JSON response helpers
  - `auth.go` — Login, Callback, Me, Logout
  - `leagues.go` — ListLeagues, GetLeague, CreateLeague (creates a **native** league — settings + teams in one transaction; Yahoo leagues arrive only via `/api/sync`, never through this endpoint), `requireCommissioner()` (shared auth check for native-league mutations, `leagues.user_id` must match the caller)
  - `native_leagues.go` — GetLeagueSettings, UpdateLeagueSettings, CreateLeagueTeam, UpdateLeagueTeam, DeleteLeagueTeam: native-league-only CRUD, commissioner-gated. Rejects a nonzero `DEF` slot (`validateNativeSlots`) — nflverse has no team-defense `gsis_id`s, so a DEF roster row can never be filled
  - `rollover.go` — RolloverLeague: advances a native league to its next season, strategy selected by `leagues.format` (`redraft`/`dynasty` implemented, `keeper` 501s — no native keeper-designation flow exists). `rolloverDynasty` carries every contract forward (`years_used++`), releases expired contracts to FA, banks each team's unused cap space into `league_team_seasons` (can go negative if a team finished over cap — no amnesty), generates next season's rookie picks, opens a fresh offseason FA window. `rolloverRedraft` releases everyone, regenerates a full draft class, and does not bank cap (nothing persists). Idempotent via `lockLeagueAtSeason` (locks the league row, confirms `leagues.season` hasn't moved). 422s if a free-agency window is still open.
  - `league_rosters.go` — GetLeagueRosters, AssignLeagueRoster, UpdateLeagueRoster, DropLeagueRoster, GetLeagueFreeAgents: roster + contract management. Every roster row gets a contract row (even a $0 undrafted pickup) via the shared `assignRosterTx` helper (also used by `draft_picks.go`). `GetLeagueFreeAgents` is `nfl_players` minus `league_rosters`, ordered by projection, with an optional `?search=` filter. `validSlots` excludes `DEF`. Enforces lineup-slot eligibility (`slotEligibleForPosition`): QB/RB/WR/TE/K slots take only that position, FLEX takes RB/WR/TE/K, SFLEX takes anyone, BN/TAXI/IR take anyone. `DropLeagueRoster` writes dead money before cascading the roster/contract rows. Also home for `leagueTeamIDs`, `generateDraftClassTx`, and `relevantPlayerStats` (per-player projected stat totals restricted to whichever categories the league's own scoring weights nonzero — backs the dynamic stat columns on Roster/Players). Mutations commissioner-gated; reads RequireAuth only.
  - `draft_picks.go` — ListLeagueDraftPicks, GenerateLeagueDraftPicks, UseLeagueDraftPick: tradable future draft picks (`league_draft_picks`). `GenerateLeagueDraftPicks` is idempotent (409 if a class already exists for that season). `UseLeagueDraftPick` locks the pick row, derives salary/years from the rookie scale (`rookie_scale.go`) rather than accepting client input, and rejects a pick whose season isn't the league's current one.
  - `trades.go` — CreateLeagueTrade (atomic multi-asset trade between teams — locks every asset, validates destinations, rejects no-op moves, requires ≥2 teams; cap-checked per team across the *whole* trade before any write applies), ListLeagueTransactions (ungated read over the shared activity log every mutation writes to).
  - `cap.go` — `teamCap` prices a team's roster for any season (dead money and multi-year deals project forward automatically). `capCheckAdd`/`capCheckDelta` are the two enforcement gates, wired into every roster/pick/trade mutation. **The cap blocks additions only, never existing obligations** — dead money, rookie contracts, and rollover can all push a team over cap; only adding something new is refused.
  - `rookie_scale.go` — prices a drafted rookie off the league's own real auction-value board (`computeDraftBoard`, shared with `draft_values.go`) at a rank interpolated by the pick's position in its class (`league_settings.rookie_scale`'s `top_pct`/`bottom_pct`); contract length by round (`years_by_round`).
  - `free_agency.go` — competitive free agency (no FAAB): teams submit ranked-priority contract offers against one open window per league (`league_fa_windows`/`league_fa_offers`). `ResolveFAWindow` resolves players best-market-value-first, reserving each team's cap for its higher-priority pending offers before considering lower ones; offers below `player_value × fa_reservation_pct` never win. Winning offer = `salary × years × fit(years, preferred_length)`.
  - `message_board.go` — per-league discussion board: threads/replies (one level of nesting), reactions (single "+1"), polls, pinning, read tracking. Posts are authored by a *team*, not a user (no per-manager identity beyond team ownership). `RequireAuth`, not commissioner-gated except pinning. Activity feed interleaves stored posts with events derived from `league_transactions`/`league_matchups` (no separate event storage).
  - `teams.go` — ListLeagueTeams, GetTeam, GetTeamRoster (with stat period support); gsis_id batch lookup for player detail links
  - `players.go` — ListPlayers, CreatePlayer, GetPlayer
  - `scoring.go` — GetLeagueScoreboard, GetLeagueStandings: dispatch on `leagues.source` (`leagueSource()`) — Yahoo leagues hit the Yahoo API; native leagues read `league_matchups` directly (standings are a pure aggregation over scored matchup rows, no separate standings table)
  - `schedule.go` — GenerateLeagueSchedule (round-robin "circle method", idempotent per season), ScoreLeagueWeek (an **explicit, re-runnable commissioner action, not automatic** — sums each team's starter-slotted players' real stat lines through `scoring.ScoreWithModifiers` and freezes the result onto `league_matchups`; underlying stats come from a manual `make import-nfl` batch import, never live). Regular season only — no playoff bracket/seeding.
  - `yahoo_helpers.go` — `leagueYahooKey()`, `userTokens()`, `newYahooClient()`; `leagueSettingsSource()` dispatches on `leagues.source` to resolve a `leaguesettings.Source` (native vs. live Yahoo)
  - `sync.go` — Sync (Yahoo league+team upsert)
  - `league_players.go` — SearchLeaguePlayers, GetAvailablePlayers (Yahoo leagues only — no native equivalent yet)
  - `keepers.go` — Yahoo-draft-result-shaped keeper rules/wishlist/results/summary endpoints
  - `analysis.go` — GetLeagueRankings: weighted z-score rankings (Yahoo leagues only; not yet wired for native). Category weights = CV × FA-scarcity (normalised); `overall_score` = weighted z-sum; `position_score` = z-score within position group. NFL season stats source locally from `nfl_player_stats` via `services/nflstats`; non-season stat types + NBA fall back to Yahoo. See `docs/ranking-algorithm.md`
  - `projections.go` — ListProjections, GetProjectionDetail: pre-computed comp-based NFL player projections
  - `rankings_public.go` — ListPublicRankings: no-auth projection-based rankings with PPR/Half/Standard toggle
  - `nfl_players.go` — GetNFLPlayer (metadata + YoY stats + projection + notes; adds a `contract` field when `?league_id=` is given and the caller has a session), GetNFLPlayerByYahooID
  - `draft_values.go` — GetDraftValues: league-specific auction values (VOR + $), resolving roster positions/scoring via `services/leaguesettings` (Yahoo or native, transparently), `services/scoring` for stat-ID translation, `services/tiers` to group interchangeable players
  - `grades.go` — ListGrades, GetPlayerGrades: real-life player grades (0–100 percentile)
  - `draft_prep.go` — GetDraftPrep, UpsertDraftPrepPlayer, ReorderDraftPrep: personal draft board (`draft_prep_players`), scoped to (user, league, season)
  - `draft_consensus.go` — `loadConsensusValues`: consensus auction value per player on the league's dollar scale — prefers imported market data, otherwise derives it by reading our own value curve at the market's median within-position rank. See `docs/stats/auction-values.md`
  - `divergences.go` — ListDivergences: projection-vs-consensus rank gaps + situational notes, ordered by |delta|
- `internal/models/models.go` — shared domain types (User, League, Team, Player, RosterEntry, LeagueSettings)
- `internal/middleware/auth.go` — RequireAuth: reads session, attaches *models.User to ctx
- `internal/yahoo/` — Yahoo Fantasy API client, OAuth config, XML types (`client.go`, `oauth.go`, `types.go`), plus `mock.go`/`mockdata.go`/`mockplayers.go` for `YAHOO_MOCK` mode (see below)
- `internal/services/scoring/` — canonical stat-ID vocabulary + Yahoo-stat-ID translation + projection→canonical-total helpers; the single place stat-ID knowledge lives
- `internal/services/tiers/` — groups players whose value is close enough to be interchangeable (a tier's spread stays inside `draftable range ÷ targetTiers`, default 8), per position. See `docs/stats/tiering.md`
- `internal/services/nflstats/` — season-level aggregation of `nfl_player_stats` keyed by gsis_id; the NFL stats source for rankings (not Yahoo)
- `internal/services/leaguesettings/` — the fantasy-context vocabulary (roster slot names, per-stat point values) and a `Source` interface (`RosterPositions`, `ScoringMods`); `YahooSource` wraps live Yahoo calls, `NativeSource` reads a native league's `league_settings` row. See `.claude/plans/native-leagues.md`
- `internal/db/db.go` — pgxpool connect helper
- `migrations/` — numbered SQL migration files

**Frontend layout:**
- `src/api/client.ts` — all typed API functions + TypeScript interfaces
- `src/api/queryKeys.ts` — all TanStack Query cache keys
- `src/lib/queryClient.ts` — QueryClient config (staleTime: 30s, retry: 1)
- `src/lib/utils.ts` — cn() utility, zScoreIndicator, zScoreColor, contract-label formatting
- `src/lib/grades.ts` — shared grade display utilities
- `src/lib/constants.ts` — CURRENT_SEASON (2025), PROJECTION_SEASON (2026)
- `src/pages/` — pages by route; complex pages split into subdirectories:
  - `Home.tsx` — Leagues home: hero + your-leagues list (format badge, Yahoo sync-status tag) + "Player Outlooks" signal cards
  - `league-detail/` — `index.tsx` + tabs: `MyTeamTab` (Yahoo only), `StandingsTab`/`NativeStandingsTab`, `NativeRosterTab` (native leagues' single combined "my team"/"every team" page, second tab), `ScoreboardTab`/`NativeScoreboardTab`, `PlayersTab`/`NativePlayersTab`, `DraftSection` (wraps `DraftTab`/`KeepersTab` or `NativeDraftPicksTab` sub-sections), `NativeFreeAgencyTab` (native only), `MessagesTab` (native only, discussion board). `components/`: `PlayerAssignForm`, `TradeBuilder`, `NativeRosterTable`, `NativeTeamOverview`, `EditContractForm`, `FAOfferForm`, `DraftSettingsPanel`, `ActivityRail`, `PostCard`/`PostComposer`/`PollBar`/`ThreadDetailView`, `L4Sparkline`. `lib/nativeSlots.ts` (slot vocabulary + eligibility table). Yahoo-only team pages resolve through `TeamDetailRouter.tsx`, which redirects a native team into its league's Roster tab instead.
  - `team-detail/`, `matchup-detail/` — Yahoo-only detail pages (`TeamPanel`/`RosterTable`/`MatchupCard`, `CategoryTotalsTable`/`TeamRosterTable`)
  - `player-detail/` — `PlayerDetailBody.tsx` (metadata, grade card, YoY stats, projection w/ PPR/Half/Standard toggle, comps, contract card when reached via `?league=`) + `index.tsx` (route) + `PlayerDetailPanel.tsx` (same body as a `SidePanel`/`MobileSheet`, opened in place from a table row)
  - `statistics/` — `/statistics` (Projections/Grades view toggle via `?view=`); absorbed the former standalone `/rankings` and `/projections` pages
  - `draft-prep/` — `/draft-prep`: a Board/Tiers layout toggle over `DraftBoardTable` (sortable table) or `TiersView` (players bucketed by position then tier, both shared read-only by league-detail's Draft tab) + a dockable side panel (`TeamPanel`) with two buckets — Settings (`DraftSettingsPanel`) and Plan (`TargetList` or the team-builder)
  - `divergences/` — `/divergences`: full consensus-divergence table (linked from Home, not in main nav)
  - `wiki/` — `/wiki`: static stat-engine reference page, content mirrors `docs/*.md`
- `src/components/ui/` — shadcn/ui components (badge, button, input, table, tabs) + table-helpers (SortableHead, PlayerCell, TeamAvatar, ClickableRow, ZScoreCell, HeaderRow) + hand-rolled zero-animation primitives with no Radix dependency: `mobile-sheet.tsx`, `dialog.tsx`, `side-panel.tsx`, `responsive-dialog.tsx` (mounts both Dialog and MobileSheet, call sites don't branch on breakpoint)
- `src/App.tsx` — router + nav + auth check; `src/main.tsx` — React root
- Vite proxies `/api` and `/auth` → `http://localhost:8080` in dev
- HTTPS via `vite-plugin-basic-ssl` (required for Yahoo OAuth)

## Routes

Frontend (nav: Leagues, Draft Prep, Statistics, Wiki):
```
/                                       Leagues home — hero, league list, Player Outlooks
/draft-prep                             Draft Prep — board (ranks, interest scale −3..+3, notes) + dockable team panel
/wiki                                   Stat-engine reference — grades, projections, rankings, auction pricing, tiering, consensus (static, no auth)
/leagues                                → redirect to /
/leagues/{id}                           League detail (Standings/Roster[native]/Scoreboard/Players/Draft/Free Agency[native]/Messages[native]/My Team[Yahoo])
/leagues/{id}?tab=draft&sub=values|keepers  Draft tab sub-sections (?tab=keepers redirects here)
/leagues/{id}/my-team                   → redirect to /leagues/{id}?tab=my-team (Roster tab for native)
/leagues/{leagueId}/matchup/{week}/{t1}/{t2}
/teams/{id}                             Team roster — Yahoo, or a redirect into the league's Roster tab for native
/statistics?view=projections|grades     Player data — projections + real-life grades
/rankings, /projections                 → redirect to /statistics?view=...
/divergences                            Full consensus-divergence table (linked from Home)
/players/{gsisId}                       Player detail
/projections/{gsisId}                   → redirect to /players/{gsisId}
```

API:
```
Public:
  GET  /auth/login, /auth/callback, /auth/logout
  GET  /api/health
  GET  /api/leagues, GET /api/leagues/{id}
  GET  /api/leagues/{id}/teams
  GET  /api/teams/{id}
  GET/POST /api/players, GET /api/players/{id}
  GET  /api/projections?season=&position=&sort=&limit=&offset=
  GET  /api/projections/{gsisId}?season=
  GET  /api/nfl/players/{gsisId}?league_id=   — full player detail; league_id adds `contract` (native-league team/slot/salary/years), only if the caller has a session
  GET  /api/nfl/players/by-yahoo/{yahooKey}   — resolves Yahoo key → HTTP redirect to /api/nfl/players/{gsisId}
  GET  /api/grades?season=&position=&limit=&offset= — position supports comma-separated (e.g. RB,WR,TE)
  GET  /api/grades/{gsisId}                  — all seasons of grades for a player
  GET  /api/rankings?season=&format=ppr|half|standard&position=&limit=&offset= — projection-based public rankings (no Yahoo, no login)
  GET  /api/divergences?season=&format=ppr|half_ppr|standard&position=&limit=&offset= — projection vs consensus gaps + situational notes

Protected (RequireAuth):
  GET  /api/auth/me
  POST /api/sync
  POST /api/leagues                                      — creates a native league (settings + teams, one transaction)
  GET  /api/leagues/{id}/settings                         — native leagues only
  PUT  /api/leagues/{id}/settings                         — commissioner only; replaces slots/scoring/budget/num_teams wholesale
  POST /api/leagues/{id}/teams                            — commissioner only
  PUT  /api/leagues/{id}/teams/{teamId}                   — commissioner only; {name?, claim?} — renames and/or claims/releases the team as the caller's own
  DELETE /api/leagues/{id}/teams/{teamId}                 — commissioner only
  GET  /api/leagues/{id}/rosters                          — every rostered player + contract terms + per-category projected stats, native only
  POST /api/leagues/{id}/rosters                          — commissioner only; {gsis_id, team_id, slot, acquired_via, salary, years_total}; 409 if already rostered
  PUT  /api/leagues/{id}/rosters/{gsisId}                 — commissioner only; move team (trade), slot, and/or contract terms
  DELETE /api/leagues/{id}/rosters/{gsisId}               — commissioner only; drop to free agency (contract cascades, writes dead money)
  GET  /api/leagues/{id}/free-agents?position=&season=&limit=&offset=&search= — unrostered players in a native league
  GET  /api/leagues/{id}/teams/{teamId}/cap?seasons=N      — hard-cap breakdown (budget, banked, active spend, dead money, available, roster count/max) for current + N future seasons
  GET  /api/leagues/{id}/picks?season=&team_id=            — tradable future draft picks
  POST /api/leagues/{id}/picks/generate                    — commissioner only; {season?, rounds?}; idempotent
  POST /api/leagues/{id}/picks/{pickId}/use                — commissioner only; {gsis_id, slot?}; spends the pick — salary/years derived from rookie scale, not client-set
  POST /api/leagues/{id}/trades                            — commissioner only; {assets:[{kind:"player"|"pick", gsis_id?, pick_id?, to_team_id}]}; ≥2 assets, ≥2 teams involved
  GET  /api/leagues/{id}/transactions?season=               — recent league activity (draft/trade/rollover/sign/drop), newest first
  POST /api/leagues/{id}/fa/windows                          — commissioner only; {kind?: "offseason"|"weekly", week?}; 409 if one's already open
  GET  /api/leagues/{id}/fa/windows                          — every window this league has opened, newest first
  POST /api/leagues/{id}/fa/windows/{windowId}/resolve        — commissioner only; runs every pending offer to a conclusion, signs winners, 409 if already resolved
  GET  /api/leagues/{id}/fa/offers?window_id=&team_id=       — offers for a window (defaults to the currently open one)
  POST /api/leagues/{id}/fa/offers                            — commissioner only; {gsis_id, team_id, salary, years, priority?}; upserts on window+player+team
  DELETE /api/leagues/{id}/fa/offers/{offerId}                — commissioner only; withdraws a pending offer in a still-open window
  PUT  /api/leagues/{id}/fa/offers/priority                   — commissioner only; {team_id, offer_ids:[...]}; array position becomes priority order
  GET  /api/leagues/{id}/fa/valuations?gsis_ids=a,b,c        — each player's auction value, reservation floor, preferred contract length
  POST /api/leagues/{id}/rollover                          — commissioner only; {to_season?, rounds?}; 422s if a free-agency window is still open
  GET  /api/leagues/{id}/scoreboard?week=N                — Yahoo API for Yahoo leagues, league_matchups for native
  POST /api/leagues/{id}/schedule/generate                — commissioner only, native only; {season?, weeks?}; idempotent
  POST /api/leagues/{id}/scoreboard/score?week=N&season=  — commissioner only, native only; freezes that week's matchup points from real stats
  GET  /api/leagues/{id}/standings                        — Yahoo API for Yahoo leagues, aggregated from league_matchups for native
  GET  /api/leagues/{id}/players?search=q
  GET  /api/leagues/{id}/players/available?position=&start=&status=
  GET  /api/leagues/{id}/draftresults
  GET  /api/leagues/{id}/keepers
  GET/PUT /api/leagues/{id}/keeper-rules
  GET  /api/leagues/{id}/keeper-summary
  GET  /api/leagues/{id}/rankings?stat_type=season
  GET  /api/leagues/{id}/draft-prep?season=              — personal board: interest, custom ranks, tier overrides, notes
  PUT  /api/leagues/{id}/draft-prep/order?season=        — {"gsis_ids":[...]}; array position becomes the rank
  PUT  /api/leagues/{id}/draft-prep/{gsisId}?season=     — {interest, custom_rank, custom_tier, note, planned_cost, my_value, my_value_source}; all-empty deletes the row
       interest = +1 target, -1 avoid (null = no opinion; 0 rejected); custom_tier overrides the algorithm's tier (1-20, null = computed); planned_cost = null means "not in the team plan"
  GET  /api/leagues/{id}/draft-values?season=&budget=200&teams=12&format=league|ppr|half|standard&slots=QB:1,RB:2,WR:3,TE:1,FLEX:1,SFLEX:1,K:1,DEF:1,BN:6&scoring=pass_yds:0.04,pass_td:4,...
       — every scoring-dependent setting is overridable; omitted ones fall back to the league's real settings. Response echoes what was used in `settings`. `scoring`, present, replaces `format` entirely and prices every position (not just kickers) from those exact weights.
  GET  /api/leagues/{id}/feed                              — activity rail: pinned thread + recent posts/derived trade+scored-week events
  GET  /api/leagues/{id}/threads?filter=all|unread|mentions|mine&offset=&limit=
  POST /api/leagues/{id}/threads                            — {title?, body, image_url?, attached_gsis_id?, poll_options?, poll_closes_at?, poll_votes_visible?}
  GET  /api/leagues/{id}/threads/{threadId}                 — thread + replies; marks it read as a side effect
  POST /api/leagues/{id}/threads/{threadId}/replies         — {body, image_url?, attached_gsis_id?} (no polls on replies)
  POST /api/leagues/{id}/threads/{threadId}/read             — mark read without opening
  PUT  /api/leagues/{id}/threads/{threadId}/pin              — commissioner only; {pinned}; at most one pinned thread per league
  PUT  /api/leagues/{id}/posts/{postId}                      — author or commissioner; {body}
  DELETE /api/leagues/{id}/posts/{postId}                    — author or commissioner; soft delete
  POST /api/leagues/{id}/posts/{postId}/react                — toggles the caller's "+1"
  POST /api/leagues/{id}/posts/{postId}/vote                 — {option_id}; one vote per poll, revote overwrites
  GET  /api/teams/{id}/roster?stat_type=lastweek|season|today  (or ?week=N)
  GET  /api/teams/{id}/keepers
  POST /api/teams/{id}/keepers/{playerKey}
  DELETE /api/teams/{id}/keepers/{playerKey}
  POST/DELETE /api/teams/{id}/keepers/submit
```

## Commands

### Backend (`cd backend`)

```bash
make run           # go run ./cmd/api
make build         # go build -o bin/api ./cmd/api
make test          # go test ./...
make migrate-up    # run migrations up (requires DATABASE_URL env)
make migrate-down  # roll back one migration
make generate      # sqlc generate (regenerates internal/db/queries/)
make import-nfl    # import nflverse data (default: 2020-2024)
make import-nfl ARGS="-from 2015 -to 2024"       # custom year range
make import-nfl ARGS="-rosters-only"              # just player metadata
make import-nfl ARGS="-stats-only"                # just weekly stats
make project-nfl ARGS="-profiles"                # build player season profiles (run after import)
make project-nfl ARGS="-project -season 2025"    # compute comp-based projections
make project-nfl ARGS="-grades"                  # compute real-life player grades (0-100)
make project-nfl ARGS="-all -season 2025"        # profiles + grades + projections
make backtest-nfl ARGS="-from 2015 -to 2024"     # backtest projections across historical seasons
make project-nfl ARGS="-cohort-bias -from 2015 -to 2024 -min-base-ppg 8"   # SIGNED bias by cohort + projection level
make autotune-nfl ARGS="-from 2015 -to 2024 -train-to 2021"  # auto-tune weights, saves projection_config.json
make project-nfl ARGS="-import-consensus f.json -season 2026" # load external rankings/ADP (see docs/stats/consensus-ensemble.md)
make project-nfl ARGS="-import-notes f.json -season 2026"     # load situational news (injuries, camp battles, etc.)
make project-nfl ARGS="-consensus-diff -season 2026 -format ppr" # diff our projection rank vs consensus (non-mutating)
```

Run a single test:
```bash
go test ./internal/handlers/... -run TestHealthHandler
```

### Frontend (`cd frontend`)

```bash
yarn dev           # Vite dev server on :5173 (proxies /api and /auth to :8080)
yarn build         # tsc + vite build → dist/
yarn lint          # eslint
```

### Full stack (Docker)

```bash
cp .env.example .env
docker compose up          # starts db + backend + frontend
docker compose up db       # just PostgreSQL on :5432
```

## Environment

Copy `.env.example` → `.env`. Required:
- `DATABASE_URL` — e.g. `postgres://fantasy:fantasy@localhost:5432/fantasy_sports`
- `PORT` — defaults to `8080`
- `YAHOO_CLIENT_ID`, `YAHOO_CLIENT_SECRET`, `YAHOO_REDIRECT_URL`
- `SESSION_SECRET` — random 32+ char string

Optional (development only):
- `YAHOO_MOCK=1` — serve synthetic Yahoo data instead of calling the real API, and expose `/auth/mock-login` (a session with no authentication). See "Mock Yahoo mode" below. **Never set in a deployed environment.**

## Key Patterns

- **Design system:** frontend follows `~/Downloads/design_handoff_fantasy_sports_app` — warm near-black neutrals, **one** coral-pink accent for all positive/primary meaning, cool blue for negative/secondary, purple **reserved strictly for projected/future data** (never actual results). Three fonts, one job each: `font-display` (Space Grotesk) headings/nav/buttons/table labels, `font-sans` (Inter) body, `font-mono` (IBM Plex Mono) every number. No gradients, shadows, transitions, animations (loading spinners excepted), emoji, or icon set in table headers — direction is a typographic glyph (▲ ▼ — ↑ →). Tokens live in `src/index.css` as HSL triplets via `hsl(var(--x))`; **never** raw Tailwind palette classes. Dark is the designed theme; light is a derived inversion. Migration plan: `.claude/plans/design-system-migration.md`
- **Handler pattern:** all handlers are methods on `*Handler`; use `r.Context().Value(models.UserContextKey)` for current user in protected routes
- **Yahoo client:** instantiate per-request via `yahoo.NewClient(ctx, db, oauthConfig, userID, accessToken, refreshToken, expiry)`; tokens auto-refresh and persist to DB
- **Mock Yahoo mode (`YAHOO_MOCK=1`, dev only):** Yahoo's API has rejected this app since ~March 2026 (`403 This application is not authorized`, an app-registration problem outside this repo), blocking every league route *and* login. Mock mode swaps the `http.RoundTripper` inside `yahoo.NewMockClient()` (`internal/yahoo/mock.go`) rather than stubbing individual client methods, so the real `decode()` path still runs against fixtures marshalled from Go structs (`internal/yahoo/mockdata.go`) — they can't drift from `types.go`. A deterministic fixed-seed 12-team snake draft over a static player pool (`internal/yahoo/mockplayers.go`); package `yahoo` has no DB dependency, so mock mode works against an empty database. `/auth/mock-login` is registered only when the flag is set; `/auth/login` redirects to it. **Applies to every league, not just the mock one** — `newYahooClient` returns the mock client unconditionally under the flag, so a real previously-synced league hits the same fixtures for anything re-fetched from Yahoo.
- **Stat type:** GetTeamRoster passes statType directly to Yahoo's semicolon-path syntax (`type=week`, `type=lastweek`, `type=date;date=YYYY-MM-DD`, etc.)
- **Concurrent fetching:** buffered channels fan out Yahoo API calls in GetTeamRoster, GetLeagueDraftResults, GetLeagueRankings
- **Dynamic stat columns:** frontend derives column headers from `roster[].stats[].label` — no hardcoded stat IDs
- **sort_order:** Yahoo's `sort_order` ("1" = higher is better) is passed through for correct winner determination in MatchupDetail
- **Player rankings:** `analysis.go` (Yahoo-only) concurrently fetches rosters, scoring categories, top-25 FA stats. Category weights = `CV × scarcity` (normalised): CV = stdev/|mean|; scarcity = `1/(1 + max(0, avgFAz))`. `overall_score` = weighted z-sum; `position_score` = z-score within position group. See `docs/ranking-algorithm.md`
- **nflverse data import:** `cmd/import/` downloads roster + player_stats CSVs from nflverse GitHub releases, idempotent upserts into `nfl_players`/`nfl_player_stats`. Default range 2020–2024, data available from 1999. `nfl_players.team` is normalized through `teamAliases` for franchises whose abbreviation varies across import vintages without the team relocating (ARI/AZ/ARZ, BAL/BLT, CLE/CLV, HOU/HST) — deliberately excludes real relocation history (SD→LAC, OAK→LV, STL→LA).
- **Comp-based projections:** `cmd/projections/main.go`. Step 1 (`-profiles`) aggregates `nfl_player_stats` into SOS-adjusted, Bayesian-shrunk (`shrinkage.go`), z-scored per-player-season profiles. Step 2 (`-project`) blends the target's base season with the immediately preceding one (`recency_blend.go`), finds similar historical players (weighted Euclidean distance over dimension groups: passing/rushing/receiving/value/physical/context/grade), applies their post-match growth (weighted by similarity²), shrunk toward an age/position baseline when comps are scarce. Comps that washed out contribute zero growth (survivorship guard); zero-comp players regress halfway to position mean. Output includes an outcome distribution (`_stdev/_p10/_p50/_p90`). Draft capital only used for players with < 3 years experience. See `docs/projection-algorithm.md` + `docs/algorithm-review.md` + `docs/stats/`.
  - **Short-season shrinkage** (`short_season_shrinkage.go`): a base season with no prior season to blend against (e.g. an injury-shortened rookie debut) regresses its production fields (fpts, yards, TDs, receptions) and z-scores toward the position-group mean, weighted by `games_played/17`; usage/opportunity counts stay untouched. Full seasons are a no-op.
  - **Rookie projections** (`rookies.go`): incoming rookies have no profile to grow, so `computeRookieProjections` comps them against historical rookies at the same position via a Gaussian kernel on draft slot (σ=40 picks; UDFA pinned past the last pick), using comps' own rookie-season output directly. `ConfDataQuality` pinned to 0. Requires the current season's roster already imported for `entry_year`/`draft_number`.
  - **Consensus divergence** (diagnostic, non-mutating): `cmd/projections/consensus.go` imports external rankings/ADP + situational notes from curated JSON, computes gap vs. **median** external rank/ADP **within position group** into `nfl_projection_divergences`. Only standard/half_ppr/ppr are diffable. See `docs/stats/consensus-ensemble.md` and `docs/algorithm-review.md` §6.
  - **Calibration:** `cmd/projections/calibration.go` scales projected quantities by a per-position factor (`projection_config.json`), currently seeded **uniform at 0.884**. A uniform factor cancels out of VOR/auction math, so it only scales displayed points, not ranks/prices. See `docs/algorithm-review.md` §7.8.
  - **Cohort bias** (`-cohort-bias`): reports signed projection bias by base-season trend + projected level (unlike backtest's unsigned RMSE/MAE). `TargetBlendDecayUp` stays at 0 — the measured bias is a uniform over-projection, not a breakout-regression artifact. See `docs/algorithm-review.md` §7.7.
  - **Projection config:** `cmd/projections/backtest.go` defines `projConfig`, read from `projection_config.json` if present. `-autotune` does coordinate ascent maximizing per-game Spearman rank correlation, keeping the tuned config only if it beats defaults on held-out seasons. `-backtest` stores metrics in `nfl_backtest_results` (`eval_basis` 'total'/'per_game' + quantile calibration coverage).
- **Player detail:** `GET /api/nfl/players/{gsisId}` returns metadata + YoY stats + projection + `notes`. Player rows app-wide are clickable → `/players/:gsisId`. Every list-shaped endpoint that returns `gsis_id` batch-attaches notes via the shared `loadNotesForPlayers` helper. The notes button (`PlayerCell`, `table-helpers.tsx`) highlights when a note is "new" (`reported_date`/`created_at` within 3 days). **Scope cut:** desktop `PlayerCell` only — `MobileStatCard` doesn't show notes yet.
- **Draft tab:** `DraftSection.tsx` groups Draft Values (NFL only) and Keepers under `?sub=`. Draft Values (`GET /api/leagues/{id}/draft-values`) reads whatever settings were saved on `/draft-prep` but can't edit them. Target season is `league.season + 1` clamped to `PROJECTION_SEASON`.
- **Auction pricing** (`draft_values.go`): dollar values are VOR's share of the budget *surplus* (`teams × budget` minus $1 reserved per roster spot), matching standard VBD convention. VOR is raised to the **0.75 power** (`auctionVORCompressionExponent`) before sharing — linear VOR blew the top of the board past real-world auction ceilings in a single-QB/few-flex league. Empirically tuned, not per-league. Flows through to `consensus_auction_value` too. See `docs/stats/auction-values.md`.
- **Draft Prep** (`/draft-prep`): the single editable draft board; the league Draft tab reads the same board read-only. Personal board (`draft_prep_players`, per user/league/season): target(+1)/avoid(−1) interest, custom rank, tier override (`custom_tier`, 1–20, independent of the algorithm's own tier count), note, planned cost, my value (see below). `NULL` = no opinion, `0` = rejected (the only two states). A row with no interest/rank/note/cost/value carries no information and is deleted. Reordering rewrites the whole board in one `PUT .../order` (rank is relative, so a partial write would be inconsistent). Consensus columns (`Cons $`, `Edge`) are prep-page only; uncovered players show `—`, never `$0`. Plan panel (`TeamPanel`/`TargetList`) has Team mode (roster your planned prices assemble into) and Targets mode (grouped by position → tier), docked to the right edge on `lg`+, persisted open/closed in `localStorage`. Assignment is greedy by projected points, filling the most restrictive slots first (not a global optimum, but predictable). Bench (`BN`) is carried for roster-size accounting but starts nobody.
- **Board / Tiers layout toggle** (`/draft-prep` and the league Draft tab, `?layout=board|tiers`): `DraftBoardTable` (the sortable table, ranks/interest/plan editable via `PrepControls`) or `TiersView` (players bucketed by position, then by tier within it — one panel per position, tier sections inside — since tier numbers are only comparable within a position, see `docs/stats/tiering.md`). Position and target/avoid filters (plus a name search, `boardFiltered` in `index.tsx`) apply to Board only; Tiers always shows everyone, since it already organizes by position and a position filter there would just collapse it to one panel. Tier reassignment in `TiersView` has two equivalent paths — drag a player onto a different tier's panel section (cross-position drops are refused), or a picker dialog listing every tier the algorithm has actually produced (plus one, to open a new bottom tier) — both funnel through one `moveToTier` that also drives the my-value auto-fill below. `TiersView`'s own ▲/▼ (next to the My value field) don't touch tier at all — they nudge that player's `my_value` by $1, based off their own value if set or the system's otherwise. Below `md`, `MobileTiersView`'s markup (same file) replaces the desktop drag-and-drop grid — a `MobileStatCard` per player with a "Mine" face button and, expanded, "Set tier ▾" (opens the same `TierPickerDialog`, a `ResponsiveDialog`) and "Edit value" (a `MobileSheet` numeric entry), both routing through the same `moveToTier`/`setMyValue` the desktop row uses.
- **Board columns** (`DraftBoardTable.tsx`): a fixed default set (Board, Player, Interest, Plan, Pos, Auction $, Grade, Trend) plus a `Columns ▾` toggle over the rest (`#`, Note, Age, VOR, Proj Pts, Pts/G, Cons $, Edge, Confidence, Profile), persisted to `localStorage` (`fs.draft-prep.columns`) and mirrored into `MobileDraftBoard`'s expansion panel and sort-sheet options so the two never disagree. Hiding the active sort column falls back to `board` (or `rank` without `prep`). The Board rank cell shows your custom rank plus a muted `▲n`/`▼n` delta against the projection's own rank when they diverge. When the table isn't in board order, a line under the header ("Sorted by X — back to board order to reorder") explains why the drag handle/nudge arrows disappeared, with a one-click link back to board order; the arrows themselves are keyboard-focus-only (`focus-within:opacity-100`), not always-on, alongside the always-on `⠿` drag handle.
- **My value vs. system value:** `draft_prep_players.my_value` is the user's own valuation, independent of the algorithm's `auction_value` — shown alongside it in `TiersView` (editable inline; the main board doesn't surface it). **A tier's members are ordered by effective value, not projected points** — your own value where you've set one, falling back to the system's, points breaking ties (read-only, no `prep`, has no "my value" concept at all so it's just the system's price). The tier-move interpolation below deliberately still reasons in points internally, independent of this display order — re-sorting its own local copy of every tier by points rather than trusting whatever order the caller's tiers happen to be in — since points, not the value being computed, is what should place a mover among tier-mates. Moving a player — on the Board (drag/▲▼/mobile move-sheet, all funnel through `handleMove`) or in Tiers (`moveToTier`) — auto-fills `my_value` to the average of whichever players now flank it (falling back to a neighbour's `auction_value` when that neighbour has no `my_value` of its own yet; a single neighbour's value is used as-is). On the Board the neighbours are whoever ends up adjacent in the personal rank order. In Tiers, since a tier bucket itself has no manual order, the neighbours are found by where the mover's own projected points would rank among the target tier's *other* members (already points-sorted, and excluded from every tier's member list up front — not just the target tier — so a tier the mover is the sole occupant of doesn't reference the mover's own pre-move value once vacated) — falling through to the boundary player of the nearest real tier with members on either side when the mover lands at the very top/bottom of its tier, including a brand-new tier opened past the previous last one. Only fires when the tier actually changes, not on a no-op reassignment (e.g. picking "Match algorithm" when nothing moved). **`draft_prep_players.my_value_source`** (`'user' | 'derived' | null`, null whenever `my_value` is null) distinguishes a hand-typed value from one this auto-fill produced — `setMyValue` (a direct field edit) always writes `'user'`; the board-move/tier-move auto-fill writes `'derived'` and, critically, **never runs at all when the current value's source is already `'user'`** (`handleMove` in `index.tsx`, `moveToTier` in `TiersView.tsx`), so moving a player never silently overwrites a price you typed yourself. A `'derived'` value renders dimmed with a dotted underline (`MyValueField`/`MobileTierMemberCard`) as a visual "this is a suggestion, edit to keep it fixed" cue. **Both writes (rank/tier + value) go out in one `setFields` call, never two independent `patch`es** — `useDraftPrep`'s `patch` builds a whole-row upsert from the current cached snapshot, and two sequential patches within the same synchronous handler both read that *same* pre-mutation snapshot (nothing has re-rendered between them), so the second call's stale copy of the field the first call just changed would silently revert it once both requests land. Board reordering goes through the separate bulk `/order` endpoint, so its `setFields` call explicitly passes the just-computed new rank rather than trusting a fresh read there either.
- **Clearing interest in a filtered view:** in the Targets/Avoids view, re-clicking the active thumb clears it — normally dropping the row out of the filtered list instantly, with no way back. `index.tsx` intercepts this (`clearingInterest`, wrapping `prep.setInterest` as `PrepControls.setInterest`): a just-cleared row is kept in `boardFiltered`/`MobileDraftBoard`'s list for that render pass (tracked in a `recentlyCleared` `Map<gsisId, previousLevel>` state, reset on any position/view filter change) and rendered muted with a struck-through name and an inline "Undo" control (`onUndoInterest`) instead of vanishing. The `all` view is unaffected, since nothing there ever disappears on an interest change.
- **Board print sheet:** the Print button (`/draft-prep`, both layouts) renders a `hidden print:grid` six-field sheet (rank, player, pos, my value, planned cost, note) in board order, capped to `printPoolSize` — same cap convention as `TiersView`'s own print sheet, not the interactive table.
- **First-run primer:** `/draft-prep` shows one dismissible row above the Board ("Flag targets with △▽, plan a price with +, drag to set your own order") when a league's board has no ranks/targets/planned players yet; dismissed permanently (`localStorage` `fs.draft-prep.primer-dismissed`) on the ✕ or automatically on the first real interest/plan/reorder write.
- **Draft settings panel** (`/draft-prep` only): `DraftSettingsPanel`/`useDraftSettings.ts` edit teams/budget/format/slots/scoring, sent as `draft-values` query overrides — no second scoring implementation on the frontend. Editing any point value sets `scoringCustomized: true` (the only thing that puts `scoring=` on the request, replacing `format` entirely — every position prices off those exact weights, not just kickers). Edits are held as an unsaved draft until `save()`; persistence is `localStorage` only (`fs.draft.settings.v1.{leagueId}`), nothing server-side. Flex shapes translate between Yahoo's eligibility-list form and the UI's flat slot names (`yahooToSlot`/`slotToYahoo`); `ranking.ComputeStarterSlots`'s even split exists only to check that round-trip, not to determine price. **Flex/superflex replacement is pooled, not split evenly** (`ranking.ComputeReplacementLevels`): dedicated slots claim starters directly; each flex-type slot pools its still-unclaimed candidates by value and lets the best players claim spots greedily, position-blind — this is what lets SFLEX correctly price QB near a full starter slot instead of an even 1/4 split. Lives in the docked side panel's Settings bucket, alongside Plan — both share one panel, switched by a bucket toggle, rather than two separate docks.
- **My Team tab** (Yahoo only): league page's first tab, rendered only when the signed-in user owns a team in that league; reuses `team-detail`'s `TeamPanel`. `/leagues/{id}/my-team` redirects to the tab.
- **Player Grades:** three-layer separation — Player Grade (real-life quality, 0–100 percentile, `cmd/projections/grades.go`, position-specific sub-score weights), Stat Projections (comp-based), Fantasy League Value (VORP/z-scores). `overall_grade_z` feeds into comp-matching similarity (weight 1.25); grade YoY trend applies a bounded ±5% adjustment to projected stats.
- **Yahoo decoupling:** Yahoo supplies fantasy *context* (ownership, scoring, roster slots, FA status), not NFL *stats*. NFL rankings pull raw stats from `nfl_player_stats` via `services/nflstats`, translating stat IDs through `services/scoring`. Yahoo-only fallbacks remain for non-season stat types, NBA, and live in-season keeper/roster views. See `.claude/plans/yahoo-decoupling.md`.
- **Native leagues:** a league with no Yahoo league behind it — `leagues.source = 'native'`, `leagues.format` selects the rollover strategy. `POST /api/leagues` creates one (settings + teams, one transaction), owned by `leagues.user_id`. `services/leaguesettings.Source` is the seam that lets `draft_values.go` (and everything downstream) work identically for Yahoo or native leagues. No `DEF` slot anywhere (no team-defense `gsis_id`s). Rosters/contracts live in `league_rosters`/`league_contracts`; future picks/trades in `league_draft_picks`/`league_transactions`; weekly play in `league_matchups`; salary cap in `league_team_seasons`/`league_dead_money`; free agency in `league_fa_windows`/`league_fa_offers`; message board in `league_posts` + related tables. Every mutation (assign/drop/trade/pick-use/rollover/sign) logs a `league_transactions` row, which backs the Roster tab's activity log and the message board's derived feed events. Team ownership is opt-in claiming (`PUT .../teams/{teamId}` `{claim: true}`, one team per user per league) rather than Yahoo-style sync-time assignment. There is no separate "team page" for a native team — every team-name link resolves into the league's Roster tab with `?team={id}`, since there's no reader who isn't the commissioner in this single-user-per-league model.
  - **Explicit, deliberate scope cuts (not gaps to silently fill):** regular season only, no playoff bracket/seeding (`league_matchups.is_playoffs` reserved); scoring is a manual per-week commissioner action, never live; no native keeper-designation flow (`rollover.go`'s `keeper` format 501s); `analysis.go` rankings and `league_players.go` search are Yahoo-only, not yet ported to native; salary retention in trades is not implemented (the pressure valve that would make the full-dead-cap rule survivable long-term — see `.claude/plans/dynasty-transactions.md`'s "Open/deferred").
  - Full phased build history and remaining open decisions: `.claude/plans/native-leagues.md` (league creation, rosters, weekly play, message board) and `.claude/plans/dynasty-transactions.md` (salary cap, rookie scale, free agency, rollover integration — all 8 phases of that plan are shipped).
- **SQL approach:** handlers use raw pgx queries for all database access — this is the established pattern, do NOT introduce sqlc or an ORM. `sqlc.yaml` exists but is unused; raw queries are preferred for directness and pgx features (e.g. `ANY($1)` with slices). Keep queries in handler methods, not a separate query layer.
- **New resource checklist:** model → migration → handler → route → yahoo method (if needed) → TS interface → API function → query key → page → **update docs**
