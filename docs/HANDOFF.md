# Handoff — Fantasy Sports

_Last updated: 2026-10-14_

> **STATUS:** Current focus is the **native leagues** initiative (dynasty-first,
> keeper/redraft to follow) — Phases 0–4 done for dynasty & redraft, keeper stubbed
> (501). Full blow-by-blow status lives in
> [`.claude/plans/native-leagues.md`](../.claude/plans/native-leagues.md#status-2026-08-20)
> — this file exists so "what's the current focus" doesn't require checking four
> separate plan docs.

## Why this file exists

`CLAUDE.md` is the stable architecture reference (routes, handler map, patterns) —
it shouldn't have to double as a status log. `PLAN.md` is the frozen original design
doc (1 commit, unmaintained since). `.claude/plans/*.md` are per-initiative logs,
each with its own detail level and update cadence. This file is the one place to
check what's actually in flight right now, and where to look for the detail.

## Active initiatives

| Plan | Status |
|---|---|
| [`native-leagues.md`](../.claude/plans/native-leagues.md) | **In progress, most active.** Dynasty auction w/ contracts: Phases 0–4 done (league creation, rosters/contracts, draft picks/trades, season rollover). Not started: native rankings (`analysis.go`), native player search, native keeper-format rollover, real draft order. |
| [`yahoo-decoupling.md`](../.claude/plans/yahoo-decoupling.md) | **Shipped.** Rankings/projections no longer depend on Yahoo as a stats source (`services/nflstats`, canonical stat-ID scoring, public `/api/rankings`). |
| [`design-system-migration.md`](../.claude/plans/design-system-migration.md) | **Mid-flight, stale status marker (last stamped 2026-08-09).** Phases 1, 2, 3a–3b implemented; 3c/3e blocked on open decisions, 3d blocked on Phase 4 data, Phase 4 not started. Verify against the plan file before assuming this hasn't moved since. |
| [`steady-drifting-horizon.md`](../.claude/plans/steady-drifting-horizon.md) | Player Grades (real-life value, separate from fantasy value) — `grades.go`/`grades.tsx` exist per `CLAUDE.md`'s architecture section; check the plan file for remaining phases if picking this back up. |

## Next steps

Per `native-leagues.md`'s own "Not started" list: native rankings for native
leagues (`analysis.go` doesn't support them yet — category-metadata shape doesn't
map onto the narrower native scoring vocabulary), native player search
(`league_players.go` has no equivalent to Yahoo's search), native keeper-format
rollover (currently 501), and real draft order/`overall_pick` (no standings exist
for native leagues yet).

## Keeping this current

Update the STATUS line and the initiatives table when a plan file's own status
changes, rather than duplicating its detail here — this file should stay short
enough to read in under a minute.


## 2026-10-14 — NBA auction values moved to DARKO

- New `nba_darko_projections` table (migration 000040), `-import-darko` importer
  (`cmd/project-nba/darko.go`), and `GetNBADraftValues` rewritten to read it. 301 players
  imported for season 2026, all matched to `espn_id`. Details: `docs/stats/nba-auction-values.md`.
- Snapshot is stale (DARKO dataset as of 2026-06-03) and was cut to players projected ≥10 mpg;
  re-pull before the draft. The minutes blend/caps and pace clamp are untested judgment calls.
- Not touched: the old comp-based `nba_projections` pipeline (player detail, `/api/nba/rankings`,
  consensus diff still use it), the frontend (new `minutes` field on draft-value players is unused).
- Next: backtest the minutes/pace guards, decide whether to retire `nba_projections`, wire the
  snapshot pull into a repeatable script.
