# Native Leagues (dynasty first, keeper + redraft to follow)

## Status (2026-08-19)

- **Phase 0 — done.** `services/leaguesettings` extracted, `draft_values.go` refactored onto it, tests moved and green, byte-identical behavior confirmed against a Yahoo league.
- **Phase 1 — backend done, frontend not started.** Migration 000021 (`leagues.source`/`format`, `league_settings`), `POST /api/leagues` moved behind auth and rebuilt to create a native league (settings + teams, one transaction), `GET/PUT .../settings`, team CRUD, `requireCommissioner()` gate, `NativeSource`. Verified live: created a native dynasty league via curl, `GET /api/leagues/{id}/draft-values` priced a full board from local projections with zero Yahoo contact, non-owners/Yahoo leagues correctly rejected (403/422).
- **Not started:** `/leagues/new` creation UI, hiding Scoreboard/Standings for native leagues, source badges, Phase 2 (rosters/contracts/FA pool — `analysis.go` still Yahoo-only), Phase 3 (draft picks/trades), Phase 4 (rollover).
- **Deviation from the original Phase 0 sketch:** `leaguesettings.Source` ended up as two independent methods (`RosterPositions`, `ScoringMods`) rather than one combined `Settings()` call, because the handler needs their failures to fail differently (a roster-fetch error is only fatal without a slots override; a scoring-fetch failure just falls back to a default pointing system). `FetchSettings()` runs both concurrently for anyone who wants both, preserving the original two-goroutine fan-out.


## Context

`yahoo-decoupling.md` split Yahoo's two jobs and removed one of them:

> 1. **Fantasy context** — league scoring categories, roster slots, who owns whom, FA status. Yahoo is the only source for this and that's fine.
> 2. **Stats source** — ... this is where the heavy coupling lives.

That plan killed job 2 (`services/nflstats`, `services/scoring/statids.go`, public `/api/rankings` all shipped). **This plan kills job 1.** A native league has no Yahoo behind it, so the app itself has to become the system of record for fantasy context.

First league is a **dynasty auction with contracts**, single-user (Dave maintains all teams), offseason management only — no weekly lineups, matchups, or live scoring in this initiative. Keeper and redraft follow from the same machinery; weekly play is designed for but not built.

## What actually blocks a native league

`leagueYahooKey()` (`internal/handlers/yahoo_helpers.go:14`) hard-422s any league without a `yahoo_key`, and six handlers call it on every request:

| Handler | Yahoo reads | Native replacement |
|---|---|---|
| `draft_values.go` | roster positions, scoring modifiers | `league_settings` |
| `analysis.go` | rosters, scoring, roster positions, FA pool | `league_rosters` + `league_settings` |
| `league_players.go` | player search, available/FA pool | `nfl_players` + `league_rosters` |
| `keepers.go` | draft results, keeper state | `league_transactions` + `league_contracts` |
| `teams.go` | roster, scoring stat names | `league_rosters` + `league_settings` |
| `scoring.go` | scoreboard, standings | *out of scope — tabs hidden for native* |

Note the shape: **`draft_values.go` and `analysis.go` need exactly the same four inputs** — settings, roster positions, rosters, FA pool. A native league has all four locally. That's the seam.

## Three things that make this cheap

1. **The canonical settings vocabulary already exists.** `draftSettings` (`draft_values.go:71`) is `{num_teams, budget, format, slots, scoring}`, parsed from `slots=QB:1,RB:2,...` and `scoring=pass_yds:0.04,...` and echoed back to the client. That *is* the native settings shape. Persisting it means the native path and the existing "what if we went 14-team superflex" override path resolve through identical code — no second implementation of the draft math.

2. **Everything modern is `gsis_id`-keyed** — projections, grades, tiers, draft prep, divergences, consensus. A native league is *cleaner* than a Yahoo one: it skips `ResolveAllYahooToGsis` entirely. `/draft-prep`, `/statistics`, `/divergences`, tiering and auction values all work the day a native league exists.

3. **`rosters` and `players` (migration 000001) are dead.** `rosters` has zero references anywhere in the Go code; `players` only backs three vestigial CRUD handlers (`players.go`). No migration burden — build gsis-native tables and leave the corpses alone.

---

## Phase 0 — `leaguesettings` provider (behavior-preserving)

Ship this first, Yahoo-only, with green tests. Everything else falls out of it.

`internal/services/leaguesettings/`:

```go
type Settings struct {
    NumTeams  int
    Budget    int
    Format    string                              // ppr|half|standard|league
    Slots     map[string]int                      // editable vocabulary
    Positions []ranking.RosterPosition            // what ComputeStarterSlots wants
    Scoring   map[scoring.CanonicalStat]float64
}

type TeamRoster struct {
    TeamID  int64
    GsisIDs []string
}

// Source is the fantasy-context provider. Two implementations: Yahoo and native.
type Source interface {
    Settings(ctx context.Context) (Settings, error)
    Rosters(ctx context.Context) ([]TeamRoster, error)
    FreePool(ctx context.Context, limit int) ([]string, error)
}

func For(ctx context.Context, db *pgxpool.Pool, yc *yahoo.Client, leagueID int64) (Source, error)
```

- `yahooSource` wraps the four concurrent calls already in `rankNFLFromLocal` (`analysis.go:515`) and `GetDraftValues` (`draft_values.go:335`), plus `slotsFromYahoo` / `CanonicalModifiersFromYahoo`.
- Move `slotToYahoo`, `yahooToSlot`, `slotsFromYahoo`, `parseSlotOverride`, `parseScoringOverride`, `scoringEditableStats`, `defaultScoringFallback` out of `draft_values.go` into this package. They're settings vocabulary, not draft math.
- `draft_values.go` and `analysis.go` switch to the interface. The override layer stays in the handler: resolve base settings from the `Source`, then apply query overrides on top. Identical behavior, `settings.overridden` unchanged.

**Done when** `make test` is green and `/api/leagues/{id}/draft-values` + `/rankings` return byte-identical responses for a Yahoo league.

---

## Phase 1 — Native league CRUD

### Migration `000021_native_leagues`

```sql
ALTER TABLE leagues
    ADD COLUMN source TEXT NOT NULL DEFAULT 'yahoo'
        CHECK (source IN ('yahoo', 'native')),
    ADD COLUMN format TEXT NOT NULL DEFAULT 'redraft'
        CHECK (format IN ('redraft', 'keeper', 'dynasty'));

-- Rows created by the old public CreateLeague have no yahoo_key and no Yahoo
-- league behind them. They are native by definition.
UPDATE leagues SET source = 'native' WHERE yahoo_key IS NULL;

-- Canonical settings, in the same vocabulary as the draft-values override params.
CREATE TABLE league_settings (
    league_id  BIGINT PRIMARY KEY REFERENCES leagues(id) ON DELETE CASCADE,
    num_teams  INT    NOT NULL,
    budget     INT    NOT NULL DEFAULT 200,
    slots      JSONB  NOT NULL,   -- {"QB":1,"RB":2,"WR":3,"TE":1,"FLEX":1,"BN":6}
    scoring    JSONB  NOT NULL,   -- {"pass_yds":0.04,"pass_td":4,...}
    taxi_slots INT    NOT NULL DEFAULT 0,
    ir_slots   INT    NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`slots`/`scoring` as JSONB rather than columns: the vocabulary is already defined and validated in Go (`scoringEditableSet`, `slotToYahoo`), the whole blob is always read and written together, and adding a slot type shouldn't need a migration.

### Handlers

- **Fix first:** `POST /api/leagues` is currently registered in the *public* group (`cmd/api/main.go:91`) — anyone can create a league. Move it behind `RequireAuth` as part of this phase.
- `POST /api/leagues` — extended body: `{name, sport, season, source:'native', format, settings:{...}, teams:[{name}]}`. Creates league + settings + N teams in one transaction. Sets `leagues.user_id` to the creator (reused as commissioner).
- `GET|PUT /api/leagues/{id}/settings`
- `POST /api/leagues/{id}/teams`, `PUT|DELETE /api/leagues/{id}/teams/{teamId}`

`nativeSource.Settings()` reads `league_settings`; `Rosters()`/`FreePool()` return empty until Phase 2. Draft values and the draft-prep board work off a native league at the end of this phase.

### Frontend

- `League` type (`src/api/client.ts:11`) gains `source: 'yahoo' | 'native'` and `format`.
- `/leagues/new` wizard: name/sport/season → format → team count + names → roster slots → scoring → budget. The slots and scoring steps are the *same editors* the draft-values settings panel already has — extract them to `src/components/` rather than writing second versions.
- `league-detail/index.tsx`: hide Scoreboard and Standings tabs when `source === 'native'` (no weekly scoring yet). Home league list shows a source badge.

---

## Phase 2 — Rosters, contracts, free-agent pool

### Migration `000022_native_rosters`

```sql
CREATE TABLE league_rosters (
    league_id    BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    team_id      BIGINT NOT NULL REFERENCES teams(id)   ON DELETE CASCADE,
    gsis_id      TEXT   NOT NULL REFERENCES nfl_players(gsis_id),
    slot         TEXT   NOT NULL DEFAULT 'BN',   -- QB|RB|WR|TE|K|DEF|FLEX|BN|TAXI|IR
    acquired_via TEXT   NOT NULL,                -- draft|auction|trade|waiver|fa|keeper
    acquired_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- A player belongs to at most one team per league. This is the invariant the
    -- FA pool is derived from, so it's enforced here rather than in application code.
    PRIMARY KEY (league_id, gsis_id)
);
CREATE INDEX idx_league_rosters_team ON league_rosters (team_id);

CREATE TABLE league_contracts (
    league_id      BIGINT NOT NULL,
    gsis_id        TEXT   NOT NULL,
    salary         INT    NOT NULL,
    signed_season  INT    NOT NULL,
    years_total    INT,           -- NULL = year-to-year, escalated by keeper_rules
    years_used     INT    NOT NULL DEFAULT 1,
    PRIMARY KEY (league_id, gsis_id),
    FOREIGN KEY (league_id, gsis_id)
        REFERENCES league_rosters (league_id, gsis_id) ON DELETE CASCADE
);
```

Contracts hang off the roster row by composite FK, so dropping a player releases the contract automatically and an orphaned contract is unrepresentable.

Cap space is **derived** (`budget − SUM(salary)`), never stored.

### Wiring

- `nativeSource.Rosters()` — one query grouped by team.
- `nativeSource.FreePool()` — `nfl_players` LEFT JOIN `league_rosters` WHERE roster row IS NULL, ordered by projection.
- `analysis.go` native rankings now light up: all four inputs are local, and the `isPointsLeague && statType == "season"` branch already sources stats locally.
- `league_players.go` search/available get a native branch.
- Reuse `services/keepers.ComputeKeeperCost` for contract escalation — the model (`cost_increase`, `undrafted_base`, `max_years`) is already the right one, and `keeper_rules` already exists per league.

Frontend: a Roster/Contracts view on league detail — roster by slot, salary, years, cap space.

---

## Phase 3 — Draft picks and trades

Future picks as tradable assets. **This is the most dynasty-specific piece and has no analogue anywhere in the codebase.**

```sql
CREATE TABLE league_draft_picks (
    id               BIGSERIAL PRIMARY KEY,
    league_id        BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season           INT    NOT NULL,
    round            INT    NOT NULL,
    original_team_id BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    current_team_id  BIGINT NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    overall_pick     INT,    -- NULL until draft order is set
    used_on_gsis_id  TEXT REFERENCES nfl_players(gsis_id),
    UNIQUE (league_id, season, round, original_team_id)
);

-- Append-only history. Roster/contract/pick tables hold current state; this holds why.
CREATE TABLE league_transactions (
    id         BIGSERIAL PRIMARY KEY,
    league_id  BIGINT NOT NULL REFERENCES leagues(id) ON DELETE CASCADE,
    season     INT    NOT NULL,
    kind       TEXT   NOT NULL,  -- draft|auction|trade|add|drop|keeper|rollover
    payload    JSONB  NOT NULL,  -- {"moves":[{team_id, gsis_id|pick_id, direction, salary}]}
    created_by BIGINT REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

`POST /api/leagues/{id}/transactions` is the single mutation entry point: every roster change goes through it, in one DB transaction, writing both the state tables and the log. Trades move players and picks together atomically.

Deliberately keeping current state in tables rather than folding the log — a pure event-sourced roster would make the FA-pool query and the `(league_id, gsis_id)` uniqueness invariant much harder for no benefit at this scale.

---

## Phase 4 — Season rollover

One machinery, three rules — not three systems. `leagues.format` selects a strategy:

```go
type Rollover interface {
    // Given last season's state, produce next season's roster + contract rows.
    Roll(ctx context.Context, leagueID int64, from, to int) error
}
```

- **dynasty** — everything carries; contracts advance `years_used`; expiring contracts release to FA; rookie picks generated for the new season.
- **keeper** — carries only players designated as keepers, priced by `services/keepers.ComputeKeeperCost`; everyone else releases. The existing keeper wishlist/submission flow feeds it.
- **redraft** — release everything, regenerate full draft picks.

Emits a `kind='rollover'` transaction so the change is auditable and reversible.

---

## Open decisions

**Team defenses have no `gsis_id`.** `nfl_players` comes from nflverse *player* rosters — there are no DST rows, so `league_rosters.gsis_id REFERENCES nfl_players(gsis_id)` cannot hold a defense. The settings model already understands a `DEF` slot (`yahooToSlot`, `draft_values.go:120`) but the projection board can't price one today either, so this is a pre-existing gap the native league inherits rather than creates. Options: synthetic `gsis_id`s (`DST-KC`) inserted into `nfl_players`, a separate roster column, or no DEF slot in the dynasty league. **Cheapest for league one: no DEF, no K.** Decide before Phase 2.

**Yahoo OAuth is the only login** (`auth.go`). Fine while it's single-user, but "leaguemates later" means a second auth provider — every leaguemate would otherwise need a Yahoo account. Not blocking; worth knowing the shape of the bill.

**Authorization.** Single-user now, but put a commissioner/team-owner check on every mutation from the start (`leagues.user_id` for commissioner, `teams.user_id` for owner). Opening up later should be additive, not a rewrite.

**Season as `TEXT`.** `leagues.season` is TEXT (Yahoo's shape) while every native table here uses `INT`, matching `nfl_projections` and `draft_prep_players`. Not worth a migration; just don't join across them by accident.

## Not doing now

- Weekly lineups, matchup schedule generation, live scoring, waivers/FAAB. The schema is designed so these layer on (`league_rosters.slot` already carries starter slots; `nfl_player_stats` is weekly and `services/scoring` can already score a stat line) — but none of it is built.
- NBA native leagues (no non-Yahoo NBA stats feed, same blocker as `yahoo-decoupling.md`).
- Pushing anything back to Yahoo. Native leagues never touch Yahoo at all.
