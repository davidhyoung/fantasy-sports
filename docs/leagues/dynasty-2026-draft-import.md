# Importing the 2026 draft into league 13 ("Dynasty")

How `dynasty-2026-draft.md`'s 192 picks became real `teams`/`league_rosters`/
`league_contracts` rows for league 13, and the judgment calls that required.

## What existed before

League 13 had 12 placeholder teams ("Team 1"–"Team 12", unclaimed) with 180
random-looking `league_rosters` rows, all `acquired_via = 'fa'` — clearly test/mock
data from earlier native-league development, not a real draft (no `league_contracts`
rows, no coherent per-team logic). This import replaced all of it.

## Decisions made (confirmed with the user before running)

- **12 team-defense picks are dropped.** `league_rosters.gsis_id` has a foreign key
  to `nfl_players`, which has no rows for team defenses — the native roster model
  has no DEF support anywhere (see `CLAUDE.md`'s native-leagues scope cuts). The
  defense picks are preserved in `dynasty-2026-draft.md` for the record but aren't
  represented in the app.
- **Fresh auction picks (draft slots 1–160) get 1-year contracts.** The pick list
  only gives a price, not a contract length. Per the user, every non-keeper pick
  defaults to `years_total = 1` — standard "prove-it" convention for a dynasty
  startup auction; anyone can be extended later via a trade/re-sign.
- **Keeper picks (slots 161–192, the 32 players already recorded as `kept` in
  `draft_prep_players` from an earlier session) get `years_total = NULL`** — their
  real remaining contract length is genuinely unknown from the data available, and
  the column is nullable specifically for "unknown," so that's what's stored rather
  than guessing a number.
- **Starting lineups are auto-assigned, not from real data.** The draft gives price
  only, not who started each week. Slots were filled with the exact greedy
  algorithm `frontend/src/pages/draft-prep/lib/roster.ts`'s `buildRoster` already
  uses elsewhere in this app (most-restrictive-slot-first fill order, highest
  `proj_fpts_ppr` first within each slot) rather than a bespoke one-off rule, so a
  team's "starters" here read the same way the app's own team-builder would have
  assembled them. `SFLEX` is 0 in this league's settings and `DEF` is unfillable
  (see above), so the actual fill order used was QB→RB→WR→TE→K→FLEX→BN.
- **Team → team_id mapping** follows the pick list's own nomination order against
  the placeholder teams' ids in ascending order (145–156) — arbitrary but stable,
  since the placeholder teams had no other identity to preserve.
- **"Ripping Darts" (David's real team) is claimed for `user_id = 3` (Mock User)**,
  not `user_id = 1` (the real David Young account) — see
  `project_deployment_infra.md`'s memory note: Mock User is the account actually
  reachable in production while Yahoo login stays broken.
- **No `league_transactions` rows were backfilled.** Checked first: the app's own
  `AssignLeagueRoster` endpoint doesn't log a transaction for a plain roster
  assignment either (only `UseLeagueDraftPick`, trades, signs, and rollovers do) —
  so skipping this for a bulk historical import doesn't introduce any inconsistency
  with how the app behaves normally.

## Mechanics

Name→`gsis_id` resolution used the same fallback order as the consensus-rankings
importer (`cmd/projections/consensus.go`): exact name+team, then name+position,
normalizing suffixes (Jr./Sr./II/III) and punctuation. 179 of 180 non-DEF picks
resolved on the first pass; the one holdout ("Kenny Gainwell", TB) was a nickname
for the DB's "Kenneth Gainwell" (still showing his pre-trade team, PHI) — confirmed
and mapped by hand. Zero duplicate `gsis_id`s, zero ambiguous multi-match names.

The working script and generated SQL aren't checked into the repo (one-off data
migration, not reusable pipeline code) — this doc is the durable record of what ran
and why.
