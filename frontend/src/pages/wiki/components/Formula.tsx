import type { ReactNode } from 'react'

/**
 * A math/pseudocode block. Comments (use <C>) render muted so the formula
 * itself stays the visual focus.
 */
export default function Formula({ children }: { children: ReactNode }) {
  return (
    <pre className="my-4 overflow-x-auto whitespace-pre rounded-md border border-l-[3px] border-border border-l-primary bg-muted px-4 py-3 font-mono text-[13px] leading-[1.75] text-foreground">
      {children}
    </pre>
  )
}

/** Inline comment span for use inside a <Formula>. */
export function C({ children }: { children: ReactNode }) {
  return <span className="text-muted-foreground">{children}</span>
}
