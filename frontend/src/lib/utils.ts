import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
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
