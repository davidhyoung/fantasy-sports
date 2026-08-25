# Dynasty Transactions — hard cap, full dead money, competitive free agency

## Status (2026-08-25)

Design settled in conversation, nothing built yet. This document is the agreed
model plus the phased plan; `native-leagues.md` remains the parent initiative
and its Phases 0–4 (settings, rosters, contracts, picks, trades, rollover,
weekly play) are the foundation everything here sits on.

The one-line summary of what changes: **contracts stop being decorative.**
Today the cap is computed in the frontend for display only, no backend
mutation checks it, and a drop releases the full salary instantly — so a
manager can sign anyone to any deal and cut them for free. Every decision
below exists to close that.

## The model

Four decisions, made deliberately, that fix the shape of everything else:

1. **Hard cap.** The backend rejects any move that would put a team over its
   cap. Same rule on every path — assign, draft, trade, free-agent signing.
2. **Full dead cap.** Cutting a player with $40 × 3 years remaining charges
   $40 against each of those three seasons. The money is owed either way.
3. **Rookie scale.** A drafted player's contract is derived from his draft
   slot, not typed in by the commissioner.
4. **Competitive free agency.** Teams *offer* contracts; the player signs the
   best one. There is no FAAB — the salary cap is the only currency, so every
   dollar spent in free agency is a dollar unavailable to the roster.

Plus three follow-ups settled the same session:

5. **Cap space banks.** Unused space carries into next season, making
   deliberate austerity a real strategy.
6. **Taxi and IR players are discounted, not exempt** — configurable, default
   25% of salary for `TAXI`, 50% for `IR`.
7. **Offer ties break on earliest submission**, and **unsigned offers expire**
   at window close rather than rolling forward.

### The consequence worth stating out loud

Full dead cap plus a hard cap means **cutting a player never creates cap
space.** The only thing a cut buys is the roster spot; the salary is charged
regardless. The sole genuine escape from a bad contract is trading the player
to a team willing to absorb him.

That is deliberate, and the escape valve is scoped narrowly: **the cap blocks
additions, never obligations.** Dead money, rookie scale, and rollover can all
push a team over the cap — that's allowed. What's blocked while over is
*adding*: no signings, no draft picks, no incoming trades that raise payroll.
Salary-shedding trades are always legal. There is no amnesty. Being over the
cap is its own punishment and it resolves itself as contracts expire.

## The cap ledger

Banking turns the cap from a single settings number into a per-team,
per-season ledger:

```
cap(T, S)       = base_budget + banked(T, S)
banked(T, S)    = cap(T, S−1) − spend(T, S−1)
spend(T, S)     = Σ charged(active contracts) + Σ dead money charged to S
available(T, S) = cap(T, S) − spend(T, S)

charged(contract) = salary × slot_factor
  slot_factor: TAXI → taxi_cap_pct (default 0.25)
               IR   → ir_cap_pct   (default 0.50)
               else → 1.0
```

`banked` is **materialized at rollover**, not recomputed recursively on read.
Rollover is already the transaction that advances the season under a row lock;
freezing the carry-in there means a cap read is one query instead of a walk
back through league history, and it means an after-the-fact edit to an old
season can't silently rewrite every subsequent season's cap.

**This function must be the single source of truth**, called by every
mutation. The current arrangement — cap math living only in
`NativeRosterTab`'s summary strip — is exactly the thing being replaced. A
`services/cap` package (or a shared helper in `league_rosters.go`, matching
how `assignRosterTx` is already shared) computes it once; handlers gate on it.

Note that `slot_factor` makes the cap **slot-dependent**, which means moving a
player between BN and TAXI changes payroll. Slot changes therefore become
cap-gated mutations too — a detail that's easy to miss, since today
`UpdateLeagueRoster`'s slot path is pure lineup-setting with no financial
consequence.

## Transaction by transaction

### Rookie draft

Salary and length come from the pick. This is newly unblocked: `overall_pick`
has been NULL since migration 000023 because native leagues had no standings
to seed a draft order from — but weekly play shipped (migration 000024,
`league_matchups` + `nativeStandings`), so **reverse-standings order is now
computable**. `GenerateLeagueDraftPicks` populates `overall_pick`;
`UseLeagueDraftPick` derives terms from it and stops accepting `salary` /
`years_total` from the caller.

The scale should **interpolate along the auction-value curve**, not linearly.
`draft_values.go` already prices talent for this specific league's roster and
scoring; a linear scale would disagree with the board about what 1.01 is worth
relative to 1.02, and the board is the more considered number.

Config lives in `league_settings.rookie_scale` (JSONB, same reasoning as
`slots`/`scoring` — validated in Go, always read and written whole).

### Free agency

Teams submit offers (salary + years) against a **ranked priority list**. At
window close, resolution runs:

1. Free agents resolve in **market-value order, best player first**, so cap
   spent at the top of the market cascades down — the way real free agency
   actually behaves.
2. For each player: drop offers from teams that no longer have cap room or a
   roster spot; drop offers below the player's reservation value; score what
   remains; sign the best. Ties go to the earliest submission — it rewards
   decisiveness and can't be gamed by a $1 bump.
3. A signing voids that team's now-unaffordable offers, walking down their
   priority list.

Teams may over-offer. That's the point of the priority list: a front office
has a board and falls down it, rather than being restricted to offers it could
all simultaneously afford.

**Reservation value comes from `GetDraftValues`.** The app already computes a
league-specific auction dollar value per player from that league's own roster
settings and scoring — that *is* his market price. A fraction of it is the
floor below which he won't sign. This deliberately avoids inventing a second
valuation that would drift from the first.

**Offer scoring:**

```
score = salary × years × fit(years, L*)
fit   = 1 − penalty × |years − L*| / max_years
L*    = f(age, position prime, projection percentile)
```

`L*` is the player's preferred contract length. Age pushes it up (an older
player wants security); a high projection percentile pulls it down (a young
star would rather re-hit the market). Inputs already exist —
`reference_nfl_prime_years` has the position-specific prime zones and
`nfl_projections` has the quality signal.

**The formula is shown on the player's card.** A hidden scoring function turns
free agency into a lottery; a visible one makes it a solvable strategic
problem, which is the part that's actually fun. Same instinct as `/wiki`
existing at all.

**Windows:** a large offer period after rollover where most business happens,
then weekly resolution in-season for churn. Resolution is an **explicit
commissioner click, never a cron** — matching `ScoreLeagueWeek`, which is
manual for the same reason (the whole app's data pipeline is a batch import,
so nothing here is genuinely live).

Unsigned offers expire at window close. Re-offering is a deliberate act.

### Cuts

The roster row and contract row are deleted as they are today; a
`league_dead_money` row is written charging the full salary to each remaining
season. Dead money **must outlive the contract**, so it cannot hang off
`league_contracts` — that table cascades on drop by design (migration 000022's
composite FK), which is exactly what makes an orphaned contract
unrepresentable and also exactly why dead money needs its own table.

### Trades

Now cap-validated on both sides — `CreateLeagueTrade` currently validates
asset ownership and destination but never touches payroll. Since trades are
the only real escape from a bad contract, **salary retention** (eating part of
a deal to move a player) is a natural follow-on, deferred for now.

### Rollover

Extends `rolloverDynasty`: decrement dead money and drop exhausted rows,
compute and freeze next season's `banked`, open the offseason FA window. The
existing `lockLeagueAtSeason` idempotency gate covers all of it unchanged.

## Schema

```
league_team_seasons   league_id, team_id, season, base_budget, banked
league_dead_money     league_id, team_id, season, amount, source_gsis_id
league_fa_offers      league_id, gsis_id, team_id, salary, years,
                      priority, status, window_id, created_at
league_fa_windows     league_id, season, kind (offseason|weekly), opens_at,
                      closes_at, resolved_at
league_settings     + rookie_scale JSONB, taxi_cap_pct, ir_cap_pct
league_draft_picks    overall_pick actually populated
league_transactions   kind gains 'sign', 'dead_money'
```

`league_transactions.kind` already allows `'waiver'` in `acquired_via` and
lacks it in `kind` — an inconsistency from migration 000022/000023. The offer
model makes waivers moot, so the resolution is to leave `kind` without it and
stop using `acquired_via = 'waiver'` rather than adding it.

## Phases

| Phase | Scope |
|---|---|
| **5** | **Cap becomes real.** `league_team_seasons`, `league_dead_money`, one server-side cap function, hard enforcement on every mutation (assign, slot change, trade, pick use), max roster size enforcement, drops write dead money, taxi/IR discounting, multi-season cap panel replacing the frontend-only strip. |
| **6** | **Rookie scale.** Reverse-standings draft order into `overall_pick`, `rookie_scale` config, `UseLeagueDraftPick` derives terms. |
| **7** | **Free agency offers.** `league_fa_offers` + windows, valuation service (`L*`, reservation off draft-values), resolution algorithm, offer-sheet UI with drag-priority, player card showing preference and floor. |
| **8** | **Rollover integration.** Dead-money decrement, banking freeze, window open. |

Phase 5 is the hard prerequisite — 6 and 7 are independent of each other once
it lands, and 8 is small but can't be tested until 5 exists.

## Open / deferred

- **Salary retention in trades** — deferred, but it's the pressure valve that
  makes the dead-cap rule survivable, so expect to want it.
- **Max roster size** is not enforced anywhere today. Phase 5 has to add it,
  since "a roster spot" is now a scarce resource the FA resolver checks
  against. Natural definition: sum of configured slots including BN/TAXI/IR.
- **Does `banked` floor at zero?** A team that ends a season over the cap
  should presumably carry the deficit forward rather than resetting free.
  Leaning yes, carry it — but unsettled.
- **Multi-user.** Everything here still assumes the single-commissioner model.
  The offer system is the first mechanic that's genuinely more interesting
  with real opponents, since it's a sealed-bid auction against other managers.
  `league_transactions.created_by` and the team-claiming flow already
  anticipate this.
