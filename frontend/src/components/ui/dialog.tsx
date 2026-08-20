import * as React from 'react'

interface DialogProps {
  open: boolean
  onClose: () => void
  title?: string
  children: React.ReactNode
}

/**
 * Centered modal shell — assign-player form, trade builder, rollover
 * confirmation. MobileSheet's desktop counterpart: same no-animation,
 * instant-state-change convention (the design system has zero transitions
 * anywhere except loading spinners), backdrop click and Escape both close.
 */
export function Dialog({ open, onClose, title, children }: DialogProps) {
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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-background/80" onClick={onClose} aria-hidden="true" />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative max-h-[85vh] w-full max-w-md overflow-y-auto rounded-lg border border-border bg-card p-5"
      >
        <div className="mb-4 flex items-center justify-between gap-3">
          {title && <h2 className="font-display text-sm font-semibold text-foreground">{title}</h2>}
          <button
            onClick={onClose}
            aria-label="Close"
            className="ml-auto flex h-8 w-8 flex-none items-center justify-center font-mono text-sm text-muted-foreground hover:text-foreground"
          >
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}
