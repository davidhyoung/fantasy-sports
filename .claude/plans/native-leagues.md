# Native Leagues (dynasty first, keeper + redraft to follow)

## Status (2026-08-21)

- **Pos click replaced with clickable destination rows (2026-08-21).** The floating eligible-slots dropdown from the previous entry was replaced per Dave's follow-up: clicking Pos now enters a "picking" mode where every legal destination *row* (not slot name) lights up (left accent edge + pointer cursor) and is directly clickable — fixes a real ambiguity the dropdown had (a slot name like "RB" can't tell two RB rows apart; a specific row can, so clicking an empty one assigns and clicking an occupied one swaps). Outside-click cancels. Verified live: correct rows lit up for an RB (RB/FLEX/SFLEX/BN/TAXI/IR only), click-to-assign and click-to-swap both worked, cancel worked.
- **Pos as a button, team switcher as a dropdown (2026-08-21).** Pos in `NativeRosterTable` now has visible button chrome (border + bg, hover state) instead of looking like plain text. `NativeRosterTab`'s team switcher replaced its row of up to 12 `FilterChip` pills with a single `SelectControl` dropdown (same primitive `PlayerAssignForm` already uses).
- **Slot column read-only, Pos click opens the slot picker (2026-08-21).** `NativeRosterTable` reordered to Slot/Player/Pos/Salary/Years/actions, with Slot now plain text instead of an always-visible `<select>`. Clicking Pos opens a small hand-rolled menu (no new dependency) of that player's eligible slots (via `isSlotEligible`), current slot shown disabled; only one row's menu is open at a time, closed by an outside click; opens upward near the bottom of the table to avoid clipping. Drag-and-drop is untouched (row-level, not cell-level). Verified live: RB's Pos menu correctly excluded QB/WR/TE/K, clicking FLEX moved the player, outside-click closed the menu cleanly.
- **Lineup slot eligibility (2026-08-21).** Players can now only be set into slots their real position is eligible for — dedicated QB/RB/WR/TE/K slots take only that position, FLEX takes RB/WR/TE, SFLEX takes QB/RB/WR/TE, BN/TAXI/IR take anyone. Enforced server-side first: `slotEligibleForPosition` (`league_rosters.go`) checks the player's real `nfl_players.position` against the target slot in both `AssignLeagueRoster` and `UpdateLeagueRoster`, rejecting a mismatch with a 400 — same pattern as every other native-league invariant (DEF rejection, double-rostering) living in the backend rather than being assumed from the UI. Frontend (`isSlotEligible` in `lib/nativeSlots.ts`, same table) mirrors it for UX: the roster table's slot dropdown and the assign-player form's slot picker both filter to eligible slots only, and drag-and-drop refuses an ineligible drop before it happens (via `onDragOver` not calling `preventDefault()`, which makes the browser itself refuse the drop — no separate error UI needed). Verified live: curl confirmed a 400 on RB→QB and a 200 on RB→FLEX; in the browser, an RB's dropdown excluded QB/WR/TE/K, dragging onto an ineligible empty slot fired zero network requests, and dragging onto an eligible one succeeded and survived a reload.
- **Drag and drop for lineup slots (2026-08-21).** `NativeRosterTable` rows are now draggable — native HTML5 DnD (`draggable`/`onDragStart`/`onDragOver`/`onDragLeave`/`onDrop`, payload is the dragged player's `gsis_id` as `text/plain`), no new dependency. It's a second way to set a player's slot, alongside the existing inline `<select>` dropdown, not a replacement — HTML5 DnD has no touch story, so the dropdown remains the one mechanism guaranteed to work on mobile/keyboard. Dropping on an empty row reassigns the dragged player (one `updateLeagueRoster` call); dropping on an occupied row swaps the two players' slots (two calls — the occupant takes the dragged player's old slot, then the dragged player takes the target's) rather than leaving a slot double-booked. Dragged-over rows highlight via `dragOverKey` state; both paths invalidate the same roster/free-agent/transaction queries the dropdown already used, so the two mechanisms can't disagree. Verified live on the user's real league 42: drag-to-empty (moved a rostered RB into an empty QB row) and drag-to-occupied (swapped two RBs' slots, confirmed via the network tab that both PUTs fired 200) both worked, and the result survived a full page reload.
- **Full lineup shown even when empty (2026-08-21).** `NativeRosterTable` used to only list players who were actually rostered, so an unstarted team showed nothing. It now takes the league's `slots` config (from `getLeagueSettings`, threaded down through `NativeTeamOverview`) and renders one row per configured slot via a new `SLOT_DISPLAY_ORDER` in `lib/nativeSlots.ts` (starters then BN/TAXI/IR — distinct from `ROSTER_SLOTS`, which is just the `<select>`'s option order), padding unfilled slots with an inert "Empty" row. A slot with more actual players than its configured count (e.g. after a settings change) still shows all of them — padding is `Math.max(configured, actual)`, never a truncation. Verified live: an empty team shows its full configured shape; a partially-filled team slots real players into the right position within that shape.
- **Players tab stat columns (2026-08-20).** The native league's Players tab now shows one column per scoring category the league actually weights nonzero — not a fixed set. Backend: `relevantPlayerStats` (`league_rosters.go`) reads the league's `league_settings.scoring` via `leaguesettings.NewNativeSource`, filters the 9-category `ScoringEditableStats` vocabulary to nonzero weights, and pulls season-total projections for just those categories from `nfl_projections` via `scoring.ProjectionToCanonicalTotals` — attached as `stats: [{stat, value}]` on both `GetLeagueRosters` and `GetLeagueFreeAgents`. Frontend: `NativePlayersTab.tsx` derives its columns from whatever stats are actually present in the fetched data, ordered by the existing `SCORING_STATS` vocabulary. Verified live: league 42 (scores all 9 categories) renders all 9 as columns with real, varying per-player values in both Free Agents and Rostered views.
- **My Team + Roster merged into one page, "Roster" (2026-08-21).** They'd become the same content twice. `NativeMyTeamTab.tsx` is deleted; `NativeRosterTab.tsx` is now the combined page — the team switcher (defaults to your claimed team) and commissioner tools (Claim/Trade/Assign/Rollover/activity log) sit alongside the matchup-card-and-roster view (`NativeTeamOverview`) for whichever team is selected. Native leagues show no separate My Team tab; a `?tab=my-team` deep link resolves to Roster. Yahoo leagues are untouched.
- **Standalone team page merged into Roster too (2026-08-21, same day).** The `/teams/:id` read-only native team page (`NativeTeamDetail.tsx`) turned out to be the *same* content a third time — no reader who isn't the commissioner exists in this single-user model, so "read-only" was never load-bearing. Deleted `NativeTeamDetail.tsx`; every team-name link (Standings, Scoreboard, Draft Picks, a matchup card's opponent) now points straight at `/leagues/{id}?tab=roster&team={teamId}`. `NativeRosterTab`'s selected team is derived from that `team` URL param (falling back to your claimed team, then the first team) rather than kept in `useState` — no state-vs-URL sync to maintain, and it sidesteps the exact "teams loads after mount, default freezes wrong" bug class fixed earlier in this initiative. `TeamDetailRouter` still exists at `/teams/:id` and still branches Yahoo vs. native, but the native branch is now just a `<Navigate>` redirect into the Roster tab, kept only so an old bookmark or stray link still resolves — the app itself never generates a `/teams/:id` link for a native team anymore. `NativeTeamOverview`/`NativeRosterTable` lost their `readOnly` prop in the same change (dead code once the one read-only caller was gone).

- **Phase 0 — done.** `services/leaguesettings` extracted, `draft_values.go` refactored onto it, tests moved and green, byte-identical behavior confirmed against a Yahoo league.
- **Phase 1 — done, backend + frontend.** Migration 000021 (`leagues.source`/`format`, `league_settings`), `POST /api/leagues` moved behind auth and rebuilt to create a native league (settings + teams, one transaction), `GET/PUT .../settings`, team CRUD, `requireCommissioner()` gate, `NativeSource`. `/leagues/new` creation form, "+ New league" on Home, Standings/Scoreboard hidden for native leagues with a stale-tab fallback to Draft, format badge in the league header.
- **Phase 2 — done, backend + frontend.** Migration 000022 (`league_rosters`, `league_contracts`). `league_rosters.go`: GetLeagueRosters, AssignLeagueRoster (roster row + contract row in one transaction via shared `assignRosterTx`, 409 on double-rostering), UpdateLeagueRoster (trade/slot/contract edits), DropLeagueRoster (contract cascades), GetLeagueFreeAgents (`nfl_players` minus `league_rosters`, ordered by projection, `?search=` name filter). Frontend: `NativeRosterTab.tsx` (native-only "Roster" tab) — team switcher, cap-space summary, roster table with inline Edit-contract/Drop, "Assign player" flow (`PlayerAssignForm.tsx`, the app's first player-search UI).
- **Phase 3 — done.** Migration 000023 (`league_draft_picks`, `league_transactions`, `league_settings.draft_rounds`). `draft_picks.go`: ListLeagueDraftPicks, GenerateLeagueDraftPicks (idempotent — 409 if a class already exists for that season), UseLeagueDraftPick (locks the pick row, spends it via `assignRosterTx` with `acquired_via="draft"`). `trades.go`: CreateLeagueTrade (atomic multi-asset player+pick swap, single-user model so the commissioner executes directly with no accept flow), ListLeagueTransactions (read, no gate). Frontend: `TradeBuilder.tsx` (2-team swap, players + unused picks), `NativeDraftPicksTab.tsx` (Draft tab's native-only "Picks" sub-section, replacing "Keepers" — which is Yahoo-shaped and would 422 for a native league), activity log in `NativeRosterTab.tsx`. New `components/ui/dialog.tsx` primitive (centered modal, no Radix, matches `mobile-sheet.tsx`'s no-animation convention) backs all three new flows.
- **Phase 4 — done for dynasty and redraft, keeper stubbed.** `rollover.go`: `RolloverLeague` switches on `leagues.format` (a plain switch, not an interface — one consumer, not enough real implementations to justify the indirection). `rolloverDynasty`: expiring contracts (`years_total` reached) release to FA, survivors get `years_used++`, next season's rookie picks generate, `leagues.season` advances. `rolloverRedraft`: full release + regenerate. `keeper` format 501s — no native keeper-designation flow exists yet (existing `keeper_wishlist`/`keeper_rules` are Yahoo-draft-result-shaped), consistent with the original "dynasty first, keeper/redraft later" sequencing. Idempotency is `lockLeagueAtSeason` (locks the league row, confirms `leagues.season` hasn't moved) — **not** "do next season's picks exist," which was tried first and produced a false-positive 409 on the very first rollover call whenever a commissioner had already pre-generated picks for trading purposes (a real, intended use case — picks are tradable in advance of the actual rollover). Frontend: rollover confirm-and-execute control in `NativeRosterTab.tsx`.
- **DEF dropped from native leagues entirely (2026-08-20):** nflverse has no team-defense `gsis_id`s, so `DEF` was allowed by the schema/validation but would always fail the `league_rosters` FK. Removed from `league_rosters.go`'s `validSlots` and rejected at the settings layer (`validateNativeSlots`, called from both `CreateLeague` and `UpdateLeagueSettings`); `CreateLeague.tsx`'s slot editor filters it out of what's offered for a native league. Kickers are unaffected (real `gsis_id`s exist). Yahoo leagues are untouched — DEF still works there via Yahoo's own player universe.
- **Verified live** against the real dev server end-to-end on a throwaway test league (created, exercised, deleted with zero orphaned rows): DEF rejection, pick-class generation + idempotency 409, FA name search, use-a-pick, a player+pick trade (plus self-trade/used-pick/duplicate-asset rejections), the transaction log, dynasty rollover (expiring contract released, survivors' `years_used` incremented, season advanced, re-run correctly rejected), redraft rollover (full release), and the keeper-format 501. `go build`/`go vet`/`tsc --noEmit`/`yarn build` all clean.
- **Weekly play — done beyond the original four phases (2026-08-20).** The user asked for native leagues to have the same My Team/Standings/Scoreboard screens Yahoo leagues have — genuinely new scope, not in the original phased plan below. Migration 000024 (`league_settings.regular_season_weeks`, `league_matchups`). `schedule.go`: `GenerateLeagueSchedule` (round-robin circle method, idempotent per season), `ScoreLeagueWeek` (explicit commissioner action — stats come from a manual `make import-nfl` batch import, never live — sums each team's starter-slotted players' real per-week stats via the new `nflstats.LoadWeekStats` through `scoring.ScoreWithModifiers`, freezes the result). `scoring.go`'s `GetLeagueScoreboard`/`GetLeagueStandings` gained native branches (`leagueSource()` dispatch) reading/aggregating `league_matchups` directly — no separate standings table, wins/losses/points are a pure read over scored matchups. Team claiming: `UpdateLeagueTeam` extended with an optional `claim` field (native teams start with no `user_id` at all — only Yahoo sync ever sets that — so this is what makes "My Team" reachable). **Regular season only, no playoff bracket** — `is_playoffs` reserved on the schema, not wired to any seeding logic. Frontend: `NativeMyTeamTab.tsx` (matchup card + roster, its own component rather than a branch inside the Yahoo `MyTeamTab`), `NativeStandingsTab.tsx`/`NativeScoreboardTab.tsx` (reuse the *same* `getLeagueStandings`/`getLeagueScoreboard` client calls as Yahoo — the branch is server-side), a shared `components/NativeRosterTable.tsx` with an **inline-editable Slot dropdown** — that's the entire lineup-setting UI, no separate "Lineup" screen; extracted from `NativeRosterTab.tsx` so the commissioner Roster tab and the personal My Team tab don't duplicate the row markup. Verified live end-to-end in the browser (not just build/curl) against the user's real league 42: claimed a team, generated a 14-week schedule, navigated weeks, edited a lineup slot and confirmed it survived a reload; scoring itself was separately verified via backend smoke test against real historical stats on a throwaway league, deliberately not run against the user's real league to avoid freezing arbitrary numbers into it.
- **Team pages, done (2026-08-20).** Clicking a team name anywhere in a native league (Standings, Scoreboard matchup cards, Draft Picks' owner columns) now goes to `/teams/:id`. `TeamDetailRouter.tsx` (new, sits in front of that route in `App.tsx`) resolves the team's league source *before* mounting anything — the existing Yahoo `TeamDetail`/`useTeamDetail` fires `getTeamRoster`/`getLeagueRankings` unconditionally, both Yahoo-only and 422 for a native team, so a native team gets routed to the new `NativeTeamDetail.tsx` instead and the Yahoo hook never mounts. Both `NativeTeamDetail` and `NativeMyTeamTab` render a shared `components/NativeTeamOverview.tsx` (matchup card + `NativeRosterTable`), the only difference being a `readOnly` flag — someone else's team gets no slot dropdown, no Edit/Drop. `NativeMyTeamTab` also gained a "Full team page →" link to its own `/teams/:id`, matching the existing Yahoo `MyTeamTab` convention. Verified live in the browser: clicked through Standings → team page (empty roster, correct matchup), My Team → "Full team page →" (own team, now read-only, contract terms intact), Draft Picks' original-owner column → traded-away player's new team page, and confirmed the Yahoo `/teams/:id` path (a real synced team) still renders correctly through the new router — no regression.
- **Not started:** `analysis.go` native rankings (still Yahoo-only — the category-metadata shape doesn't map 1:1 onto the native league's narrower editable-scoring vocabulary), `league_players.go` native player search (no equivalent to Yahoo's player search exists for native leagues — `GetLeagueFreeAgents`'s `?search=` covers the free-agent half only), native keeper-format rollover, playoff bracket/seeding, draft order / `overall_pick` (no real standings-based seeding exists yet even though standings themselves now do, so picks are still generated in team-id order and never get a real draft slot).
- **Deviation from the original Phase 0 sketch:** `leaguesettings.Source` ended up as two independent methods (`RosterPositions`, `ScoringMods`) rather than one combined `Settings()` call, because the handler needs their failures to fail differently (a roster-fetch error is only fatal without a slots override; a scoring-fetch failure just falls back to a default pointing system). `FetchSettings()` runs both concurrently for anyone who wants both, preserving the original two-goroutine fan-out.
- **Deviation from the original Phase 2 sketch:** `Rosters()`/`FreePool()` did *not* land on `leaguesettings.Source` — they're plain REST handlers in `league_rosters.go` (`GetLeagueRosters`, `GetLeagueFreeAgents`) instead. Reason: nothing outside this file needed the `Source` abstraction for them yet (`analysis.go` isn't wired to a native league at all, and won't be able to reuse `yahooRostersToLocalPlayerData`'s shape unchanged even once it is — see below), so adding the indirection now would be speculative. Revisit if/when `analysis.go` actually needs a provider-agnostic roster read.
- **Deviation from the original Phase 3 sketch:** `league_draft_picks`/`league_transactions` gained an idempotency-support column not in the original sketch — `league_settings.draft_rounds` (rounds-per-class is per-league config, same shape as `taxi_slots`/`ir_slots`). The sketch's rollover idempotency approach ("does a class exist for the target season") was replaced by `lockLeagueAtSeason` for the reason above.


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

- ✅ Roster reads/writes — `GetLeagueRosters`, `AssignLeagueRoster`, `UpdateLeagueRoster`, `DropLeagueRoster` (`league_rosters.go`), commissioner-gated, contract row written/cascaded alongside every roster row.
- ✅ `GetLeagueFreeAgents` — `nfl_players` LEFT JOIN `league_rosters` WHERE roster row IS NULL, ordered by projection for the league's season.
- ⬜ `analysis.go` native rankings — **not done, and not as simple as it looked.** The Yahoo path's category metadata (`buildCategoryMeta`, from `yahoo.LeagueStat` — arbitrary stat count, names, sort order) has no native equivalent: a native league's scoring is deliberately the *narrower* `leaguesettings.ScoringEditableStats` (9 categories with rate columns), not an open-ended category list. Wiring this needs a native-specific category-meta builder off `league_settings.scoring`, not just a new roster/FA data source behind the existing shape. Deferred.
- ⬜ `league_players.go` search/available — still Yahoo-only; a native league can use `GET .../free-agents` (added above) for the FA half, but there's no player-search equivalent yet.
- ✅ Contract escalation at rollover — turned out not to need `services/keepers.ComputeKeeperCost` after all: dynasty contracts are fixed-cost multi-year deals, not annually-escalating keeper costs, so rollover just increments `years_used` and releases on expiry. `ComputeKeeperCost` stays reserved for whenever native keeper-format leagues get built.

Frontend: ✅ `NativeRosterTab.tsx` — roster by slot, salary, years, cap space, assign/edit/drop, trade builder, rollover control. Done.

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
