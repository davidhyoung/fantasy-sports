import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** "$74 · 2 yrs left" / "$9 · final yr" / "$18 · Y2Y" — years *remaining* is
 *  what a commissioner acts on; total contract length is derivable but less
 *  immediately useful. Shared by the native-league Roster table and the
 *  player profile page's contract card so the two never disagree. */
export function contractYearsLabel(salary: number, yearsTotal: number | null, yearsUsed: number): string {
  if (yearsTotal == null) return `$${salary} · Y2Y`
  const remaining = yearsTotal - yearsUsed
  if (remaining <= 0) return `$${salary} · final yr`
  return `$${salary} · ${remaining} yr${remaining === 1 ? '' : 's'} left`
}

/** No background — use zScoreIndicator for visual cues instead. */
export function zScoreBg(_z: number): string {
  return ''
}

/** Directional arrow indicating z-score intensity.
 *  Returns empty string for near-zero values (|z| < 0.5). */
export function zScoreIndicator(z: number): string {
  if (z >= 1.5) return '▲▲'
  if (z >= 0.5) return '▲'
  if (z <= -1.5) return '▼▼'
  if (z <= -0.5) return '▼'
  return ''
}

/** CSS class for z-score indicator color. Above average reads in the primary
 *  accent, below average in the secondary; intensity tracks magnitude, and
 *  zero/near-zero stays neutral. Purple is deliberately unused here — it is
 *  reserved for projected/future data. */
export function zScoreColor(z: number): string {
  if (z >= 1.5) return 'text-positive font-medium'
  if (z >= 0.5) return 'text-positive-foreground'
  if (z <= -1.5) return 'text-negative font-medium'
  if (z <= -0.5) return 'text-negative-foreground'
  return 'text-muted-foreground/40'
}
