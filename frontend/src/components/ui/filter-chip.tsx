import * as React from 'react'
import { cn } from '@/lib/utils'

interface FilterChipProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  active: boolean
}

/**
 * Single-select pill chip — position filters, signal filters. Active state is a
 * tinted background with an accent border, never a solid fill.
 */
function FilterChip({ active, className, children, ...props }: FilterChipProps) {
  return (
    <button
      type="button"
      aria-pressed={active}
      className={cn(
        'rounded-pill border px-3.5 py-1.5 font-display text-xs font-semibold',
        active
          ? 'border-positive-border bg-positive-light text-primary'
          : 'border-border text-muted-foreground hover:text-foreground',
        className
      )}
      {...props}
    >
      {children}
    </button>
  )
}

/**
 * Bordered dropdown control — "Scoring: PPR ▾", "Year: 2026 (proj) ▾". Wraps a
 * native select so keyboard and mobile behavior stay standard; the ▾ is drawn
 * by the wrapper since the native arrow is suppressed.
 */
interface SelectControlProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string
}

function SelectControl({ label, className, children, ...props }: SelectControlProps) {
  return (
    <label className="inline-flex items-center gap-2">
      {label && (
        <span className="font-display text-xs font-semibold text-muted-foreground">{label}</span>
      )}
      <span className="relative inline-flex items-center">
        <select
          className={cn(
            'appearance-none rounded-md border border-input bg-card py-1.5 pl-3 pr-7 font-display text-xs font-semibold text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            className
          )}
          {...props}
        >
          {children}
        </select>
        <span aria-hidden className="pointer-events-none absolute right-2.5 text-[9px] text-muted-foreground">
          ▾
        </span>
      </span>
    </label>
  )
}

export { FilterChip, SelectControl }
