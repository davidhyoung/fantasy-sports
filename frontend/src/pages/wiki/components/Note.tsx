import type { ReactNode } from 'react'

type NoteKind = 'context' | 'fix' | 'open' | 'caveat'

/**
 * Color-coded marginalia, reusing the app's semantic palette so the edge
 * color tells you what kind of aside it is before you read it: pink = a fix
 * that shipped, blue = background context, purple = open/future work
 * (matching the design system's "purple is projected/future only" rule),
 * amber = a tradeoff or caveat worth weighing.
 */
const STYLES: Record<NoteKind, { edge: string; bg: string; label: string; text: string }> = {
  context: {
    edge: 'border-l-[3px] border-l-negative',
    bg: 'bg-negative-light',
    label: 'text-negative-foreground',
    text: 'Context',
  },
  fix: {
    edge: 'border-l-[3px] border-l-positive',
    bg: 'bg-positive-light',
    label: 'text-positive-foreground',
    text: 'Fix',
  },
  open: {
    edge: 'border-l-[3px] border-l-highlight',
    bg: 'bg-highlight-light',
    label: 'text-highlight-foreground',
    text: 'Open question',
  },
  caveat: {
    edge: 'border-l-[3px] border-l-warning',
    bg: 'bg-warning-light',
    label: 'text-warning-foreground',
    text: 'Caveat',
  },
}

export default function Note({
  kind = 'context',
  title,
  children,
}: {
  kind?: NoteKind
  title?: string
  children: ReactNode
}) {
  const s = STYLES[kind]
  return (
    <div className={`my-4 rounded-r-md ${s.edge} ${s.bg} px-4 py-3`}>
      <div className={`font-display text-[11px] font-semibold uppercase tracking-wide ${s.label}`}>
        {title ?? s.text}
      </div>
      <div className="mt-1.5 space-y-2 text-[13px] leading-relaxed text-foreground">{children}</div>
    </div>
  )
}
