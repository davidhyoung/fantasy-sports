import * as React from 'react'

interface MobileSheetProps {
  open: boolean
  onClose: () => void
  title?: string
  children: React.ReactNode
  /** Optional action-button row pinned to the bottom of the sheet, outside
   *  the scrollable body — e.g. player-detail's Edit contract/Drop/Trade
   *  actions, which should stay reachable without scrolling a long profile. */
  footer?: React.ReactNode
}

/**
 * Bottom sheet shell for mobile — Team panel, Note editor, Planned-cost editor,
 * Move-to-rank editor, Set-tier picker, Wiki table of contents.
 *
 * No animation: the design system has zero transitions/animations anywhere
 * except loading spinners, and that rule holds here too — the sheet snaps
 * open/closed as an instant state change (conditional render, not a
 * slide/fade). No drag handle either, to avoid gesture logic for a purely
 * decorative element — a plain close button does the same job.
 */
export function MobileSheet({ open, onClose, title, children, footer }: MobileSheetProps) {
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end">
      <div
        className="absolute inset-0 bg-background/80"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative flex max-h-[85vh] flex-col rounded-t-lg border-t border-border bg-background"
      >
        <div className="flex items-center justify-between gap-3 p-4 pb-3">
          {title && <h2 className="font-display text-sm font-semibold text-foreground">{title}</h2>}
          <button
            onClick={onClose}
            aria-label="Close"
            className="ml-auto flex h-11 w-11 flex-none items-center justify-center font-mono text-sm text-muted-foreground hover:text-foreground"
          >
            ✕
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-4 pb-4">
          {children}
        </div>
        {footer && (
          <div className="border-t border-border p-3">
            {footer}
          </div>
        )}
      </div>
    </div>
  )
}
