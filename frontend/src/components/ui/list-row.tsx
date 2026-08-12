import * as React from 'react'
import { Link } from 'react-router-dom'
import { cn } from '@/lib/utils'

interface ListRowProps {
  /** Left slot — typically an avatar circle or a colored initial square. */
  leading?: React.ReactNode
  title: React.ReactNode
  subtitle?: React.ReactNode
  /** Right-aligned primary value, rendered in the accent color. */
  trailing?: React.ReactNode
  /** Right-aligned secondary value below `trailing`. */
  trailingSub?: React.ReactNode
  /** Renders the row as a router link. Takes precedence over `onClick`. */
  href?: string
  onClick?: () => void
  className?: string
}

/**
 * Bordered, rounded row for a clickable list item — leagues list, player signal
 * cards, transactions. Hover darkens to the next surface step; no shadow, no
 * scale effect.
 */
function ListRow({
  leading,
  title,
  subtitle,
  trailing,
  trailingSub,
  href,
  onClick,
  className,
}: ListRowProps) {
  const interactive = Boolean(href || onClick)

  const content = (
    <>
      {leading && <span className="flex-none">{leading}</span>}
      <div className="flex-1 min-w-0">
        <div className="font-display text-[15px] font-bold text-foreground">{title}</div>
        {subtitle && <div className="mt-[3px] text-xs text-muted-foreground">{subtitle}</div>}
      </div>
      {(trailing || trailingSub) && (
        <div className="flex-none text-right">
          {trailing && (
            <div className="font-display text-xs font-semibold text-primary">{trailing}</div>
          )}
          {trailingSub && (
            <div className="mt-0.5 text-[11px] text-muted-foreground">{trailingSub}</div>
          )}
        </div>
      )}
    </>
  )

  const classes = cn(
    'flex items-center gap-4 rounded-xl border border-border px-[18px] py-4',
    interactive && 'cursor-pointer hover:bg-card focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
    className
  )

  if (href) {
    return (
      <Link to={href} className={classes}>
        {content}
      </Link>
    )
  }

  return (
    <div
      className={classes}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={
        onClick
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onClick()
              }
            }
          : undefined
      }
    >
      {content}
    </div>
  )
}

export { ListRow }
export type { ListRowProps }
