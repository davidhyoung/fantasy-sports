import * as React from 'react'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import type { SortDir } from '@/components/ui/table-helpers'

export interface MobileSortOption {
  col: string
  label: string
  description?: string
}

interface MobileSortSheetProps {
  options: MobileSortOption[]
  current: string
  dir: SortDir
  /** Same handler `SortableHead`/`useTableSort` already use — clicking the
   *  active column flips direction, clicking another switches to it. */
  onSort: (col: string) => void
}

/**
 * Replaces clickable `SortableHead` column headers below `md` — headers don't
 * fit in a card list. A pill button shows the active sort column with the
 * same ▲▼ typographic glyph convention as desktop (never a lucide icon in a
 * table context); tapping it opens a flat list of every sortable column.
 */
export function MobileSortSheet({ options, current, dir, onSort }: MobileSortSheetProps) {
  const [open, setOpen] = React.useState(false)
  const currentOption = options.find((o) => o.col === current)

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="inline-flex h-11 items-center gap-1.5 rounded-pill border border-border bg-card px-3.5 font-display text-xs font-semibold text-foreground"
      >
        <span>Sorted: {currentOption?.label ?? ''}</span>
        <span aria-hidden className="text-primary text-[9px] leading-none">
          {dir === 'desc' ? '▼' : '▲'}
        </span>
      </button>
      <MobileSheet open={open} onClose={() => setOpen(false)} title="Sort by">
        <ul>
          {options.map((o) => {
            const active = o.col === current
            return (
              <li key={o.col} className="border-b border-border last:border-0">
                <button
                  onClick={() => {
                    onSort(o.col)
                    setOpen(false)
                  }}
                  className={`flex min-h-[var(--tap-target-min)] w-full items-center justify-between gap-2 py-2 text-left font-display text-sm ${
                    active ? 'font-semibold text-primary' : 'text-foreground'
                  }`}
                >
                  <span>
                    {o.label}
                    {o.description && (
                      <span className="mt-0.5 block font-sans text-xs font-normal normal-case text-muted-foreground">
                        {o.description}
                      </span>
                    )}
                  </span>
                  {active && <span aria-hidden className="flex-none text-[10px]">{dir === 'desc' ? '▼' : '▲'}</span>}
                </button>
              </li>
            )
          })}
        </ul>
      </MobileSheet>
    </>
  )
}
