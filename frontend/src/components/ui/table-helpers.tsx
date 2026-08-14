import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { TableHead, TableRow, TableCell } from '@/components/ui/table'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { zScoreIndicator, zScoreColor } from '@/lib/utils'

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
        className={`${cls} rounded-full object-cover bg-muted shrink-0`}
      />
    )
  }
  return <div className={`${cls} rounded-full bg-muted shrink-0`} />
}

// ── Player name cell ────────────────────────────────────────────────────────

interface PlayerCellProps {
  name: string
  imageUrl?: string | null
  /** Optional subtitle shown below the name (e.g. team abbreviation). */
  sub?: string
  /** Whether this row is clickable (adds hover:text-primary to name). */
  linked?: boolean
  /** Avatar size. Default 28. */
  avatarSize?: AvatarSize
}

/** Standard player cell with avatar + name (+ optional subtitle). */
function PlayerCell({ name, imageUrl, sub, linked, avatarSize = 28 }: PlayerCellProps) {
  return (
    <TableCell className="font-medium">
      <div className="flex items-center gap-2">
        <PlayerAvatar src={imageUrl} alt={name} size={avatarSize} />
        <div>
          <span className={linked ? 'hover:text-primary' : undefined}>{name}</span>
          {sub && <div className="text-xs text-muted-foreground">{sub}</div>}
        </div>
      </div>
    </TableCell>
  )
}

// ── Clickable row ───────────────────────────────────────────────────────────

const CLICKABLE_ROW_CLASS = 'cursor-pointer hover:bg-card focus-visible:bg-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

interface ClickableRowProps extends React.HTMLAttributes<HTMLTableRowElement> {
  /** URL to navigate to. If undefined, the row renders as a normal non-clickable row. */
  href?: string
  children: React.ReactNode
}

/** TableRow that optionally navigates on click/Enter/Space. Pass `href` to enable. */
function ClickableRow({ href, children, className, ...rest }: ClickableRowProps) {
  const navigate = useNavigate()
  if (!href) {
    return <TableRow className={className} {...rest}>{children}</TableRow>
  }
  return (
    <TableRow
      className={`${CLICKABLE_ROW_CLASS} ${className ?? ''}`}
      onClick={() => navigate(href)}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          navigate(href)
        }
      }}
      tabIndex={0}
      role="link"
      {...rest}
    >
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
  useTableSort,
  PlayerAvatar,
  PlayerCell,
  ClickableRow,
  ZScoreCell,
  HeaderRow,
}
export type { SortDir, AvatarSize }
