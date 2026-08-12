# Design System Migration — Fantasy Sports

> **Status (2026-08-09):** Phases 1, 2, and 3a–3b are implemented. Remaining:
> 3c/3e (both blocked on open decisions below), 3d (blocked on Phase 4 data),
> and Phase 4 itself.
>
> **Decisions made:** light theme is derived, not dropped (option B). The full IA
> consolidation *is* being adopted — 2-item nav, Rankings+Projections merged into
> Statistics, Divergences moved to Home as signal cards, Home merged with Leagues.
> That work is all Phase 3.

Adopting the `design_handoff_fantasy_sports_app` bundle (warm-dark palette, three-font
system, table-first layout) into the existing React/Vite/Tailwind/shadcn frontend.

Source of truth: `~/Downloads/design_handoff_fantasy_sports_app/` (README.md + prototype +
design-system/tokens).

---

## Guiding constraints

1. **Keep the existing token architecture.** `tailwind.config.js` consumes HSL triplets via
   `hsl(var(--x))`. Converting the design's hex tokens to HSL triplets means the Tailwind
   config shape is untouched and every existing `bg-background` / `text-muted-foreground`
   class keeps working. Do *not* switch to raw hex custom properties.
2. **Retheme before restructuring.** Phases 1–2 change only colors/type and are reversible.
   Phases 3+ move screens around.
3. **No new component library.** Translate the handoff's inline-style JSX components into
   the existing shadcn primitives (`components/ui/*`), not alongside them.

---

## Phase 1 — Token layer

**Files:** `src/index.css`, `tailwind.config.js`, `index.html`

### 1a. Palette

Replace the `.dark` block in `src/index.css` with the handoff's warm neutrals. Converted
values (hex → HSL triplet):

| Design token | Hex | HSL triplet | Maps to |
|---|---|---|---|
| `--n-950` | `#15130f` | `40 17% 7%` | `--background` |
| `--n-900` | `#1c1913` | `40 19% 9%` | `--card`, `--popover`, table header bg, hover bg |
| `--n-850` | `#231f18` | `38 19% 12%` | `--muted` (chip/track/avatar bg) |
| `--n-800` | `#2b2620` | `33 15% 15%` | `--border`, `--input` |
| `--n-700` | `#3a352c` | `39 14% 20%` | emphasis border |
| `--n-500` | `#7d7566` | `39 10% 45%` | `--muted-foreground` |
| `--n-450` | `#8a8272` | `40 10% 49%` | table column labels |
| `--n-400` | `#a89f8f` | `38 13% 61%` | secondary text |
| `--n-200` | `#dcd6c9` | `41 21% 83%` | table body text |
| `--n-100` | `#f0ece3` | `42 30% 92%` | `--foreground` |
| `--n-50` | `#faf7f1` | `40 47% 96%` | headings |
| `--accent-pink` | `#e2617d` | `347 69% 63%` | `--primary`, `--positive` |
| `--accent-pink-hover` | `#ef8098` | `347 78% 72%` | `--ring`, link hover |
| `--accent-pink-tint-bg` | `#2b1c20` | `344 21% 14%` | `--positive-light` |
| `--accent-pink-tint-border` | `#402530` | `336 27% 20%` | `--positive-border` |
| `--accent-blue` | `#7c93e0` | `226 62% 68%` | `--negative`, `--secondary` |
| `--accent-blue-tint-bg` | `#191d29` | `225 24% 13%` | `--negative-light` |
| `--accent-purple` | `#9a7cf0` | `256 79% 71%` | `--highlight`, `--chart-line` (projected data only) |

**Semantic remap — the risky part.** Today `positive` = teal, `negative` = orange. The
design uses pink = positive/primary and blue = negative. That collapses two currently
distinct roles (`primary` and `positive`) onto one hue, and inverts the warm/cool
convention. Every `text-positive` / `text-negative` usage needs a read-through, not a
blind find-replace — particularly:

- `src/lib/utils.ts` — `zScoreColor`, `zScoreIndicator`
- `src/lib/grades.ts` — `gradeColorClass`, `trendIndicator`, `phaseColor`
- `src/pages/divergences/components/DeltaBadge.tsx`
- `src/pages/projections/components/ConfidenceBadge.tsx`, `UniquenessBadge.tsx`

`--warning` has no counterpart in the handoff. Keep the existing orange as an
unrebranded escape hatch, or drop it if unused — audit first.

### 1b. Typography

- `index.html`: add the Google Fonts link for Space Grotesk (500/600/700), Inter
  (400/500/600/700), IBM Plex Mono (400/500/600). Keep `preconnect`.
- `tailwind.config.js`: extend `fontFamily` with
  `display: ['Space Grotesk', ...]` and `mono: ['IBM Plex Mono', ...]`; `sans` stays Inter.
- The "every number is mono" rule is the widest-reaching change in the bundle. It touches
  every stat cell in every table. Implement it at the primitive level — `TableCell` variants
  and `ZScoreCell` in `table-helpers.tsx` — rather than sprinkling `font-mono` per call site.

### 1c. Motion

The design specifies no transitions or animations anywhere. `transition-colors` appears
throughout `App.tsx` and the page components. Strip it globally rather than per-file.
The existing `prefers-reduced-motion` block in `index.css` becomes redundant but is
harmless to keep.

### Open decision — light theme

The handoff defines **only** a dark palette, yet its nav includes a ☀ toggle. The app
currently ships a working light theme (`:root` block, `provider.tsx`). Two paths:

- **(A) Dark-only.** Delete the light block, drop the toggle, remove `provider.tsx`'s theme
  switching. Truest to the handoff, smallest surface, loses a working feature.
- **(B) Derive a light palette.** Invert the warm neutral ramp (`--n-50` → background,
  `--n-950` → foreground) and darken the pink/blue accents for contrast on light. Keeps the
  toggle honest. Not designer-specified, so it's our invention and will need a contrast pass.

**Recommendation: (B).** The ramp is symmetric enough to invert cleanly, and removing a
shipped feature to match a prototype that still draws the toggle is the wrong trade.

---

## Phase 2 — Core primitives

Translate the six handoff components into the existing `components/ui/` files. None of
these are new files; all are edits to existing primitives.

| Handoff component | Target | Change |
|---|---|---|
| `DataTable` | `ui/table.tsx`, `ui/table-helpers.tsx` | Header row bg `--card`, 11px/600 Space Grotesk uppercase labels in `--text-label`; body rows borderless with 1px hairline separators; 28px gutter padding on first/last cells |
| `Badge` | `ui/badge.tsx` | Add `pink` / `blue` / `neutral` tonal variants — tinted bg + accent text + optional tinted border. Pill radius 16px. Never solid-fill |
| `Button` | `ui/button.tsx` | `primary` = solid pink, dark text, 6px radius; `text` = pink label, no bg; `secondary` = elevated bg. Space Grotesk 600/13px. Remove press/scale effects |
| `SegmentedControl` | `ui/tabs.tsx` | Pill track on `--muted`, active segment solid pink w/ dark text. Two shapes: pill (nav) and 6px-radius rect (sub-tabs, view toggles) |
| `ListRow` | **new** `ui/list-row.tsx` | Bordered 10px-radius row: leading slot / title+subtitle / right-aligned trailing+trailingSub. Used by leagues list, signal cards, transactions |
| `GradeBadge` | `pages/player-detail/components/GradeCard.tsx` | 34px mono pink grade number + label + YoY stat, in a pink-tinted cell inside a 12px-radius pink-bordered container, beside a metrics list |

Also needed: a flat circular **avatar placeholder** (`--muted` bg, no border) at 28/32/72/84px.
`PlayerAvatar` in `table-helpers.tsx` already does this — extend its size union.

**Sort indicators.** The design appends ▲/▼ glyphs to the header label text. Current code
uses lucide `ChevronUp`/`ChevronDown`/`ChevronsUpDown` icons in `SortableHead`. The handoff
uses no icon set at all — typographic glyphs only. Switching means dropping lucide from
table headers.

---

## Phase 3 — Screen restructure

This is where the handoff diverges from the current IA, and it is a larger change than
Phases 1–2 combined.

### 3a. Navigation: 4 items → 2

Design nav is **Leagues | Statistics**. Current nav is Leagues / Rankings / Projections /
Divergences. The handoff folds three top-level pages into one:

- `/rankings` (grades) + `/projections` (comp projections) become **one** Statistics table
  with a This view / Traditional stats toggle and a Year dropdown.
- `/divergences` stops being a page and becomes the **"Player Outlooks"** section on Home
  (signal cards with filter chips: All moves / We're higher / We're lower / Has news).

Keep the old routes as redirects so existing links don't break.

### 3b. Home absorbs Leagues

Design's home = hero headline + CTAs + "Your leagues" list + Player Outlooks. Currently
`Home.tsx` (32 lines) and `Leagues.tsx` (72 lines) are separate. Merge into one page;
`/leagues` redirects to `/`.

### 3c. League tabs

Design: Standings / Scoreboard / Players / Draft. Current adds a **Keepers** tab; the design
folds keepers into the Draft tab as a second table ("Draft Prep" + "Keepers"). The existing
`KeepersTab.tsx` (143 lines) plus `CommissionerKeeperView.tsx` and `KeeperDraftTable.tsx`
(201 lines each) are substantially richer than the design's 4-column keepers table —
**the design under-specifies this area.** Recommend keeping the Keepers tab as-is and
retheming it rather than deleting working functionality to match a prototype that never
modelled it.

`RankingsTab.tsx` (389 lines) is already marked absorbed into Players tab per CLAUDE.md —
verify it's dead and delete if so.

### 3d. New screen: league-context player page

The one genuinely net-new screen (`isLeaguePlayer`). Distinct from `/players/:gsisId`:
ownership status badge, roster slot, bye week, rest-of-season projection, acquired-via,
avg pts/g (L6), a week-by-week table with inline trend bars, and add/drop + propose-trade
actions.

**Blocked on data** — see below.

### 3e. Team roster simplification

Design shows PLAYER · POS · NFL · GRADE · PROJ PTS. Current `RosterTable.tsx` renders
dynamic Yahoo stat columns plus the Value column. The design is a significant reduction in
information density. Flag before implementing — this may be a case where the prototype
simply didn't model the live-stats use case.

---

## Phase 4 — Data gaps

These block parts of Phase 3. All need backend work; none are frontend-only.

| Need | Screen | Status |
|---|---|---|
| Transactions feed (team, action, player, timestamp) | League → Standings | **No endpoint.** Yahoo has a transactions API; not yet wired |
| Bye week | League player page | Not in `NFLPlayerMeta` |
| Acquired-via ("Drafted R1", "Added 2h ago") | League player page | Not exposed; derivable from draft results + transactions |
| Weekly points, last 6 | League player page | `nfl_player_stats` has weekly rows; needs a new endpoint or roster-response extension |
| ADP | League → Draft Prep | Exists in `nfl_consensus_rankings`; not exposed on any draft endpoint |
| Roster slot ("Starter (RB1)"/"Bench") | League player page | Available from Yahoo roster response; not surfaced in `RosterPlayer` |
| Per-stat projections (yds/td/rec/gp) | Statistics → Traditional view | `ProjPlayerListItem` has only `proj_fpts*`; needs new fields on `GET /api/projections` |

Already available, no backend work: grades + Production/Efficiency/Usage/Durability
sub-scores, comps with per-season stat lines, career trajectory, confidence, consensus
delta, situational notes, `tags`.

---

## Suggested sequencing

1. **Phase 1** — tokens, fonts, motion. Whole app rethemes at once; visually verifiable
   immediately; fully reversible.
2. **Phase 2** — primitives. Tables and badges land the look on every screen.
3. **Phase 3a/3b** — nav + home merge. Highest-visibility structural change.
4. **Phase 4** — backend endpoints for the gaps, in parallel with 3c/3e retheming.
5. **Phase 3d** — league player page, once its data exists.

Phases 1–2 are the bulk of the visual payoff and carry the least risk. Phase 3 should not
start until the two open decisions below are settled.

---

## Open decisions

Settled:

1. **Light theme** — ✅ derived (option B). `:root` now holds an inverted warm ramp with
   darkened accents. Needs a contrast audit before it can be called done.
2. **IA consolidation** — ✅ adopt the design's 2-item nav in full (Phase 3).

Still open, both flagged because the design under-specifies working functionality:

3. **Keepers tab** — keep it (recommendation) or fold into Draft as designed? The existing
   `KeepersTab` + `CommissionerKeeperView` + `KeeperDraftTable` do far more than the
   prototype's 4-column table.
4. **Team roster columns** — reduce to the design's fixed 5, or keep the dynamic Yahoo stat
   columns? The prototype has no live in-season roster view to model.

---

## Implementation notes (Phases 1–2, done)

- **`--warning` is effectively retired.** It has no counterpart in the design's three-hue
  system. Every former call site now maps to pink (positive) or blue (negative); the token
  is still defined as an escape hatch but nothing consumes it.
- **All raw Tailwind palette colors are gone.** ~30 instances of `text-red-600`,
  `bg-green-100`, `text-amber-400`, `bg-purple-600` etc. bypassed the token system entirely
  and would have read as off-brand. Errors → `destructive`, positive values/top tiers →
  `positive`, cautions → `negative`, projection confidence bars → a `positive` intensity ramp.
- **Purple is now genuinely projected-data-only.** It was previously used for z-score
  intensity, grade tiers 70–89, player tags, and confidence bars. Those all moved to the
  pink/blue axis; purple survives only on the projected season row, projected points card,
  and the projected chart segment.
- **Mono numerals** were applied by targeting the existing `tabular-nums` marker, which this
  codebase already used consistently on numeric cells — 88 sites, all now `font-mono`.
- **`--tabs-height`** is hardcoded at `league-detail/index.tsx` for the sticky table-header
  offset; it moved 49px → 64px when tabs became a segmented control. If the segmented
  control's padding changes again, that constant must follow.
- **Deliberate deviation:** the design puts a 28px gutter *inside* first/last table cells so
  tables read full-bleed. The current pages wrap tables in padded containers, so cell padding
  was left alone — revisit as part of the Phase 3 page shell.
### Phase 3a–3b notes

- **`/divergences` was kept as a deep page**, not deleted. The design turns it into Home
  signal cards only, but the full table carries situational notes and source counts the
  cards can't. Home shows the top 6 and links out with "See all divergences →". It's out of
  the nav, so the design's 2-item nav is intact.
- **The "Traditional stats" view could not be built.** `ProjPlayerListItem` exposes only
  `proj_fpts*` — no yds/td/rec/gp. The design's Statistics toggle swaps the last four columns
  for those, which needs per-stat fields on `GET /api/projections`. Added to Phase 4. The
  view toggle currently reads Projections / Grades instead.
- **Year and view are coupled deliberately.** Projections only exist for `PROJECTION_SEASON`
  and grades only for completed seasons, so the Year control is scoped to Grades view rather
  than offering year×view combinations with no data behind them.
- **Deleted as dead:** `pages/Leagues.tsx`, `pages/rankings/index.tsx`,
  `pages/projections/index.tsx`, `league-detail/RankingsTab.tsx` (389 lines, already
  superseded per CLAUDE.md) and its now-orphaned `league-detail/hooks/useRankings.ts`.
  The projections/rankings *hooks and components* survive — `statistics/` consumes them.

- **Naming friction to revisit:** the token family is `positive`/`negative`, but the design's
  blue also carries neutral-informational meaning (tags, Free Agent status), not just
  "negative". A rename to something like `accent-primary`/`accent-secondary` would read
  better. Deferred — it touches every call site.
