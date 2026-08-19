# Design Handoff — Mobile Optimization

This doc catalogs every screen and UI state in the app so a design-focused
session (Claude Design) can produce mobile-optimized designs without having
to re-derive the app's structure from the codebase. It describes **what
exists today** on desktop, then enumerates **exactly what needs a mobile
design** — not a wishlist, a checklist. The result gets handed back to an
implementation session, so precision here saves a round trip later.

## What this product is

A full-stack fantasy sports companion app (NFL primary, NBA supported) for
people who play in Yahoo fantasy leagues. Users log in with Yahoo OAuth,
sync their leagues, and get two categories of value:

1. **Live league data** pulled from Yahoo: rosters, scoreboards, standings,
   matchups, keeper management, draft results.
2. **Original analysis Yahoo doesn't provide**: a comp-based NFL stat
   projection engine, real-life player grades (0–100 percentile, independent
   of fantasy scoring), VORP-based rankings and auction draft-value
   calculators, a personal draft-prep board with drag-and-drop reordering,
   and a diagnostic layer comparing our projections against external
   consensus rankings.

**Audience**: fantasy football players doing draft prep and in-season roster
management. Assume moderate-to-high domain literacy (they know what VORP,
WOPR, and target share mean) — this is a power-user tool, not a mass-market
consumer app. Draft prep in particular happens in bursts of high engagement
(the week or two before a live draft) where the user is frequently on their
phone, sometimes literally *during* a live draft on another screen — mobile
usability there is not a nice-to-have.

## Tech & platform constraints (design has to fit inside these)

- React 18 + Vite 5 + TypeScript, React Router v6 — client-side routing, no
  SSR, no native mobile app. **This is a responsive web app, not a
  React-Native/Capacitor build** — "mobile" means "this same web app,
  rendered well on a phone browser," not a separate app shell.
- Tailwind CSS 3 (default breakpoints: `sm` 640px, `md` 768px, `lg` 1024px,
  `xl` 1280px) + a minimal `shadcn/ui` (Radix-based) install.
- TanStack Query v5 for all data fetching.
- `lucide-react` icons — but see the design system note below: icons are
  used sparingly and **never** in table headers.
- Any design direction should assume it's implemented as Tailwind utility
  classes against the existing token set, not a new design-token pipeline —
  there's no Figma-to-code sync.
- `<meta name="viewport" content="width=device-width, initial-scale=1.0">`
  is already in place; no PWA manifest, no installability today.

## Current design system (as-built, source of truth)

The visual system already went through one full migration pass (tokens,
fonts, IA) and is considered done on desktop — a mobile pass should **extend
it, not replace it**.

- **Color**: warm near-black neutrals (dark is the designed theme; light is
  a derived, contrast-audited inversion — not designer-specified as its own
  thing). **One** coral-pink accent (`--primary`/`--positive`) for all
  positive/primary meaning, a cool blue (`--secondary`/`--negative`) for
  negative/secondary meaning, and a purple (`--highlight`) **reserved
  strictly for projected/future data** — never used for actual results.
  Every color is an HSL custom property consumed via `hsl(var(--x))`; raw
  Tailwind palette classes (`text-red-600`) are not used anywhere.
- **Type**: three families, one job each — `font-display` (Space Grotesk)
  for headings/nav/buttons/table column labels, `font-sans` (Inter) for
  body copy, `font-mono` (IBM Plex Mono) for **every single number** on the
  site (a hard, consistently-applied rule — ~90 call sites).
- **No gradients, no shadows, no CSS transitions, no animations** (loading
  spinners are the one exception). This is deliberate and should hold on
  mobile too — don't introduce motion to "make it feel native."
- **No icon set in table headers or sort indicators** — sort direction is a
  typographic glyph (▲ ▼ — ↑ →), not a lucide icon. Icons appear elsewhere
  (nav hamburger, theme toggle, thumbs up/down) but tables stay
  icon-free/glyph-only.
- `--radius: 0.5rem` — moderate rounding, pill shape (`rounded-pill`) for
  nav/segmented controls/badges only.
- **No brand identity beyond a 4-square flat mark** (`BrandMark` in
  `App.tsx`) + a text wordmark. No illustration, no photography except
  player headshots.

## Current mobile/responsive state — the honest baseline

Almost none of this app has been designed for mobile; a handful of ad hoc
Tailwind breakpoint tweaks exist, listed exhaustively below so nothing gets
assumed that isn't there:

- **Top nav** (`App.tsx`) already collapses at `md`: below that, the pill
  nav/theme toggle/login state disappear behind a hamburger button that
  opens a stacked full-width panel. This is the **one genuinely
  mobile-designed piece of UI in the app today** — worth looking at as the
  existing pattern to extend, not replace.
- `divergences/index.tsx`, `statistics/index.tsx`: page header row goes
  `flex-col` → `sm:flex-row` (title/controls stack on narrow screens).
- `player-detail/index.tsx`, `projection-detail/index.tsx`: a metadata grid
  goes `grid-cols-4` → `sm:grid-cols-6`, and a two-column info block goes
  `grid-cols-1` → `sm:grid-cols-2`.
- `wiki/index.tsx`: the left section-nav sidebar is `hidden` below `lg` —
  **currently just disappears with no mobile replacement** (no hamburger,
  no in-page jump menu).
- `league-detail/DraftTab.tsx`: header row and a settings summary chip stack
  at `lg`.
- `draft-prep/components/TeamPanel.tsx` + `draft-prep/index.tsx`: the one
  component with real responsive *behavior* (not just reflow) — on `lg`+ it's
  `fixed` to the right edge of the viewport as a docked panel with its own
  scroll; below `lg` it drops into normal flow and stacks under the board as
  a plain block. The page reserves margin for it (`lg:mr-[332px]` /
  `lg:mr-14` when collapsed) only above that breakpoint.
- **Everything else — every data table in the app — has zero mobile
  treatment.** Tables sit inside a plain `overflow-x-auto` wrapper, so on a
  phone they become a horizontally-scrolling strip with no column
  prioritization, no reflow, no card fallback. This is not a small gap —
  dense tables are the dominant UI pattern across essentially every screen.

## The central mobile design problem: dense tables

The overwhelming majority of this app's screen space is **sortable data
tables** — rosters, standings, rankings, projections, grades, draft boards,
matchup comparisons. Real column counts, current desktop build:

| Table | Columns | Where |
|---|---|---|
| Draft Prep board | 19 (with consensus columns on) | `/draft-prep` |
| Statistics — Projections view | ~21 | `/statistics?view=projections` |
| Statistics — Grades view | ~19 | `/statistics?view=grades` |
| League Players tab | ~19 | `/leagues/:id?tab=players` |
| Consensus Divergences | 15 | `/divergences` |
| League Draft tab (read-only board) | same shape as Draft Prep, no prep controls | `/leagues/:id?tab=draft` |
| Keeper draft results | 9 | `/leagues/:id?tab=draft&sub=keepers` |
| Standings | 8 | `/leagues/:id?tab=standings` |
| Team roster | 5–7 (Yahoo stat columns are dynamic, vary by league) | `/teams/:id` |
| Matchup team roster / category totals | 4 each | `/leagues/:leagueId/matchup/...` |

This is the single highest-leverage design problem to solve, and it needs
**one systematic answer that every table screen reuses** — not 10 different
one-off mobile layouts. Whatever pattern Claude Design proposes (an
expandable-card-per-row layout, a fixed leading column + horizontal scroll
for the rest, a column-priority system with a "more" expansion, a
completely different information architecture for narrow screens, etc.) is
expected to become the **one shared mobile table primitive**, analogous to
how `table-helpers.tsx` (`SortableHead`, `PlayerCell`, `ClickableRow`,
`HeaderRow`) is the one shared desktop table primitive today. Solve it once,
well, generically — every table below should visibly use the same answer.

A secondary, related problem: **column priority varies by table and by
role.** For example on the Draft Prep board, `Player`/`Auction $`/`Tier` are
load-bearing during a live draft, while `Confidence`/`Profile`/`Trend` are
research-phase, lower-priority-on-a-phone columns. Design should account for
"what are the 3–4 columns a user actually needs at a glance on a phone" per
table, not just uniformly truncate.

## Touch-interaction gaps to design around

The desktop build leans on a few interaction patterns that have **no touch
equivalent today** — these need explicit mobile designs, not just smaller
versions of the same control:

1. **Hover-only tooltips.** `HeaderTip` (in `table-helpers.tsx`) shows
   column explanations on hover, used across nearly every table header.
   Touch has no hover. Needs a tap-to-reveal pattern (or a persistent
   glossary/legend surface) that works the same way everywhere it's used.
2. **Drag-and-drop reordering.** The Draft Prep board supports dragging a
   player to a new rank *and* dropping it on a tier-boundary divider to
   re-tier it (top half of the divider = join the tier above, bottom half =
   join the tier below). This is built on the native HTML5 Drag and Drop
   API, which **does not fire on touch devices at all** — it's a
   mouse-only browser API. On a phone today this feature is silently
   inert. A ▲/▼ nudge-by-one-row fallback already exists in the same UI
   (small up/down arrows next to the rank number) and works via plain
   `onClick`, so it isn't *completely* broken — but the flagship drag
   interaction (including the "drop on a tier boundary" tier-reassignment,
   which has no non-drag equivalent at all right now) needs a touch-native
   design: long-press-and-drag, a "move to position" input, an explicit
   "set tier" action separate from reordering, etc.
3. **Row hover states.** `ClickableRow` (whole-row navigation) and general
   row highlighting rely on `hover:bg-muted/30`. On touch this just doesn't
   fire before tap — not a blocker (tap still navigates), but any design
   that leans on hover to reveal per-row actions (e.g. Draft Prep's Note
   field, +/− interest thumbs, planned-cost `+` button, which currently sit
   inline and only need `onClick`, not hover) should confirm those affordances
   stay visibly tappable without a hover reveal step.
4. **Inline-editable table cells.** Draft Prep's Tier column and Note column
   are small always-visible `<input>` elements inside table cells (narrow —
   the tier field is `w-7`, ~28px). That's a small tap target on mobile;
   needs a mobile-appropriate control (larger tap target, possibly a
   bottom-sheet/modal editor rather than an inline cell).

## Full screen inventory

Every route in the app, grouped by nav section. Nav itself: **Leagues |
Draft Prep | Statistics | Wiki**, plus the player-detail and matchup-detail
drill-down pages that aren't in the nav directly.

### Leagues section

> **Out of scope for now:** the backend recently gained a "native league"
> concept (leagues that live entirely in this app's own DB rather than
> syncing from Yahoo — creation, settings, and team CRUD, all
> commissioner-gated) — `POST /api/leagues`, `GetLeagueSettings`,
> `UpdateLeagueSettings`, `CreateLeagueTeam`, etc. **There is no frontend for
> any of it yet** — `createLeague` exists in `api/client.ts` but nothing
> calls it. Don't design mobile mockups for league creation/settings/roster
> management yet; there's no desktop version to extend, so there's nothing
> to optimize. Flagging it here only so it isn't mistaken for a missed
> screen — it'll need its own (desktop-first) design pass before a mobile
> one makes sense.

- **`/` — Home.** Hero + CTA (logged out) or, logged in: your synced
  leagues list + "Player Outlooks" signal cards (top consensus divergences,
  with filter chips: All moves / We're higher / We're lower / Has news).
  List-of-cards layout already, probably the least table-heavy screen —
  still needs a mobile pass on card density and the filter-chip row.
- **`/leagues/:id` — League detail.** Tabbed: **My Team** (only if the
  signed-in user owns a team in this league) / Standings / Scoreboard /
  Players / Draft. Tab bar itself is a segmented control — needs a mobile
  treatment (horizontal scroll? dropdown? stacked?) once it doesn't fit one
  row on a phone.
  - **My Team / `/teams/:id` (same `TeamPanel` component, shared)**:
    matchup summary card, roster table (5–7 dynamic Yahoo stat columns),
    stat-period switcher (This week / Last week / Season / a specific date).
  - **Standings**: 8-column ranked table.
  - **Scoreboard**: week-by-week matchup cards, one per team pairing, with a
    week selector.
  - **Players**: ~19-column searchable/filterable roster+free-agent table.
  - **Draft** (`DraftSection` wraps two sub-tabs, NFL-only shows both):
    - **Draft Values**: read-only version of the Draft Prep board (same
      table component, no editing controls) — position filter only.
    - **Keepers**: keeper rules editor + team selector + draft-results table
      (9 columns) + wishlist checkboxes. Commissioner view is a distinct,
      richer variant (`CommissionerKeeperView.tsx`) — check both.
- **`/leagues/:leagueId/matchup/:week/:t1/:t2` — Matchup detail.**
  Head-to-head: category totals table (4 cols) + two team roster tables (4
  cols each) side by side on desktop — this side-by-side layout is an
  obvious stacking candidate on mobile.

### Draft Prep section

- **`/draft-prep`.** The single most feature-dense screen in the app:
  - League picker, position filter chips (All/QB/RB/WR/TE/K), view filter
    chips (All players/Targets/Avoids).
  - Collapsible **Draft Settings panel** — a form: teams, budget, scoring
    format (segmented control), roster slots (numeric steppers per slot
    including QB/RB/WR/TE/FLEX/SFLEX/K/DEF), and a 9-row "pointing system"
    editor (per-stat-category point values). This is the most complex form
    in the app and has no mobile design today.
  - **The 19-column board table** (see table above) — target/avoid thumbs,
    planned-cost `+`/editable field, inline note field, drag handle + rank
    nudge arrows, editable tier field, tier-divider rows (which are
    themselves drop targets, see touch-interaction gaps above).
  - **Team panel** (fixed-dock on desktop, see responsive state above) — two
    modes (Team: assembled roster from planned prices; Targets: grouped by
    position then tier), collapsible to a slim tab.
- Also reachable from here: clicking any player row goes to `/players/:gsisId`.

### Statistics section

- **`/statistics?view=projections|grades`.** One page, a view toggle
  (segmented control) between two ~20-column tables (comp-based projections
  vs. real-life grades), a Year selector (Grades view only — projections are
  pinned to next season), position filter.
- **`/divergences`.** Full 15-column table: our projection rank vs.
  consensus rank/ADP, by position, with inline situational notes
  (injuries/camp battles/depth-chart news) and single-source-flag markers.
  Linked from Home's Player Outlooks cards ("See all divergences →").
- **`/players/:gsisId` — Player detail.** Metadata header (name, team, age,
  physical stats — the `grid-cols-4`/`sm:grid-cols-6` grid mentioned above),
  a Grade card (large percentile number + sub-scores + trend), a
  year-over-year stats table, a projection block with a PPR/Half/Standard
  toggle and a comps section (similar historical players + trajectory
  chart), and situational notes if any exist for the player/team.

### Wiki section

- **`/wiki`.** Static, no data fetching — a long-form reference page
  explaining the stat engine (grades, projections, VORP/rankings, auction
  pricing, tiering, consensus divergence) in prose with a handful of small
  worked-example tables (4–5 cols each) and diagrams. Two-column layout on
  desktop: a `lg`-only left section-nav + main content. **The section-nav
  currently just vanishes below `lg` with no replacement** — needs an
  in-page nav pattern for mobile (jump links, a collapsible TOC, etc.), plus
  a general reading-width pass since this is the one prose-heavy page in an
  otherwise data-dense app.

## Deliverables checklist

Design the following as concrete mobile mockups/specs (phone width — treat
`sm`/375–428px as primary target, note behavior at `md`/tablet where it
meaningfully differs from both phone and current desktop). Ordered roughly
by how central each is to the app's core loop:

**P0 — core loop, needed first**
1. The shared mobile table pattern (see "central mobile design problem"
   above) — this one decision underpins everything else on this list.
2. Draft Prep board (`/draft-prep`) including: tier dividers, inline note
   editing, planned-cost editing, and a touch-native reorder/re-tier
   interaction to replace drag-and-drop.
3. Draft Settings panel (the form inside Draft Prep).
4. Team panel (Draft Prep's docked panel, both Team and Targets modes) —
   what replaces "fixed dock" on a phone.
5. League Players tab + Statistics (Projections/Grades) — these three share
   the same table-heavy shape and should visibly reuse the P0-1 pattern.

**P1 — secondary flows**
6. Home (leagues list + Player Outlooks cards).
7. League detail tab bar (the mobile treatment for 5 tabs that don't fit
   one row) + Standings + Scoreboard.
8. My Team / Team detail (roster table + matchup card + period switcher).
9. Matchup detail (category totals + two roster tables — a stacking
   candidate).
10. Player detail (metadata grid, grade card, YoY table, projection +
    comps).
11. Consensus Divergences table + situational notes.
12. Keepers (rules editor, draft-results table, commissioner view).

**P2 — lower traffic, still needs an answer**
13. Wiki (mobile nav replacement for the vanishing sidebar + reading width).
14. Top nav — confirm/extend the existing hamburger pattern rather than
    redesign it; call out anything it's currently missing (e.g. no active
    "you are here" affordance beyond a background fill that may be too
    subtle at mobile touch-target sizes).

## Constraints / guardrails for Claude Design

- **Reuse the existing token system.** Every color must map to an existing
  `--token` (or a clearly-flagged *new* token proposal, called out
  explicitly, not silently introduced) — no new hex values, no raw Tailwind
  palette classes.
- **No new component library.** Mobile patterns should be expressed as new
  variants/compositions of the existing primitives
  (`table`, `table-helpers`, `badge`, `button`, `tabs`) or as clearly-scoped
  new primitives in that same style — not an import of a mobile UI kit.
- Keep the **no gradients / no shadows / no CSS transitions / no animations**
  rule (loading spinners excepted). A mobile redesign is not license to add
  motion polish this app has deliberately avoided everywhere else.
- Keep **mono for every number**, **display font for headings/labels**,
  **sans for body** — don't introduce a fourth typeface for mobile.
- Keep the **typographic sort-glyph convention** in tables (▲ ▼ — ↑ →) — no
  icon-based sort indicators, on any breakpoint.
- Purple stays **reserved for projected/future data only** — don't reach for
  it as a generic "third color" for mobile-only UI (e.g. a tab bar accent).
- Respect the existing breakpoint vocabulary (Tailwind defaults — `sm` 640,
  `md` 768, `lg` 1024, `xl` 1280) rather than inventing new device-based
  breakpoints; call out explicitly which existing breakpoint each design
  targets so implementation can match by number, not eyeball it.

## What the handback should include (for the executing session)

To turn designs back into code efficiently without another research round
trip, please ensure the handback ties every mockup to:

- The **exact route and component** it replaces or extends (use the names
  in this doc — e.g. "this is `DraftBoardTable.tsx` at `sm`", not "the
  draft board").
- Which **existing columns/fields are kept, hidden-by-default, or moved into
  an expansion** for any table redesign — implementation needs the mapping,
  not just the visual result.
- Explicit interaction specs for anything replacing a desktop-only pattern
  (hover tooltips, drag-and-drop) — described precisely enough to build
  without guessing (e.g. "tap the column label to open a bottom sheet with
  the tooltip text" vs. "tap-and-hold").
- Any newly-proposed token, spacing scale, or breakpoint, flagged as new
  rather than left implicit.
