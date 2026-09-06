import * as React from 'react'
import { useNavigate } from 'react-router-dom'

export interface MobileStatField {
  label: string
  value: React.ReactNode
}

interface MobileStatCardProps {
  /** Navigates on tap of the card body (not the chevron). Omit for a non-clickable card. */
  href?: string
  /** Runs on tap of the card body instead of navigating — for a card whose
   *  tap does something other than go to a detail page (e.g. the Roster
   *  card's picking-mode "tap to place/swap"). Takes precedence over `href`
   *  when both are given. */
  onClick?: () => void
  /** Reuse PlayerAvatar exactly as PlayerCell renders it today. */
  leading?: React.ReactNode
  title: React.ReactNode
  subtitle?: React.ReactNode
  /** 2–3 right-aligned fields on the card face — the columns a table's role calls load-bearing. */
  face?: React.ReactNode
  /** Extra row(s) below the title/face line — e.g. draft prep's interest thumbs and plan control. */
  faceExtra?: React.ReactNode
  /** Remaining columns, a literal mapping — nothing dropped — rendered as a 2-col label/value grid. */
  expanded?: MobileStatField[]
  /** Arbitrary content appended after the label/value grid (e.g. an action row). */
  expandedExtra?: React.ReactNode
  className?: string
}

/**
 * Replaces a `<Table>` row below `md` on every table screen in the mobile
 * handoff. Card row: `bg-card`, bordered, `rounded-lg`, ≥44px tall, 12px
 * padding — same flat, no-shadow vocabulary as `table-helpers.tsx`.
 *
 * The chevron (▾/▴ text glyph, never a lucide icon in a table context) toggles
 * the expansion panel with `stopPropagation`, the same convention the desktop
 * board already uses for its inline thumb/+ buttons — so tapping it never also
 * navigates the card.
 */
export function MobileStatCard({
  href, onClick, leading, title, subtitle, face, faceExtra, expanded, expandedExtra, className,
}: MobileStatCardProps) {
  const [open, setOpen] = React.useState(false)
  const navigate = useNavigate()
  const hasExpansion = (expanded && expanded.length > 0) || !!expandedExtra
  const clickable = !!onClick || !!href
  const activate = onClick ?? (href ? () => navigate(href) : undefined)

  return (
    <div
      className={`min-h-[var(--tap-target-min)] rounded-lg border border-border bg-card p-3 ${clickable ? 'cursor-pointer' : ''} ${className ?? ''}`}
      onClick={activate}
      role={clickable ? (onClick ? 'button' : 'link') : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={
        activate
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                activate()
              }
            }
          : undefined
      }
    >
      <div className="flex items-center gap-2">
        {leading && <span className="flex-none">{leading}</span>}
        <div className="min-w-0 flex-1">
          <div className="truncate font-display text-sm font-semibold text-foreground">{title}</div>
          {subtitle && <div className="text-xs text-muted-foreground">{subtitle}</div>}
        </div>
        {face && (
          <div onClick={(e) => e.stopPropagation()} className="flex flex-none items-center gap-3">
            {face}
          </div>
        )}
        {hasExpansion && (
          <button
            onClick={(e) => {
              e.stopPropagation()
              setOpen((o) => !o)
            }}
            aria-expanded={open}
            aria-label={open ? 'Collapse details' : 'Expand details'}
            className="flex h-11 w-11 flex-none items-center justify-center font-mono text-xs text-muted-foreground hover:text-foreground"
          >
            {open ? '▴' : '▾'}
          </button>
        )}
      </div>

      {faceExtra && (
        <div onClick={(e) => e.stopPropagation()} className="mt-2">
          {faceExtra}
        </div>
      )}

      {hasExpansion && open && (
        <div onClick={(e) => e.stopPropagation()} className="mt-3 border-t border-border pt-3">
          {expanded && expanded.length > 0 && (
            <div className="grid grid-cols-2 gap-x-3 gap-y-2">
              {expanded.map((f, i) => (
                <div key={i}>
                  <div className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                    {f.label}
                  </div>
                  <div className="mt-0.5 text-xs text-foreground">{f.value}</div>
                </div>
              ))}
            </div>
          )}
          {expandedExtra}
        </div>
      )}
    </div>
  )
}
