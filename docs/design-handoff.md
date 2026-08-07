# Design Handoff — Fantasy Sports App

This doc exists to give a design-focused session enough context to work on this
app's UI without re-deriving it from the codebase. It describes **what exists
today** — current IA, visual system, component vocabulary, and honest gaps —
not a wishlist. Pair it with whatever specific ask prompted this handoff (a
page redesign, a new feature's UI, a full visual refresh, etc.).

## What this product is

A full-stack fantasy sports companion app (NFL primary, NBA supported) for
people who play fantasy football/basketball in Yahoo leagues. Users log in
with Yahoo OAuth, sync their leagues, and get two categories of value:

1. **Live league data**, pulled straight from Yahoo: rosters, scoreboards,
   standings, matchups, keeper management, draft results.
2. **Original analysis Yahoo doesn't provide**: a comp-based NFL stat
   projection engine (finds statistically similar historical players and
   projects growth), real-life player grades (0–100 percentile scores
   independent of fantasy value), VORP-based rankings and draft-value
   calculators, and (newest) a diagnostic layer comparing our projections
   against external consensus rankings.

The analysis side is the more differentiated, "why would someone use this
instead of just ESPN/Yahoo" part of the product, and it's also the more
data-dense part of the UI — lots of numeric columns, comparison tables,
percentile/z-score coloring, not much narrative UI at all.

**Audience**: fantasy football players doing draft prep and in-season
roster management. Assume moderate-to-high domain literacy (they know what
VORP, WOPR, and target share mean) — this is a power-user tool, not a
mass-market consumer app. No mobile app; responsive-ish web only, primarily
used on desktop during draft prep season.

## Current tech stack (constraints for any design work)

- React 18 + Vite 5 + TypeScript, React Router v6 (client-side routing, no SSR)
- Tailwind CSS 3 + `shadcn/ui` (Radix-based) + `class-variance-authority`
- TanStack Query v5 for all data fetching (30s stale time)
- `lucide-react` for icons
- Any design direction should assume it's implemented as Tailwind
  utility classes + shadcn component variants, not a separate design-token
  pipeline — there's no Figma-to-code sync or design-system package.

## Information architecture (all current routes)

```
Public (no login required):
  /                          Home — login CTA or "go to your leagues" for logged-in users
  /leagues                   List of the user's synced Yahoo leagues
  /leagues/:id               League detail — tabbed: Standings / Scoreboard / Players / Keepers / Rankings / Draft (NFL only)
  /teams/:id                 Team roster detail
  /leagues/:leagueId/matchup/:week/:t1/:t2   Head-to-head matchup detail
  /rankings                  Standalone real-life player grades (0-100), no login, no Yahoo — Flex/Superflex filters
  /projections               Comp-based projection rankings, no login — PPR/Half/Standard toggle
  /players/:gsisId           Unified player detail — metadata, grade, year-over-year stats, projection + comps
```

Nav bar (top, sticky): logo-less text wordmark "Fantasy Sports" + 3 links
(Leagues / Rankings / Projections) + theme toggle + login state, all in one
horizontal row. That's the entire global navigation — no sidebar, no
breadcrumbs, no search.

## Current visual system

**Color**: HSL CSS custom properties, full light/dark pairs, class-based dark
mode (`.dark` on root, toggled by the sun/moon button in nav, persisted via
`useTheme`). Palette:

- **Primary**: teal (`hsl(170 90% 35%)` light / `hsl(170 90% 50%)` dark) — used for links, primary buttons, active nav state, focus rings.
- **Secondary/Accent**: purple (`hsl(270 80% 58%)` light / `hsl(270 80% 65%)` dark) — used sparingly, mostly in the "highlight" semantic color and chart lines.
- **Semantic stat colors** (this is the most distinctive part of the system): four named pairs — `positive` (teal-green), `highlight` (purple), `warning` (orange), `negative` (red-orange) — each with a `DEFAULT`/`foreground`/`light`/`border` set, used to color-code z-scores, grades, and confidence indicators throughout the data tables.
- Neutral grays for background/card/border/muted, standard shadcn shape.
- `--radius: 0.5rem` (moderate rounding, not pill-shaped, not sharp).

**Typography**: Inter, system fallback stack. No display/serif pairing — one
typeface throughout.

**No brand identity beyond color.** No logo/icon (just a text wordmark), no
illustration, no photography except player headshots (from nflverse). This
reads as a functional internal tool, not a consumer product — worth naming
explicitly since it's probably one of the things a design pass would most
want to address if "make it feel more like a product" is the goal.

## Dominant UI pattern: dense data tables

The overwhelming majority of screen real estate across this app is **sortable
data tables** — rosters, rankings, projections, standings, draft boards,
grades. Shared table primitives already exist and are used consistently
(`src/components/ui/table-helpers.tsx`):

- `SortableHead` — clickable column header with chevron sort indicator
- `PlayerCell` — round headshot (28px, 32px for emphasis) + name + optional subtitle (team), name goes `hover:text-primary` when the row is a link
- `ClickableRow` — whole-row navigation with keyboard support (Enter/Space), `hover:bg-muted/30`
- `ZScoreCell` — right-aligned numeric cell with a small ▲▲/▲/▼/▼▼ glyph colored by the `highlight` semantic scale (`src/lib/utils.ts: zScoreIndicator/zScoreColor`) — this is the app's primary way of conveying "how good/bad is this number" at a glance across dozens of stat columns
- `HeaderRow` — consistent card-colored header background with rounded top corners

Grades get their own small color/label utility (`src/lib/grades.ts`):
`gradeColorClass`, `trendIndicator`, `phaseLabel`, `phaseColor` — same idea
(percentile → color) applied to the 0–100 grade scale instead of z-scores.

**Any design work on this app has to reckon with this pattern seriously** —
it's not a UI that can be redesigned into card grids or heavy visual
storytelling without losing the actual value (scanning 15+ numeric columns
across 50+ players quickly). The design opportunity here is more about
information hierarchy, scannability, and making the color-coding system
legible and consistent than about departing from tables as the core object.

## Component inventory (shadcn primitives currently installed)

Only 7 files in `src/components/ui/`: `badge`, `button`, `input`, `provider`
(theme context), `table-helpers`, `table`, `tabs`. This is a **minimal**
shadcn install — no dialog/modal, no dropdown menu, no toast/sonner, no
select, no card component, no tooltip. Badge variants: `default` (teal),
`secondary` (purple), `destructive` (red), `outline`. Any new UI pattern
(modals, toasts, comboboxes, popovers) needs its primitive added from
scratch via the shadcn CLI — nothing to repurpose today.

## Known gaps / where design attention would matter most

Said plainly, based on what's actually in the code (not the user's stated
priorities — confirm with them what this handoff is actually for):

1. **No visual identity.** Text wordmark, no icon/logomark, no distinct
   personality beyond the teal/purple palette. If the ask is "make this feel
   like a real product," this is the highest-leverage gap.
2. **Home page is a stub** (`Home.tsx` is ~30 lines, just a login CTA) — no
   onboarding, no explanation of what the analysis features are, nothing
   that would help a new user understand why the projections/grades/rankings
   tools are valuable before they've synced a league.
3. **Nav is minimal to the point of being easy to miss** — no active-state
   affordance beyond a font-weight/color change, no grouping between
   "your league data" (Leagues) and "our original analysis" (Rankings,
   Projections), even though those are conceptually pretty different parts
   of the product.
4. **The z-score/grade color-coding system, while functional, has no legend
   or onboarding anywhere** — a first-time user sees ▲▲ purple glyphs next to
   numbers with no explanation of what they mean.
5. **New this week, not yet in the UI at all**: the consensus-divergence
   diagnostic layer (comparing our projections against external expert
   rankings/ADP, surfacing situational news like injuries and camp battles)
   exists as backend data only — `nfl_consensus_rankings`,
   `nfl_player_situational_notes`, `nfl_projection_divergences` tables, no
   frontend yet. If this handoff is meant to help design that surface, there
   is no existing UI convention for "show a divergence between our number and
   the outside world" or "show a news/context item inline with a stat row" —
   that would be new design territory, not an extension of an existing pattern.

## Suggested angle for a design session

Given the above, a design pass on this app is more likely to be well-spent on
**information hierarchy and product identity** than on visual novelty — the
underlying interaction model (dense sortable tables, drill into a player
detail page) is probably right for the audience and shouldn't be discarded
wholesale. The open question worth deciding explicitly with whoever's doing
the design work: is this pass about (a) making the existing screens feel more
like a considered product (branding, home page, nav hierarchy, legends for
the color system), or (b) designing net-new UI for the consensus-divergence
data that has no frontend yet, or (c) both. That scope decision changes the
brief a lot and isn't something this doc can answer on its own.
