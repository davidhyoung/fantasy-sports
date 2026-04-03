# Fantasy Sports App — Planning Document

> Working doc for tracking what we want to build and in what order.
> Update this as priorities shift.

---

## Decisions Made

| Question | Decision |
|----------|----------|
| Sports scope | Football (NFL) + Basketball (NBA) first |
| Auth | Yahoo OAuth 2.0 (Authorization Code Grant) |
| Data source | Yahoo Fantasy Sports API |
| Scoring | Pull from Yahoo API (their scoring engine) |
| External API format | Yahoo returns **XML** (not JSON) — we parse in Go |

---

## Current State (as of 2026-03-12)

| Area | Status |
|------|--------|
| DB schema (leagues, teams, players, rosters) | ✅ Done (migration 000001) |
| Leagues API (list, get, create) | ✅ Done — will be replaced/augmented by Yahoo sync |
| Players API (list, get, create) | ✅ Done — will be replaced/augmented by Yahoo sync |
| Health endpoint | ✅ Done |
| Frontend: Home + Leagues pages | ✅ Done |
| Typed API client (client.ts) | ✅ Done |
| Docker Compose full stack | ✅ Done |
| Yahoo OAuth 2.0 login | ✅ Done |
| Users table | ✅ Done (migration 000002) |
| Yahoo API client (Go) | ✅ Done |
| League sync from Yahoo | ✅ Done (POST /api/sync, syncs leagues + teams) |
| Teams API | ✅ Done — `GET /api/leagues/{id}/teams`, `GET /api/teams/{id}` |
| Rosters API | ✅ Done — `GET /api/teams/{id}/roster` (live from Yahoo) |
| League detail + team list frontend | ✅ Done — LeagueDetail + TeamDetail pages |
| Scoring/stats from Yahoo | 🚧 In progress (Phase 5) |
| Tests | ❌ Not started |

---

## Architecture

### Auth Flow (Yahoo OAuth 2.0)

```
Browser                 Our Backend               Yahoo
   |                        |                        |
   |  GET /auth/login        |                        |
   |----------------------->|                        |
   |  redirect to Yahoo      |                        |
   |<-----------------------|                        |
   |                        |                        |
   |        Yahoo login + consent                    |
   |<----------------------------------------------->|
   |                        |                        |
   |  GET /auth/callback?code=...                    |
   |----------------------->|                        |
   |                        |  POST /oauth2/token    |
   |                        |----------------------->|
   |                        |  {access_token,        |
   |                        |   refresh_token,       |
   |                        |   expires_in}          |
   |                        |<-----------------------|
   |                        |  store tokens in DB    |
   |  session cookie        |  upsert user by guid   |
   |<-----------------------|                        |
```

**Key OAuth 2.0 details:**
- Scope: `fspt-r openid profile email` (Yahoo only offers read for 3rd-party fantasy apps)
- Access tokens expire in ~1 hour — store refresh token and auto-refresh
- Yahoo returns user GUID — use as stable user identifier
- Register app at: https://developer.yahoo.com/apps/

### Data Strategy: Hybrid (Yahoo as source of truth)

Yahoo owns the data. We sync it into our DB for:
- Fast queries without hitting Yahoo every time
- Adding custom features on top (notes, custom scoring, etc.)
- Offline tolerance

```
Yahoo API  --sync-->  Our Postgres DB  --serves-->  Our Frontend
```

**Sync triggers:**
- On login: sync user's leagues for current season
- On demand: user hits "Refresh" button
- Background (later): scheduled job

### Key Yahoo API Details

- Base URL: `https://fantasysports.yahooapis.com/fantasy/v2/`
- Response format: **XML** (not JSON)
- Key resources: `game`, `league`, `team`, `player`, `roster`, `scoreboard`, `standings`
- Resource keys follow pattern: `{game_key}.l.{league_id}` (e.g. `nfl.l.12345`)
- Game codes: `nfl` (football), `nba` (basketball)

---

## Database Changes Needed

### New: `users` table
```sql
CREATE TABLE users (
  id             BIGSERIAL PRIMARY KEY,
  yahoo_guid     TEXT NOT NULL UNIQUE,
  display_name   TEXT NOT NULL,
  email          TEXT,
  access_token   TEXT NOT NULL,
  refresh_token  TEXT NOT NULL,
  token_expiry   TIMESTAMPTZ NOT NULL,
  created_at     TIMESTAMPTZ DEFAULT NOW()
);
```

### Modify: `leagues` table
Add `yahoo_key` (e.g. `nfl.l.12345`), `commissioner_id` (→ users), status.

### Modify: `teams` table
Add `yahoo_key`, rename `owner_id` → `user_id` (FK to users), add manager name.

### Modify: `players` table
Add `yahoo_player_key`, sport-specific positions (different for NFL vs NBA).

---

## Build Order (Phases)

### Phase 1 — Yahoo OAuth + User Identity
**Goal:** User can log in with Yahoo account.

1. Add `users` table (migration 000002)
2. Implement `/auth/login` — redirect to Yahoo with OAuth params
3. Implement `/auth/callback` — exchange code for tokens, upsert user, set session cookie
4. Implement `/auth/me` — return current user info
5. Implement `/auth/logout`
6. Middleware: session validation (attach user to request context)
7. Frontend: Login button → Yahoo redirect; show logged-in state in nav

**Go packages to add:** `golang.org/x/oauth2`, a session library (e.g. `gorilla/sessions` with cookie store)

---

### Phase 2 — Yahoo API Client
**Goal:** Go package that wraps Yahoo Fantasy Sports API calls.

1. Create `internal/yahoo/client.go` — authenticated HTTP client with auto token refresh
2. XML structs for Yahoo responses (Game, League, Team, Player, Roster)
3. Methods:
   - `GetUserLeagues(accessToken, gameCode) []League`
   - `GetLeague(leagueKey) League`
   - `GetTeam(teamKey) Team`
   - `GetRoster(teamKey, week) []Player`
   - `GetStandings(leagueKey) []TeamStanding`
   - `GetScoreboard(leagueKey, week) []Matchup`

---

### Phase 3 — League Sync
**Goal:** After login, user's Yahoo leagues appear in our app.

1. `/api/sync` endpoint — fetches user's leagues from Yahoo, upserts into our DB
2. Update League model/schema to hold `yahoo_key`
3. Sync teams within each league
4. Show synced leagues on Leagues page

---

### Phase UI — Chakra UI Component Library
**Goal:** Replace ad-hoc Tailwind classes with Chakra UI v3 for a consistent, accessible design system.

**Why now:** Before building more screens (scoreboard, standings, keeper tool), establish a proper component library so all new UI shares the same look and behaviour.

1. Install `@chakra-ui/react @emotion/react`
2. Create `src/components/ui/provider.tsx` and wrap the app in `main.tsx`
3. Remove Tailwind directives from `index.css` (Chakra provides its own reset)
4. Migrate all existing pages to Chakra components (`Box`, `Flex`, `Button`, `Table`, `Tabs`, etc.)

---

### Phase 4 — Teams & Rosters
**Goal:** User can see their team and current roster.

1. Teams endpoints: `GET /api/leagues/{id}/teams`, `GET /api/teams/{id}`
2. Rosters endpoint: `GET /api/teams/{id}/roster?week=N`
3. Frontend: League detail page → team list → team detail → roster

---

### Phase 5 — Scoring & Standings
**Goal:** Show fantasy scores and standings.

1. Scoreboard endpoint: `GET /api/leagues/{id}/scoreboard?week=N`
2. Standings endpoint: `GET /api/leagues/{id}/standings`
3. Frontend: Scoreboard and standings views

---

### Phase 6 — Player Stats & Browse
**Goal:** Browse available players, see stats.

1. Player search/list with Yahoo player data
2. Player detail page with stats
3. Waiver wire / free agents view

---

### Phase FE-A — TanStack Query (Server State Management)
**Goal:** Replace manual `useEffect`/`useState` data-fetching with TanStack Query for caching, background refetching, and loading/error states.

**Decision:** TanStack Query v5 over SWR
- SWR: 4 KB bundle but no built-in pagination, no devtools, less TypeScript inference
- TanStack Query v5: 16 KB, excellent pagination (`useInfiniteQuery`), rewritten devtools, best-in-class TypeScript. Bundle size is negligible.

**Work:**
1. `yarn add @tanstack/react-query @tanstack/react-query-devtools`
2. Wrap app in `<QueryClientProvider>` (inside Chakra `<Provider>`)
3. Replace each page's `useEffect` + `useState` pair with `useQuery`
4. Use `useMutation` for `sync()` (POST /api/sync)
5. Add `<ReactQueryDevtools />` in dev mode
6. Use `useInfiniteQuery` for paginated player browse

---

### Phase FE-B — Testing & Component Library
**Goal:** Confidence through unit/integration tests and visual component docs.

**Tools:**
- **Vitest** — native Vite test runner, same config, fast HMR-aware
- **React Testing Library** — component tests without implementation details
- **Storybook 8** — component stories; works with Chakra UI + Vite natively

**Work:**
1. `yarn add -D vitest @testing-library/react @testing-library/user-event jsdom`
2. Add `vitest.config.ts` (extends vite config, jsdom environment)
3. Write tests for: API client helpers, key components (Leagues, TeamDetail)
4. `npx storybook@latest init` — auto-detects Vite + React
5. Add stories for shared UI components (`Button`, roster table, scoreboard card, standings table)

---

### Phase FE-C — Product Analytics (PostHog)
**Goal:** Understand how the app is used without compromising privacy.

**Decision:** PostHog Cloud over Mixpanel/Amplitude
- Mixpanel: 20 M free events but cloud-only
- PostHog: 1 M free events + self-hostable escape hatch + includes session replay, feature flags, and error tracking in one SDK — best for engineering-led teams

**Work:**
1. `yarn add posthog-js`
2. Create `VITE_POSTHOG_KEY` env var
3. Initialize PostHog in `main.tsx` with `posthog.init(key, { api_host: ... })`
4. Instrument key events: `login`, `sync_leagues`, `view_scoreboard`, `view_roster`, `search_players`
5. Add feature flag support for rolling out keeper tool

---

### Phase BE-A — OpenAPI Documentation (oapi-codegen)
**Goal:** Auto-generate an OpenAPI 3.0 spec from the Go backend; use it to generate a typed frontend client.

**Decision:** oapi-codegen (spec-first) over swaggo (comment-based, OpenAPI 2.0) and Huma (newer but smaller ecosystem)
- Spec-first enforces API contracts upfront
- Generates Chi-compatible server interfaces
- Frontend can `openapi-typescript` codegen from the same spec

**Work:**
1. Install `oapi-codegen` CLI: `go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest`
2. Write `api/openapi.yaml` — define all existing endpoints (leagues, teams, players, scoreboard, standings, rosters)
3. Generate server stubs: `oapi-codegen --config oapi-server.yaml api/openapi.yaml`
4. Wire generated interfaces to existing handlers
5. Serve spec at `GET /api/openapi.yaml` and Swagger UI at `GET /api/docs`
6. Frontend: `yarn add -D openapi-typescript` → codegen types from spec → replace hand-written interfaces in `client.ts`

---

### Phase 7 — Keeper Management Tool
**Goal:** Help managers decide keepers by showing draft history and projected keeper costs.

**What Yahoo provides:**
- `GET /team/{key}/draftresults` — round and pick each player was drafted
- `GET /league/{key}/players;status=K` — players already designated as keepers this season
- No dedicated `keeper_cost` field — we derive the cost ourselves

**Approach (read-only, derive keeper cost from draft data):**
1. Fetch draft results for every team in the league
2. Apply the keeper cost rule: keeper costs the round **one earlier** than they were drafted
   - (e.g., drafted in round 5 → costs a round 4 pick next year)
   - Round 1 picks are typically not keepable (league-dependent — surface as a warning)
3. Fetch current keeper designations (`status=K`) to show which players are already locked in
4. Allow managers to **locally mark** potential keepers (stored in our DB, not pushed to Yahoo — write API not available)
5. Show a per-team keeper comparison view: projected cost, value tier, position scarcity

**New endpoints:**
- `GET /api/leagues/{id}/draftresults` — aggregated draft board with keeper cost column
- `GET /api/leagues/{id}/keepers` — current keeper-designated players
- `POST/DELETE /api/teams/{id}/keepers/{playerKey}` — local keeper wishlist (our DB only)

**New frontend:**
- Keeper tab on LeagueDetail page
- Per-team keeper planning table: player name | drafted round | keeper cost | position | designate toggle

---

## Technical Considerations

### XML Parsing in Go
Yahoo returns XML. Go's `encoding/xml` package handles this natively.
```go
type YahooLeague struct {
  XMLName xml.Name `xml:"league"`
  LeagueKey string `xml:"league_key"`
  Name      string `xml:"name"`
  // ...
}
```

### Token Refresh
Access tokens last ~1 hour. The Yahoo client should:
- Check `token_expiry` before each request
- If expired (or within 5 min), call token refresh endpoint
- Update tokens in DB

### Session Management
Options for session storage:
- **Cookie-based** (signed/encrypted JWT or gorilla/sessions) — simpler, no DB table needed
- **Server-side sessions in DB** — more secure, revocable

Recommendation: start with encrypted cookie sessions (no extra table needed), upgrade later.

### Multi-sport
Yahoo uses different `game_key` values per sport per season. NFL = `nfl`, NBA = `nba`.
Our sync should accept a game code and handle both.

---

## Open Questions

1. **Session storage** — encrypted cookie or server-side DB sessions?
2. **Which season?** — Current season only, or allow historical?
3. **Manual leagues** — Keep the ability to create leagues manually (not from Yahoo)?
4. **Write operations** — Do we want to push changes back to Yahoo (roster moves, trades) or read-only for now?

---

## Decisions Log

| Date | Decision | Rationale |
|------|----------|-----------|
| — | Go backend | User learning Go; performance headroom |
| — | Chi router | Lightweight, idiomatic |
| — | Raw pgx first, sqlc later | Get moving fast; migrate when queries grow |
| 2026-03-12 | Yahoo OAuth 2.0 for auth | Single provider, doubles as data source |
| 2026-03-12 | Yahoo Fantasy API as data source | Rich data; avoid manual player/stats ingestion |
| 2026-03-12 | Hybrid data model | Cache Yahoo data locally for speed + custom features |
