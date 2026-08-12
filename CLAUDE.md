# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

> **Keep this file and `memory/project_fantasy_sports.md` up to date** whenever you add routes, handlers, pages, schema changes, or new patterns. Accurate docs prevent wasted effort re-reading the codebase.

## Project Overview

Full-stack fantasy sports web app (multi-sport: NFL, NBA). Go backend + React/Vite/TypeScript frontend + PostgreSQL. Users authenticate via Yahoo OAuth, sync their Yahoo fantasy leagues, and view live rosters, scoreboards, standings, and matchup details.

## Architecture

```
backend/   Go API server (Chi router, pgx/pgxpool, golang-migrate, gorilla/sessions, Yahoo OAuth2)
frontend/  React 18 + Vite 5 + TypeScript + Tailwind CSS + shadcn/ui + TanStack Query v5
```

**Backend layout:**
- `cmd/api/main.go` — entry point; wires router, DB pool, sessions, oauth config, handlers
- `cmd/import/main.go` — CLI tool to download nflverse CSV data and upsert into `nfl_players` / `nfl_player_stats`
- `internal/handlers/` — one file per resource; all handlers are methods on `Handler{db, sessions, oauthConfig, config}`
  - `handlers.go` — Handler struct definition + constructor
  - `respond.go` — JSON response helpers
  - `auth.go` — Login, Callback, Me, Logout
  - `leagues.go` — ListLeagues, CreateLeague, GetLeague
  - `teams.go` — ListLeagueTeams, GetTeam, GetTeamRoster (with stat period support); includes gsis_id batch lookup for player detail links
  - `players.go` — ListPlayers, CreatePlayer, GetPlayer
  - `scoring.go` — GetLeagueScoreboard, GetLeagueStandings
  - `yahoo_helpers.go` — `leagueYahooKey()`, `userTokens()`, `newYahooClient()` shared helpers used by multiple handlers
  - `sync.go` — Sync (Yahoo league+team upsert)
  - `league_players.go` — SearchLeaguePlayers, GetAvailablePlayers
  - `keepers.go` — GetKeeperRules, UpdateKeeperRules, GetLeagueDraftResults, GetLeagueKeepers, GetKeeperSummary, ListTeamKeeperWishlist, AddKeeperWishlist, RemoveKeeperWishlist, SubmitKeepers, UnsubmitKeepers
  - `analysis.go` — GetLeagueRankings: weighted z-score rankings; category weights = CV × FA-scarcity (normalised); adds `position_score` (z-score within position group); includes gsis_id lookup for player detail links. NFL season stats source locally from `nfl_player_stats` via `services/nflstats` (Yahoo only returns ownership + league config); non-season stat types + NBA fall back to Yahoo stats
  - `projections.go` — ListProjections, GetProjectionDetail; serves pre-computed comp-based NFL player projections from nfl_projections table
  - `rankings_public.go` — ListPublicRankings: no-auth projection-based rankings with PPR/Half/Standard format toggle (GET /api/rankings)
  - `nfl_players.go` — GetNFLPlayer (full player detail: metadata + YoY stats + projection); GetNFLPlayerByYahooID (resolves Yahoo key → gsis_id and redirects)
  - `draft_values.go` — GetDraftValues: league-specific auction values (VOR + $ value based on actual roster settings); uses `services/scoring` for canonical stat-ID translation and league-aware kicker scoring
  - `grades.go` — ListGrades, GetPlayerGrades: real-life player grades (0-100 percentile) from nfl_player_grades table; supports comma-separated position filter (e.g. `?position=RB,WR,TE`)
  - `draft_prep.go` — GetDraftPrep, UpsertDraftPrepPlayer, ReorderDraftPrep: the personal draft board (tags, custom ranks, notes) in `draft_prep_players`, scoped to (user, league, season)
  - `draft_consensus.go` — `loadConsensusValues`: consensus auction value per player, on the league's dollar scale. Prefers imported `metric_type='auction'` rows (rescaled from a 12-team/$200 market pool); otherwise **derives** it by reading our own value curve at the market's median within-position rank — "what our board pays for the slot the market gives him". Same median/within-position conventions as `consensus.go`. See `docs/stats/auction-values.md`
  - `divergences.go` — ListDivergences: projection-vs-consensus rank gaps from `nfl_projection_divergences`, ordered by |delta|, each with situational notes. `loadNotesForPlayers` (shared with `nfl_players.go`) attaches both player-scoped notes and team-scoped ones matched via `nfl_players.team`
- `internal/models/models.go` — shared domain types (User, League, Team, Player, RosterEntry)
- `internal/middleware/auth.go` — RequireAuth: reads session, attaches *models.User to ctx
- `internal/yahoo/` — Yahoo Fantasy API client, OAuth config, XML types
  - `client.go` — all API methods + dbTokenSource (auto-refreshes + persists tokens)
  - `oauth.go` — Yahoo OAuth2 endpoint + NewOAuthConfig
  - `types.go` — all XML response structs
- `internal/services/scoring/` — canonical stat-ID vocabulary + Yahoo-stat-ID translation + projection→canonical-total helpers; the single place stat-ID knowledge lives
- `internal/services/nflstats/` — season-level aggregation of `nfl_player_stats` keyed by gsis_id; replaces Yahoo as the NFL stats source for rankings
- `internal/db/db.go` — pgxpool connect helper
- `migrations/` — numbered SQL migration files

**Frontend layout:**
- `src/api/client.ts` — all typed API functions + TypeScript interfaces
- `src/api/queryKeys.ts` — all TanStack Query cache keys
- `src/lib/queryClient.ts` — QueryClient config (staleTime: 30s, retry: 1)
- `src/lib/utils.ts` — cn() utility (clsx + tailwind-merge), zScoreIndicator, zScoreColor
- `src/lib/grades.ts` — shared grade display utilities: gradeColorClass, trendIndicator, phaseLabel, phaseColor
- `src/lib/constants.ts` — CURRENT_SEASON (2025), PROJECTION_SEASON (2026)
- `src/pages/` — pages by route; complex pages split into subdirectories:
  - `Home.tsx` — the Leagues home: hero + your-leagues list + "Player Outlooks" signal cards (top consensus divergences with All moves / We're higher / We're lower / Has news chips)
  - `league-detail/` — `index.tsx` + tab components (MyTeamTab, StandingsTab, ScoreboardTab, PlayersTab, DraftSection [wraps DraftTab + KeepersTab as sub-sections]) + `MyTeamRedirect.tsx` + hooks
  - `team-detail/` — `index.tsx` + `components/` (TeamPanel, RosterTable, MatchupCard) + `hooks/useTeamDetail.ts`. `TeamPanel` (matchup card + roster + period switcher) is shared with the league page's My Team tab; `index.tsx` is just the page header around it
  - `matchup-detail/` — `index.tsx` + `components/` (CategoryTotalsTable, TeamRosterTable) + hook
  - `player-detail/` — `index.tsx` + `components/GradeCard.tsx` — unified NFL player detail page (metadata, grade card, YoY stats table, projection with PPR/Half/Standard toggle + comps)
  - `projection-detail/` — `index.tsx` + `components/` (CompCard, TrajectoryChart) + hook — legacy detail page (redirects to player-detail)
  - `statistics/` — `index.tsx` + `components/GradesTable.tsx` — the single player-data surface at `/statistics`, with a Projections/Grades view toggle (`?view=` drives it). Reuses `projections/hooks` + `projections/components` and `rankings/hooks`. Projections are fixed to `PROJECTION_SEASON`; the Year control only appears in Grades view, since no season has both
  - `rankings/hooks/useRankings.ts` — grades query, consumed by `statistics/` (the standalone page was absorbed)
  - `projections/` — `hooks/useProjections.ts` + `components/` (ProjectionTable, ConfidenceBadge, UniquenessBadge), consumed by `statistics/`. ProjectionTable shows Consensus/Δ columns (partial coverage, "—" where no consensus data exists)
  - `draft-prep/` — `index.tsx` (top-level `/draft-prep`) + `components/` (DraftBoardTable, Shortlist) + `hooks/useDraftPrep.ts`. **`DraftBoardTable` is the shared draft board**, used here with prep controls and by league-detail's DraftTab without them
  - `divergences/` — `index.tsx` + `hooks/useDivergences.ts` + `components/DeltaBadge.tsx` — the full divergence table at `/divergences`. No longer in the nav: Home surfaces the top signals and links here for the rest. Single-source divergences are flagged (`1*`) since they're uncorroborated
- `src/components/ui/` — shadcn/ui components (badge, button, input, table, tabs, provider) + table-helpers (SortableHead, PlayerCell, ClickableRow, ZScoreCell, HeaderRow)
- `src/App.tsx` — router + nav (with active page highlighting) + auth check
- `src/main.tsx` — React root, QueryClientProvider, BrowserRouter
- Vite proxies `/api` and `/auth` → `http://localhost:8080` in dev
- HTTPS via `vite-plugin-basic-ssl` (required for Yahoo OAuth)

## Routes

Frontend (nav has three destinations: Leagues, Draft Prep and Statistics):
```
/                                       Leagues home — hero, league list, Player Outlooks
/draft-prep                             Draft Prep — league settings editor + personal board (ranks, tags, notes)
/leagues                                → redirect to /
/leagues/{id}                           League detail (My Team/Standings/Scoreboard/Players/Draft)
/leagues/{id}?tab=draft&sub=values|keepers  Draft tab sub-sections (?tab=keepers redirects here)
/leagues/{id}/my-team                   → redirect to /leagues/{id}?tab=my-team
/leagues/{leagueId}/matchup/{week}/{t1}/{t2}
/teams/{id}                             Team roster
/statistics?view=projections|grades     Player data — projections + real-life grades
/rankings                               → redirect to /statistics?view=grades
/projections                            → redirect to /statistics?view=projections
/divergences                            Full consensus-divergence table (linked from Home)
/players/{gsisId}                       Player detail
/projections/{gsisId}                   → redirect to /players/{gsisId}
```

API:
```
Public:
  GET  /auth/login, /auth/callback, /auth/logout
  GET  /api/health
  GET/POST /api/leagues, GET /api/leagues/{id}
  GET  /api/leagues/{id}/teams
  GET  /api/teams/{id}
  GET/POST /api/players, GET /api/players/{id}
  GET  /api/projections?season=&position=&sort=&limit=&offset=
  GET  /api/projections/{gsisId}?season=
  GET  /api/nfl/players/{gsisId}              — full player detail (metadata + YoY stats + projection)
  GET  /api/nfl/players/by-yahoo/{yahooKey}   — resolves Yahoo key → HTTP redirect to /api/nfl/players/{gsisId}
  GET  /api/grades?season=&position=&limit=&offset= — real-life player grades; position supports comma-separated (e.g. RB,WR,TE)
  GET  /api/grades/{gsisId}                  — all seasons of grades for a player
  GET  /api/rankings?season=&format=ppr|half|standard&position=&limit=&offset= — projection-based public rankings (no Yahoo, no login)
  GET  /api/divergences?season=&format=ppr|half_ppr|standard&position=&limit=&offset= — projection vs consensus gaps + situational notes

Protected (RequireAuth):
  GET  /api/auth/me
  POST /api/sync
  GET  /api/leagues/{id}/scoreboard?week=N
  GET  /api/leagues/{id}/standings
  GET  /api/leagues/{id}/players?search=q
  GET  /api/leagues/{id}/players/available?position=&start=&status=
  GET  /api/leagues/{id}/draftresults
  GET  /api/leagues/{id}/keepers
  GET  /api/leagues/{id}/keeper-rules
  PUT  /api/leagues/{id}/keeper-rules
  GET  /api/leagues/{id}/keeper-summary
  GET  /api/leagues/{id}/rankings?stat_type=season
  GET  /api/leagues/{id}/draft-prep?season=              — your personal board: tags, custom ranks, notes
  PUT  /api/leagues/{id}/draft-prep/order?season=        — {"gsis_ids":[...]}; array position becomes the rank
  PUT  /api/leagues/{id}/draft-prep/{gsisId}?season=     — {tag, custom_rank, note}; all-empty deletes the row
  GET  /api/leagues/{id}/draft-values?season=&budget=200&teams=12&format=league|ppr|half|standard&slots=QB:1,RB:2,WR:3,TE:1,FLEX:1,SFLEX:1,K:1,DEF:1
       — every setting the scoring depends on is overridable; omitted ones fall back to the league's real Yahoo settings.
         The response echoes what was used in `settings` (same vocabulary), so clients never reverse-engineer roster slots.
  GET  /api/teams/{id}/roster?stat_type=lastweek|season|today  (or ?week=N)
  GET  /api/teams/{id}/keepers
  POST /api/teams/{id}/keepers/{playerKey}
  DELETE /api/teams/{id}/keepers/{playerKey}
  POST /api/teams/{id}/keepers/submit
  DELETE /api/teams/{id}/keepers/submit
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
- `YAHOO_MOCK=1` — serve synthetic Yahoo data instead of calling the real API, and expose `/auth/mock-login` (a session with no authentication). See the Mock Yahoo mode pattern below. **Never set in a deployed environment.**

## Key Patterns

- **Design system:** the frontend follows `~/Downloads/design_handoff_fantasy_sports_app` — warm near-black neutrals, **one** coral-pink accent for all positive/primary meaning, cool blue for negative/secondary, and purple **reserved strictly for projected/future data** (never use it on actual results). Three fonts, one job each: `font-display` (Space Grotesk) for headings/nav/buttons/table column labels, `font-sans` (Inter) for body, `font-mono` (IBM Plex Mono) for **every** number. No gradients, no shadows, no transitions, no animations (loading spinners excepted), no emoji, no icon set in table headers — direction is a typographic glyph (▲ ▼ — ↑ →). Tokens live in `src/index.css` as HSL triplets consumed via `hsl(var(--x))`; **never** use raw Tailwind palette classes (`text-red-600`, `bg-green-100`) — every color goes through a token. Dark is the designed theme; light is a derived inversion. Migration plan and remaining phases: `.claude/plans/design-system-migration.md`
- **Handler pattern:** all handlers are methods on `*Handler`; use `r.Context().Value(models.UserContextKey)` for current user in protected routes
- **Yahoo client:** instantiate per-request via `yahoo.NewClient(ctx, db, oauthConfig, userID, accessToken, refreshToken, expiry)`; tokens auto-refresh and persist to DB
- **Mock Yahoo mode (`YAHOO_MOCK=1`, dev only):** Yahoo's API has rejected this app since ~March 2026 (`403 This application is not authorized` — an app-registration problem outside this repo), which blocks every league route *and* login itself, since `Callback` resolves the user GUID via the Fantasy API. Mock mode swaps the `http.RoundTripper` inside `yahoo.NewMockClient()` (`internal/yahoo/mock.go`) rather than stubbing the 18 client methods — one interception point covers every endpoint, and the real `decode()` path still runs, so the fixtures are continuously validated against the production XML types. Fixtures are Go structs marshalled with `xml.Marshal` (`internal/yahoo/mockdata.go`), never hand-written XML, so they can't drift from `types.go`. The league is a deterministic fixed-seed 12-team snake draft over a committed static player pool (`internal/yahoo/mockplayers.go`, generated from the top ~200 by 2026 projected PPR) — package `yahoo` deliberately has no DB dependency, so mock mode works against an empty database. `/auth/mock-login` is registered **only** when the flag is set (not merely guarded inside the handler) and re-checks the flag; `/auth/login` redirects to it so the existing UI button works with no frontend change. The mock league seeds itself into `leagues`/`teams` through the normal `POST /api/sync` path — no migration, no fixture rows in schema. Mock leagues are identifiable by their `mock.l.` key prefix.
- **Stat type:** GetTeamRoster passes statType directly to Yahoo's semicolon-path syntax (`type=week`, `type=lastweek`, `type=date;date=YYYY-MM-DD`, etc.)
- **Concurrent fetching:** use buffered channels to fan-out Yahoo API calls in GetTeamRoster, GetLeagueDraftResults, GetLeagueRankings
- **Dynamic stat columns:** frontend derives column headers from `roster[].stats[].label` — no hardcoded stat IDs
- **sort_order:** Yahoo's `sort_order` field ("1" = higher is better, "0" = lower is better) is passed through to the frontend for correct winner determination in MatchupDetail
- **Player rankings:** `analysis.go` concurrently fetches rosters, scoring categories, and top-25 FA stats. Category weights = `CV × scarcity` (normalised): CV = stdev/|mean|; scarcity = `1/(1 + max(0, avgFAz))`. `overall_score` = weighted z-sum. `position_score` = z-score within position group (independent). `RankedPlayer` response includes `position_score` + `position_rank`. Frontend: TeamDetail RosterTable shows Value column (`+8.3 #4` overall, `PG #2` position) + color-coded stat cells. See `docs/ranking-algorithm.md`
- **nflverse data import:** `cmd/import/` downloads roster + player_stats CSVs from nflverse GitHub releases, upserts into `nfl_players` and `nfl_player_stats`. Idempotent (ON CONFLICT upserts). Data available from 1999–present; default imports 2020–2024. `nfl_players.yahoo_id` links to Yahoo fantasy player keys for cross-referencing.
- **Comp-based projections:** `cmd/projections/main.go` is a batch CLI tool. Step 1 (`-profiles`) aggregates `nfl_player_stats` into per-player, per-season profiles: weekly production is SOS-adjusted (capped defense-vs-position factor), small-sample rates are Bayesian-shrunk toward position-group means (`shrinkage.go`), then z-scored (JSONB). Step 2 (`-project`) blends the target's base-season profile with their immediately preceding season (`recency_blend.go`, games-weighted × `TargetBlendDecay`, `0`=no-op — `docs/stats/recency-weighted-profiles.md`), finds similar historical players (similarity ≥ 0.60, not a fixed count) using weighted Euclidean distance over **dimension groups** (passing/rushing/receiving/value/physical/context/grade), then applies their post-match growth (weighted by `similarity²`), shrunk toward an age/position growth-rate baseline when comps are scarce (`growth_baseline.go`, `GrowthShrinkageK`, `0`=no-op — `docs/stats/bayesian-shrinkage.md` growth-rate extension). Comps that washed out of the league contribute zero growth (survivorship-bias guard); zero-comp players regress halfway to the position mean. Output includes an outcome distribution (`proj_fpts_ppr_stdev/_p10/_p50/_p90`), which stays consistent with the shrunk point estimate since both flow through the same weighted-sum machinery. Draft capital only used for players with < 3 years experience. See `docs/projection-algorithm.md` + `docs/algorithm-review.md` + `docs/stats/`.
- **Consensus divergence (diagnostic, non-mutating):** `cmd/projections/consensus.go` imports external rankings/ADP (`nfl_consensus_rankings`) and situational news like injuries/camp battles (`nfl_player_situational_notes`) from hand-curated JSON files (several major ranking sites block live scraping — see the entry's tradeoffs), resolves players via `sleeper_id`/`espn_id`/name+team fallback, and computes the gap between our projection rank and the **median** external rank/ADP, **ranked within position group** (QB vs QB, RB vs RB — ranking overall conflated raw point volume, which structurally favors QBs, with consensus draft value, which is scarcity-adjusted), into `nfl_projection_divergences` (`-consensus-diff`). Only `standard`/`half_ppr`/`ppr` are diffable — dynasty/superflex consensus data is stored but not compared, since our engine has no multi-year or QB-premium output to diff it against. Situational notes carry a `scope` (`player`|`team`) — team-scoped notes (QB competitions, scheme changes) print alongside every teammate's divergence line automatically, with no inference about which way the impact cuts for that specific player; that judgment stays with whoever reads the report. Nothing here changes `nfl_projections`; it's a review/diagnostic layer pending a season of retrospective validation. A full investigation of the first run's largest divergences (Justin Jefferson, Derrick Henry, etc.) found six overlapping root-cause clusters, not 36 separate bugs — see `docs/algorithm-review.md` §6 for the backlog of genuine algorithm questions it surfaced (injury-shortened base seasons, thin comp pools for rare veteran profiles, team-context blindness on trades). See `docs/stats/consensus-ensemble.md`.
- **Projection config:** `cmd/projections/backtest.go` defines `projConfig` (similarity threshold, age window, aging multipliers, per-position dimension-group weights — keyed by group name). Reads `projection_config.json` if present (stale per-stat-keyed configs are detected and ignored). Auto-tuner (`-autotune`) does coordinate ascent maximizing **per-game Spearman rank correlation** and only keeps the tuned config if it beats defaults on held-out validation seasons. `-backtest` runs temporal-integrity backtesting (prior-season data only; shared `projectSeasonBacktest` helper mirrors production) and stores metrics in `nfl_backtest_results` with `eval_basis` 'total'/'per_game' + quantile calibration coverage. Pre-June-2026 backtest rows measured a persistence baseline (bug) and are not comparable.
- **Player detail:** `GET /api/nfl/players/{gsisId}` returns metadata + year-over-year season stats + projection (if exists) + `notes` (situational context, rendered by `SituationalNotes.tsx` below the GradeCard; team-scoped notes are visually labeled so team news isn't misread as player-specific). Player rows in all tables (rosters, rankings, matchup, players tab, draft tab) are clickable and navigate to `/players/:gsisId`. Yahoo→GSIS lookup via `nfl_players.yahoo_id`; batch ANY() query for rosters, per-request map for rankings.
- **Draft tab:** League detail's "Draft" tab (`DraftSection.tsx`) groups everything draft-related into two sub-sections selected by `?sub=`: **Draft Values** (NFL only) and **Keepers** — keeper picks are draft picks, so they live together. Non-NFL leagues show Keepers alone with no sub-tab switcher; the legacy `?tab=keepers` URL rewrites to `?tab=draft&sub=keepers`. Draft Values uses `GET /api/leagues/{id}/draft-values` for league-specific VOR + auction values based on actual roster settings (superflex-aware); position filter only — it **reads** whatever settings were saved on `/draft-prep` (via `readSettings`) but can't edit them, and links there. Its target season is `league.season + 1` **clamped to `PROJECTION_SEASON`** — a league already sitting on the upcoming season (the mock dev league is seeded at 2026) would otherwise request a season with no projections. `GetDraftValues` normalises its nil slices to `[]` so an empty season returns `players: []`, not `null`. `/projections/:gsisId` routes redirect to `/players/:gsisId`.
- **Draft Prep page (`/draft-prep`):** the pre-draft surface. Owns everything editable about a draft board; the league page's Draft tab is the same board **read-only**, so there's one place to change things and no chance of the two disagreeing.
  - **Our price vs the market's.** The board carries `Cons $` (consensus auction value) and `Edge` (ours − theirs) columns, prep-page only (`showConsensus`). Uncovered players show `—`, never `$0`, and sort last either way — external sources cover roughly the top 100 picks (62 of 471 players in the 2026 data). Single-source values are flagged `*`. See `docs/stats/auction-values.md`; mechanism in `handlers/draft_consensus.go`.
  - **League picker** (NFL leagues only — the projection engine has no NBA equivalent), remembered in `localStorage` under `fs.draft-prep.league`. A league's roster and scoring are what turn projections into draft values, so the page needs one even though everything about it can then be overridden.
  - **Personal board** (`hooks/useDraftPrep.ts` → `draft_prep_players`): target/sleeper/avoid tags, a custom ranking, and a note per player, scoped to (user, league, season). Server-side, not `localStorage` — this is the board you rely on during a live draft, so it has to survive a different browser. Writes are optimistic with rollback, since tagging players during prep should feel instant.
  - **Reordering** rewrites the whole board in one `PUT .../order`: a rank only means something relative to every other player, so a partial write would leave the board inconsistent. The ↑/↓ buttons pass the *visible* neighbour rather than a direction, because with a position filter on, "move above the row above me" is the only reading that matches what you see. Server-side it's a single `unnest(...) WITH ORDINALITY` upsert — a few hundred round trips per nudge would make the board feel like work. Ranks are cleared first, so players dropped from the order don't keep a stale position; their tags and notes survive.
  - A row with no tag, no rank and no note carries no information, so that combination **deletes** the row (client and server agree on this).
- **Draft settings panel** (rendered only on `/draft-prep`): `components/DraftSettingsPanel.tsx` + `hooks/useDraftSettings.ts` let you edit the league settings that drive draft values — teams, auction budget, scoring format, and starting-lineup slots (QB/RB/WR/TE/FLEX/**SFLEX**/K/DEF).
  - **The draft math stays on the server.** Saving sends the settings as `draft-values` query overrides and the handler rescores; there is no second implementation in the frontend. `draftQuery(season, settings|null)` builds both the request params and the cache key, and is shared by DraftTab and `useKeepers` so the keeper tab's auction column agrees with the board.
  - **Edit → Save.** Edits are held as an unsaved draft (`editing`) so a half-typed roster never triggers a rescore; `save()` applies and persists them. `settings` is what the board is fetched with, `editing` is what the panel shows. The query uses `placeholderData: (prev) => prev`, so the old board stays on screen (with a "Rescoring…" note) while the new one loads.
  - **Persistence** is `localStorage` only — `fs.draft.settings.v1.{leagueId}` for settings, `fs.draft.position.v1.{leagueId}` for the position filter; nothing is stored server-side. `readSettings()` backfills keys added after a record was saved (SFLEX was), so older records can't leave an input uncontrolled. Overrides are sent **only once saved**: an uncustomized request carries no overrides, which is exactly what lets the response seed the panel without request and response chasing each other. "Reset" drops back to the league's real settings.
  - **Slot vocabulary.** The UI edits flat slot names; Yahoo expresses flex spots as eligibility lists (`W/R/T`, `Q/W/R/T`). `yahooToSlot`/`slotToYahoo` in `draft_values.go` translate between them, and the flex distribution itself stays in `ranking.ComputeStarterSlots` — both paths score identically (`TestSlotOverrideRoundTrip`). Flex shapes not modelled exactly (`W/R`) fold into FLEX. A `slots=` override that yields nothing startable is treated as absent rather than as an empty lineup, which would otherwise make every player worth his full point total.
  - With an explicit `slots=` override the handler no longer needs Yahoo's roster settings, so a roster-settings fetch failure is non-fatal on that path.
- **My Team tab:** the league page's first tab (`?tab=my-team`), rendered only when the signed-in user owns a team in that league (`teams[].user_id`). It reuses `team-detail`'s `TeamPanel`, so the roster, matchup card and stat-period switcher are the same component as `/teams/:id` — that page is now just a header around the panel. A `?tab=my-team` URL falls back to Standings when the user owns no team here, but only *after* teams load, so deep links aren't bounced mid-fetch. `/leagues/{id}/my-team` is a stable alias that redirects to the tab.
- **Player Grades:** Three-layer separation: (1) Player Grade — real-life quality (0-100 percentile), computed in `cmd/projections/grades.go`; (2) Stat Projections — comp-based; (3) Fantasy League Value — VORP/z-scores. Grades use position-specific sub-score weights (production, efficiency, usage, durability). Computed via `make project-nfl ARGS="-grades"`. `nfl_player_grades` table stores results. Grade z-score (`overall_grade_z`) is injected into season profiles and used as a similarity dimension for comp matching (weight 1.25). Grade YoY trend applies a bounded ±5% adjustment to projected stats. Frontend: GradeCard on player detail, Grade column on projections/draft/players tabs, Grades view on `/statistics`. Former Rankings tab absorbed into Players tab; former standalone `/rankings` page absorbed into `/statistics` (`?view=grades`).
- **Yahoo decoupling:** Yahoo is the source of fantasy *context* (ownership, scoring categories, roster slots, FA status), not NFL *stats*. NFL season rankings pull raw stat values from `nfl_player_stats` via `services/nflstats.LoadSeasonStats` keyed on `gsis_id`, translating Yahoo stat IDs through `services/scoring.YahooToCanonical`. Yahoo-only fallbacks remain for: non-season stat types (lastweek, today), NBA leagues (no non-Yahoo stats feed yet), and live in-season keeper/roster views. See `.claude/plans/yahoo-decoupling.md`.
- **SQL approach:** handlers use raw pgx queries for all database access. This is the established pattern — do NOT introduce sqlc or an ORM. `sqlc.yaml` exists but is unused; raw queries are preferred for their directness and flexibility with pgx features (e.g. `ANY($1)` with slices). Keep queries in handler methods, not in a separate query layer.
- **New resource checklist:** model → migration → handler → route → yahoo method (if needed) → TS interface → API function → query key → page → **update docs**
