Fantasy Sports

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
  - `analysis.go` — GetLeagueRankings: weighted z-score rankings; category weights = CV × FA-scarcity (normalised); adds `position_score` (z-score within position group); includes gsis_id lookup for player detail links
  - `projections.go` — ListProjections, GetProjectionDetail; serves pre-computed comp-based NFL player projections from nfl_projections table
  - `nfl_players.go` — GetNFLPlayer (full player detail: metadata + YoY stats + projection); GetNFLPlayerByYahooID (resolves Yahoo key → gsis_id and redirects)
  - `draft_values.go` — GetDraftValues: league-specific auction values (VOR + $ value based on actual roster settings)
  - `grades.go` — ListGrades, GetPlayerGrades: real-life player grades (0-100 percentile) from nfl_player_grades table; supports comma-separated position filter (e.g. `?position=RB,WR,TE`)
- `internal/models/models.go` — shared domain types (User, League, Team, Player, RosterEntry)
- `internal/middleware/auth.go` — RequireAuth: reads session, attaches *models.User to ctx
- `internal/yahoo/` — Yahoo Fantasy API client, OAuth config, XML types
  - `client.go` — all API methods + dbTokenSource (auto-refreshes + persists tokens)
  - `oauth.go` — Yahoo OAuth2 endpoint + NewOAuthConfig
  - `types.go` — all XML response structs
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
  - `Home.tsx`, `Leagues.tsx` — simple single-file pages
  - `league-detail/` — `index.tsx` + tab components (StandingsTab, ScoreboardTab, PlayersTab, KeepersTab, RankingsTab, DraftTab [NFL only]) + hooks
  - `team-detail/` — `index.tsx` + `components/` (RosterTable, MatchupCard) + `hooks/useTeamDetail.ts`
  - `matchup-detail/` — `index.tsx` + `components/` (CategoryTotalsTable, TeamRosterTable) + hook
  - `player-detail/` — `index.tsx` + `components/GradeCard.tsx` — unified NFL player detail page (metadata, grade card, YoY stats table, projection with PPR/Half/Standard toggle + comps)
  - `projection-detail/` — `index.tsx` + `components/` (CompCard, TrajectoryChart) + hook — legacy detail page (redirects to player-detail)
  - `rankings/` — `index.tsx` + `hooks/useRankings.ts` — standalone real-life player grades page (top-level /rankings) with Flex/Superflex position filters
  - `projections/` — `index.tsx` + `hooks/useProjections.ts` + `components/` (ProjectionTable, ConfidenceBadge, UniquenessBadge) — comp-based projection rankings with PPR/Half/Standard toggle
- `src/components/ui/` — shadcn/ui components (badge, button, input, table, tabs, provider) + table-helpers (SortableHead, PlayerCell, ClickableRow, ZScoreCell, HeaderRow)
- `src/App.tsx` — router + nav (with active page highlighting) + auth check
- `src/main.tsx` — React root, QueryClientProvider, BrowserRouter
- Vite proxies `/api` and `/auth` → `http://localhost:8080` in dev
- HTTPS via `vite-plugin-basic-ssl` (required for Yahoo OAuth)

## Routes

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
  GET  /api/leagues/{id}/draft-values?season=&format=ppr|half|standard&budget=200
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

## Key Patterns

- **Handler pattern:** all handlers are methods on `*Handler`; use `r.Context().Value(models.UserContextKey)` for current user in protected routes
- **Yahoo client:** instantiate per-request via `yahoo.NewClient(ctx, db, oauthConfig, userID, accessToken, refreshToken, expiry)`; tokens auto-refresh and persist to DB
- **Stat type:** GetTeamRoster passes statType directly to Yahoo's semicolon-path syntax (`type=week`, `type=lastweek`, `type=date;date=YYYY-MM-DD`, etc.)
- **Concurrent fetching:** use buffered channels to fan-out Yahoo API calls in GetTeamRoster, GetLeagueDraftResults, GetLeagueRankings
- **Dynamic stat columns:** frontend derives column headers from `roster[].stats[].label` — no hardcoded stat IDs
- **sort_order:** Yahoo's `sort_order` field ("1" = higher is better, "0" = lower is better) is passed through to the frontend for correct winner determination in MatchupDetail
- **Player rankings:** `analysis.go` concurrently fetches rosters, scoring categories, and top-25 FA stats. Category weights = `CV × scarcity` (normalised): CV = stdev/|mean|; scarcity = `1/(1 + max(0, avgFAz))`. `overall_score` = weighted z-sum. `position_score` = z-score within position group (independent). `RankedPlayer` response includes `position_score` + `position_rank`. Frontend: TeamDetail RosterTable shows Value column (`+8.3 #4` overall, `PG #2` position) + color-coded stat cells. See `docs/ranking-algorithm.md`
- **nflverse data import:** `cmd/import/` downloads roster + player_stats CSVs from nflverse GitHub releases, upserts into `nfl_players` and `nfl_player_stats`. Idempotent (ON CONFLICT upserts). Data available from 1999–present; default imports 2020–2024. `nfl_players.yahoo_id` links to Yahoo fantasy player keys for cross-referencing.
- **Comp-based projections:** `cmd/projections/main.go` is a batch CLI tool. Step 1 (`-profiles`) aggregates `nfl_player_stats` into per-player, per-season profiles with pre-computed z-scores stored as JSONB. Step 2 (`-project`) finds similar historical players (similarity threshold ≥ 0.60, not a fixed count) using weighted Euclidean distance on z-scored dimensions, then applies their post-match development trajectories (weighted by `similarity²`) to project the target player. Comp count is itself a signal: many comps = common archetype (higher confidence), few/zero = unique profile. Draft capital only used as a similarity dimension for players with < 3 years experience. See `docs/projection-algorithm.md`.
- **Projection config:** `cmd/projections/backtest.go` defines `projConfig` (similarity threshold, age window, per-position dimension weights). Reads `projection_config.json` if it exists; otherwise uses hardcoded defaults. Auto-tuner (`-autotune`) does coordinate descent over these parameters across training seasons and saves the best config. `-backtest` runs temporal-integrity backtesting (only prior-season data used) and stores RMSE/MAE/correlation metrics in `nfl_backtest_results`.
- **Player detail:** `GET /api/nfl/players/{gsisId}` returns metadata + year-over-year season stats + projection (if exists). Player rows in all tables (rosters, rankings, matchup, players tab, draft tab) are clickable and navigate to `/players/:gsisId`. Yahoo→GSIS lookup via `nfl_players.yahoo_id`; batch ANY() query for rosters, per-request map for rankings.
- **Draft tab (NFL only):** League detail page shows a "Draft" tab (visible only when `league.sport === 'nfl'`). Uses `GET /api/leagues/{id}/draft-values` to fetch league-specific VOR + auction values based on actual roster settings (superflex-aware). Position filter + PPR/Half/Standard format toggle. `/projections/:gsisId` routes redirect to `/players/:gsisId`.
- **Player Grades:** Three-layer separation: (1) Player Grade — real-life quality (0-100 percentile), computed in `cmd/projections/grades.go`; (2) Stat Projections — comp-based; (3) Fantasy League Value — VORP/z-scores. Grades use position-specific sub-score weights (production, efficiency, usage, durability). Computed via `make project-nfl ARGS="-grades"`. `nfl_player_grades` table stores results. Grade z-score (`overall_grade_z`) is injected into season profiles and used as a similarity dimension for comp matching (weight 1.25). Grade YoY trend applies a bounded ±5% adjustment to projected stats. Frontend: GradeCard on player detail, Grade column on projections/draft/players tabs, standalone `/rankings` page. Former Rankings tab absorbed into Players tab.
- **SQL approach:** handlers use raw pgx queries for all database access. This is the established pattern — do NOT introduce sqlc or an ORM. `sqlc.yaml` exists but is unused; raw queries are preferred for their directness and flexibility with pgx features (e.g. `ANY($1)` with slices). Keep queries in handler methods, not in a separate query layer.
- **New resource checklist:** model → migration → handler → route → yahoo method (if needed) → TS interface → API function → query key → page → **update docs**
