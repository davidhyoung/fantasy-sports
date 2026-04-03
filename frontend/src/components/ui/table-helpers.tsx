import * as React from 'react'
import { useNavigate } from 'react-router-dom'
import { ChevronUp, ChevronDown, ChevronsUpDown } from 'lucide-react'
import { TableHead, TableRow, TableCell } from '@/components/ui/table'
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

/** Sortable column header with chevron indicators. */
function SortableHead({ col, current, dir, onSort, children, className }: SortableHeadProps) {
  const active = col === current
  return (
    <TableHead
      className={`cursor-pointer select-none hover:text-foreground whitespace-nowrap ${className ?? ''}`}
      onClick={() => onSort(col)}
    >
      <div className="flex items-center gap-1">
        <span>{children}</span>
        {active ? (
          dir === 'desc'
            ? <ChevronDown className="h-3 w-3 shrink-0" />
            : <ChevronUp className="h-3 w-3 shrink-0" />
        ) : (
          <ChevronsUpDown className="h-3 w-3 shrink-0 opacity-30" />
        )}
      </div>
    </TableHead>
  )
}

/** Standard toggle handler for sortable columns.
 *  If clicking the active column, flip direction. Otherwise switch to the new column
 *  with a sensible default direction (asc for string columns, desc for numeric). */
function useTableSort(defaultCol: string, defaultDir: SortDir = 'desc', stringCols: string[] = []) {
  const [sortCol, setSortCol] = React.useState(defaultCol)
  const [sortDir, setSortDir] = React.useState<SortDir>(defaultDir)

  const handleSort = React.useCallback((col: string) => {
    setSortCol((prev) => {
      if (prev === col) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return prev
      }
      setSortDir(stringCols.includes(col) ? 'asc' : 'desc')
      return col
    })
  }, [stringCols])

  return { sortCol, sortDir, handleSort } as const
}

// ── Player avatar ───────────────────────────────────────────────────────────

interface PlayerAvatarProps {
  src?: string | null
  alt: string
  /** Pixel size. Default 28 (h-7 w-7). Use 32 for larger variant. */
  size?: 28 | 32
}

/** Rounded player headshot with muted fallback circle. */
function PlayerAvatar({ src, alt, size = 28 }: PlayerAvatarProps) {
  const cls = size === 32 ? 'h-8 w-8' : 'h-7 w-7'
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
  avatarSize?: 28 | 32
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

const CLICKABLE_ROW_CLASS = 'cursor-pointer hover:bg-muted/30 focus-visible:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

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
    <TableCell className={`text-right text-xs tabular-nums ${className ?? ''}`}>
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

/** Standard header row with consistent background. Wraps TableRow. */
function HeaderRow({ children, className, ...props }: React.HTMLAttributes<HTMLTableRowElement>) {
  return (
    <TableRow className={`bg-card first:rounded-t-lg [&>th:first-child]:rounded-tl-lg [&>th:last-child]:rounded-tr-lg ${className ?? ''}`} {...props}>
      {children}
    </TableRow>
  )
}

export {
  SortableHead,
  useTableSort,
  PlayerAvatar,
  PlayerCell,
  ClickableRow,
  ZScoreCell,
  HeaderRow,
}
export type { SortDir }
