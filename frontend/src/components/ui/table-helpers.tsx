import * as React from 'react'
import { Link } from 'react-router-dom'
import { Newspaper } from 'lucide-react'
import { TableHead, TableRow, TableCell } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { zScoreIndicator, zScoreColor, cn } from '@/lib/utils'
import { NOTE_CATEGORY_LABELS, NOTE_DIRECTION_STYLES } from '@/lib/notes'
import type { SituationalNote } from '@/api/client'

// ── Sortable column header ──────────────────────────────────────────────────

type SortDir = 'asc' | 'desc'

interface SortableHeadProps {
  /** Key identifying this column in your sort state. */
  col: string
  /** Currently active sort column. */
  current: string
  /** Current sort direction. */
  dir: SortDir
  /** Called when the user clicks this column header. */
  onSort: (col: string) => void
  children: React.ReactNode
  className?: string
}

/** Sortable column header. The design system uses no icon set — direction is a
 *  typographic glyph appended to the label, and inactive columns show nothing. */
function SortableHead({ col, current, dir, onSort, children, className }: SortableHeadProps) {
  const active = col === current
  return (
    <TableHead
      className={`cursor-pointer select-none hover:text-foreground whitespace-nowrap ${className ?? ''}`}
      onClick={() => onSort(col)}
      aria-sort={active ? (dir === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      <div className="flex items-center gap-1">
        <span>{children}</span>
        {active && (
          <span aria-hidden className="text-primary text-[9px] leading-none">
            {dir === 'desc' ? '▼' : '▲'}
          </span>
        )}
      </div>
    </TableHead>
  )
}

interface HeaderTipProps {
  children: React.ReactNode
  /** Explanation shown on hover/focus. Omit to render the label with no tooltip. */
  description?: string
}

/**
 * Wraps a column-header label with a hover/focus tooltip explaining the stat.
 * A dotted underline is the only affordance — the design system has no icon set
 * for table headers, so discoverability is typographic, not iconographic.
 */
function HeaderTip({ children, description }: HeaderTipProps) {
  if (!description) return <>{children}</>
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          className="cursor-help border-b border-dotted border-muted-foreground/50 outline-none"
        >
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent>{description}</TooltipContent>
    </Tooltip>
  )
}

/**
 * Mobile counterpart to `HeaderTip` — hover isn't reachable on touch, so this
 * opens the same flat, bordered `TooltipContent` styling on tap instead, and
 * closes on an outside tap via a full-screen invisible catcher.
 */
function MobileHeaderTip({ children, description }: HeaderTipProps) {
  const [open, setOpen] = React.useState(false)
  if (!description) return <>{children}</>
  return (
    <span className="relative inline-block">
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          setOpen((o) => !o)
        }}
        className="cursor-help border-b border-dotted border-muted-foreground/50 outline-none"
      >
        {children}
      </button>
      {open && (
        <>
          <span className="fixed inset-0 z-40" onClick={() => setOpen(false)} aria-hidden="true" />
          <span className="absolute left-0 top-full z-50 mt-1 block w-64 rounded-md border border-border bg-popover px-2.5 py-1.5 font-sans text-xs font-normal normal-case leading-snug tracking-normal text-popover-foreground">
            {description}
          </span>
        </>
      )}
    </span>
  )
}

/**
 * Standard toggle handler for sortable columns.
 * If clicking the active column, flip direction. Otherwise switch to the new column
 * with a sensible default direction — ascending for columns where a lower value
 * reads first (names, ranks, tiers: rank 1 / tier 1 is the *best*, not the least),
 * descending everywhere else (points, grades, dollars — biggest first). `ascCols`
 * names which columns are ascending-first; pass an array for a fixed set, or a
 * predicate when the direction depends on data the table only has at render time
 * (e.g. a stat category's `sort_order`, where a counting stat like turnovers is
 * "lower is better").
 */
function useTableSort(
  defaultCol: string,
  defaultDir: SortDir = 'desc',
  ascCols: string[] | ((col: string) => boolean) = []
) {
  const [sortCol, setSortCol] = React.useState(defaultCol)
  const [sortDir, setSortDir] = React.useState<SortDir>(defaultDir)

  const isAscFirst = React.useCallback(
    (col: string) => (typeof ascCols === 'function' ? ascCols(col) : ascCols.includes(col)),
    [ascCols]
  )

  const handleSort = React.useCallback((col: string) => {
    setSortCol((prev) => {
      if (prev === col) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return prev
      }
      setSortDir(isAscFirst(col) ? 'asc' : 'desc')
      return col
    })
  }, [isAscFirst])

  return { sortCol, sortDir, handleSort } as const
}

// ── Player avatar ───────────────────────────────────────────────────────────

/** Avatar sizes used across the design: table rows (28/32), league player header
 *  (72), statistics player detail header (84). */
type AvatarSize = 28 | 32 | 40 | 72 | 84

const AVATAR_CLASS: Record<AvatarSize, string> = {
  28: 'h-7 w-7',
  32: 'h-8 w-8',
  40: 'h-10 w-10',
  72: 'h-[72px] w-[72px]',
  84: 'h-[84px] w-[84px]',
}

interface PlayerAvatarProps {
  src?: string | null
  alt: string
  /** Pixel size. Default 28. */
  size?: AvatarSize
}

/** Flat circular headshot with an elevated-surface fallback circle — no border,
 *  no ring, per the design system. */
function PlayerAvatar({ src, alt, size = 28 }: PlayerAvatarProps) {
  const cls = AVATAR_CLASS[size]
  if (src) {
    return (
      <img
        src={src}
        alt={alt}
        width={size}
        height={size}
        loading="lazy"
        decoding="async"
        className={`${cls} rounded-full object-cover bg-muted shrink-0`}
      />
    )
  }
  return <div className={`${cls} rounded-full bg-muted shrink-0`} />
}

const TEAM_AVATAR_TEXT_CLASS: Record<AvatarSize, string> = {
  28: 'text-[10px]',
  32: 'text-[11px]',
  40: 'text-xs',
  72: 'text-xl',
  84: 'text-2xl',
}

interface TeamAvatarProps {
  name: string
  /** Pixel size. Default 28. */
  size?: AvatarSize
}

/** Default-seeded teams are all named "Team N" — initialing that the normal
 *  first-letter-of-each-word way collapses "Team 1" and "Team 10" to the
 *  same "T1", since it only takes one digit off the number. Showing the
 *  full number instead keeps every default team visually distinct; a
 *  renamed/custom team name falls back to ordinary initials. */
function teamInitials(name: string): string {
  const numbered = name.match(/^team\s+(\d+)$/i)
  if (numbered) return numbered[1]
  return name
    .split(/\s+/)
    .filter(Boolean)
    .map((w) => w[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()
}

// Same three-hue cycling convention as Home's LeagueMark — a team with no
// real logo gets a flat colored mark, not an empty blob indistinguishable
// from every other team's (design review, 2026-08-21: unify the two
// placeholder conventions rather than have leagues colored and teams gray).
// Circle vs. LeagueMark's square is the type distinction: teams are
// people-ish, leagues are containers.
const TEAM_AVATAR_BG = ['bg-primary', 'bg-secondary', 'bg-highlight']

/** Simple deterministic string hash so the same team name always lands on
 *  the same hue, independent of array order (which differs between
 *  Standings/Scoreboard/the team switcher). */
function hashIndex(s: string, mod: number): number {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return h % mod
}

/** Placeholder team "profile picture" — native leagues have no real team
 *  logo pipeline yet, so every team gets a flat colored circle with its
 *  initials rather than an empty blob indistinguishable from every other
 *  team's. */
function TeamAvatar({ name, size = 28 }: TeamAvatarProps) {
  const initials = teamInitials(name)
  const bg = TEAM_AVATAR_BG[hashIndex(name, TEAM_AVATAR_BG.length)]
  return (
    <div
      className={`${AVATAR_CLASS[size]} ${TEAM_AVATAR_TEXT_CLASS[size]} ${bg} flex items-center justify-center rounded-full font-display font-semibold text-primary-foreground shrink-0`}
    >
      {initials}
    </div>
  )
}

// ── Player name cell ────────────────────────────────────────────────────────

interface PlayerCellProps {
  name: string
  imageUrl?: string | null
  /** Optional subtitle shown below the name (e.g. team abbreviation). */
  sub?: string
  /** Player-detail URL. When set, the name itself (not the row) is the
   *  click/keyboard target — a real `<Link>`, so cmd/ctrl-click and
   *  middle-click "open in new tab" work too. Omit for an unlinked name.
   *  Ignored when `onClick` is given. */
  href?: string
  /** Runs instead of navigating — e.g. opening the player in a side
   *  panel/bottom sheet in place rather than going to `/players/:gsisId`.
   *  Takes precedence over `href`, same convention as MobileStatCard. */
  onClick?: () => void
  /** Avatar size. Default 28. */
  avatarSize?: AvatarSize
  /** Situational notes (injury, depth chart, etc.). Renders a notes button
   *  next to the name — absent entirely when there's nothing to show. */
  notes?: SituationalNote[]
}

/** Standard player cell with avatar + name (+ optional subtitle). The name is
 *  the only click target for player detail — the surrounding row is not
 *  (see ClickableRow), since a row-wide click target swallowed clicks meant
 *  for other cells (stats, contract actions, drag handles) and made "click
 *  to see this player" ambiguous on rows with several interactive parts. */
function PlayerCell({ name, imageUrl, sub, href, onClick, avatarSize = 28, notes }: PlayerCellProps) {
  return (
    <TableCell className="font-medium">
      <div className="flex items-center gap-2">
        <PlayerAvatar src={imageUrl} alt={name} size={avatarSize} />
        <div>
          {onClick ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                onClick()
              }}
              className="rounded-sm text-left hover:text-primary hover:underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {name}
            </button>
          ) : href ? (
            <Link
              to={href}
              onClick={(e) => e.stopPropagation()}
              className="rounded-sm hover:text-primary hover:underline underline-offset-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {name}
            </Link>
          ) : (
            <span>{name}</span>
          )}
          {sub && <div className="text-xs text-muted-foreground">{sub}</div>}
        </div>
        <NotesButton notes={notes} />
      </div>
    </TableCell>
  )
}

// ── Notes button ────────────────────────────────────────────────────────────

interface NotesButtonProps {
  notes?: SituationalNote[]
}

/**
 * Small button next to a player's name that opens a popover with their
 * situational notes (injury, depth-chart battle, etc.) — renders nothing when
 * there are none. Highlights (positive accent) when any note is recent
 * enough to count as "new" (server-computed — see SituationalNote.is_new).
 *
 * Hand-rolled click-toggle popover, not the Radix Tooltip primitive used
 * elsewhere in this file — a Tooltip's hover/focus-triggered open and
 * mouse-leave-to-dismiss behavior fight with wanting this to open and close
 * only on an explicit click (including on touch, where hover doesn't exist).
 * Same "own markup, closes on outside click" convention as NativeRosterTable's
 * RowActionsMenu/slot picker.
 */
function NotesButton({ notes }: NotesButtonProps) {
  const [open, setOpen] = React.useState(false)
  const ref = React.useRef<HTMLSpanElement | null>(null)

  React.useEffect(() => {
    if (!open) return
    const clickHandler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', clickHandler)
    document.addEventListener('keydown', keyHandler)
    return () => {
      document.removeEventListener('mousedown', clickHandler)
      document.removeEventListener('keydown', keyHandler)
    }
  }, [open])

  if (!notes || notes.length === 0) return null
  const hasNew = notes.some((n) => n.is_new)

  return (
    <span ref={ref} className="relative inline-block shrink-0">
      <button
        type="button"
        onClick={(e) => {
          e.preventDefault()
          e.stopPropagation()
          setOpen((o) => !o)
        }}
        onKeyDown={(e) => e.stopPropagation()}
        aria-label={hasNew ? 'New player news' : 'Player news'}
        aria-expanded={open}
        className={cn(
          'inline-flex h-5 w-5 items-center justify-center rounded-full border',
          hasNew
            ? 'border-positive-border bg-positive-light text-positive-foreground'
            : 'border-border bg-muted text-muted-foreground hover:text-foreground'
        )}
      >
        <Newspaper className="h-3 w-3" aria-hidden />
      </button>
      {open && (
        <div
          onClick={(e) => e.stopPropagation()}
          className="absolute left-0 top-full z-30 mt-1 w-72 max-w-[80vw] rounded-md border border-border bg-card p-2.5 normal-case"
        >
          <ul className="space-y-2.5">
            {notes.map((n, i) => (
              <li key={i} className="text-xs">
                <span
                  className={cn(
                    'inline-block rounded border px-1.5 py-0.5 text-[10px] font-semibold whitespace-nowrap',
                    NOTE_DIRECTION_STYLES[n.impact_direction] ?? NOTE_DIRECTION_STYLES.neutral
                  )}
                >
                  {NOTE_CATEGORY_LABELS[n.category] ?? n.category}
                  {n.is_new ? ' · new' : ''}
                </span>
                <p className="mt-1 text-foreground leading-snug">{n.summary}</p>
                {(n.source || n.reported_date) && (
                  <p className="mt-0.5 text-[11px] text-muted-foreground">
                    {n.source}
                    {n.source && n.reported_date ? ' · ' : ''}
                    {n.reported_date}
                  </p>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </span>
  )
}

// ── Clickable row ───────────────────────────────────────────────────────────

const ROW_HOVER_CLASS = 'hover:bg-card'

interface ClickableRowProps extends React.HTMLAttributes<HTMLTableRowElement> {
  children: React.ReactNode
}

/**
 * TableRow with the standard row-hover treatment. Player-detail navigation
 * lives on the player's name (PlayerCell's `href`, a real `<Link>`), not the
 * row — a row-wide click target swallowed clicks meant for other cells
 * (stats, drag handles, action menus) and made "click to see this player"
 * ambiguous on rows with several interactive parts. This is now just a thin
 * hover-styled wrapper; callers that need a real row-level interaction
 * (NativeRosterTable's drag/pick, DraftBoardTable's drag-to-reorder) wire
 * their own onClick/onDrag* handlers through normal props.
 */
function ClickableRow({ children, className, ...rest }: ClickableRowProps) {
  return (
    <TableRow className={`${ROW_HOVER_CLASS} ${className ?? ''}`} {...rest}>
      {children}
    </TableRow>
  )
}

// ── Z-score stat cell ───────────────────────────────────────────────────────

interface ZScoreCellProps {
  /** Formatted display value (e.g. "1.234", "312"). */
  value: string
  /** Z-score for coloring/indicator. */
  zScore: number
  className?: string
}

/** Right-aligned stat cell with a z-score indicator glyph. */
function ZScoreCell({ value, zScore, className }: ZScoreCellProps) {
  return (
    <TableCell className={`text-right text-xs font-mono tabular-nums ${className ?? ''}`}>
      {value}
      <span
        className={`ml-0.5 text-[10px] ${zScoreColor(zScore)}`}
        aria-label={zScore > 0 ? 'Above average' : zScore < 0 ? 'Below average' : 'Average'}
      >
        {zScoreIndicator(zScore) || '●'}
      </span>
    </TableCell>
  )
}

// ── Header row ──────────────────────────────────────────────────────────────

/** Standard header row — a flat dark bar, square corners, no hover. */
function HeaderRow({ children, className, ...props }: React.HTMLAttributes<HTMLTableRowElement>) {
  return (
    <TableRow className={`bg-card hover:bg-card ${className ?? ''}`} {...props}>
      {children}
    </TableRow>
  )
}

export {
  SortableHead,
  HeaderTip,
  MobileHeaderTip,
  useTableSort,
  PlayerAvatar,
  TeamAvatar,
  PlayerCell,
  NotesButton,
  ClickableRow,
  ZScoreCell,
  HeaderRow,
}
export type { SortDir, AvatarSize }
