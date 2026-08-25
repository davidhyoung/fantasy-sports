import * as React from 'react'

interface SidePanelProps {
  open: boolean
  onClose: () => void
  title?: string
  children: React.ReactNode
  /** Optional action-button row pinned to the bottom of the panel, outside
   *  the scrollable body — e.g. player-detail's Edit contract/Drop/Trade
   *  actions, which should stay reachable without scrolling a long profile. */
  footer?: React.ReactNode
}

/**
 * Right-docked, full-height panel — desktop counterpart to MobileSheet for
 * content browsed alongside a table (e.g. a player's detail opened from a
 * roster row) rather than a transactional form, which uses the centered
 * Dialog instead. Same no-animation, instant-state-change convention as
 * Dialog/MobileSheet, and the same backdrop-click/Escape-to-close behavior.
 */
export function SidePanel({ open, onClose, title, children, footer }: SidePanelProps) {
  React.useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, onClose])

  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-background/80" onClick={onClose} aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative flex h-full w-full max-w-lg flex-col border-l border-border bg-background"
      >
        <div className="flex items-center justify-between gap-3 p-5 pb-4">
          {title && <h2 className="font-display text-sm font-semibold text-foreground">{title}</h2>}
          <button
            onClick={onClose}
            aria-label="Close"
            className="ml-auto flex h-8 w-8 flex-none items-center justify-center font-mono text-sm text-muted-foreground hover:text-foreground"
          >
            ✕
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-5 pb-5">
          {children}
        </div>
        {footer && (
          <div className="border-t border-border p-4">
            {footer}
          </div>
        )}
      </div>
    </div>
  )
}
