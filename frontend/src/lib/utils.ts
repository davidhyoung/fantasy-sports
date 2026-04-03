import { clsx, type ClassValue } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** No background — use zScoreIndicator for visual cues instead. */
export function zScoreBg(_z: number): string {
  return ''
}

/** Directional arrow colored by z-score intensity (purple shades).
 *  Returns empty string for near-zero values (|z| < 0.5). */
export function zScoreIndicator(z: number): string {
  if (z >= 1.5) return '▲▲'
  if (z >= 0.5) return '▲'
  if (z <= -1.5) return '▼▼'
  if (z <= -0.5) return '▼'
  return ''
}

/** CSS class for z-score indicator color — darker highlight for stronger signal,
 *  neutral muted for zero/near-zero. */
export function zScoreColor(z: number): string {
  if (z >= 1.5) return 'text-highlight font-medium'
  if (z >= 0.5) return 'text-highlight-foreground'
  if (z <= -1.5) return 'text-highlight/50'
  if (z <= -0.5) return 'text-highlight-foreground/50'
  return 'text-muted-foreground/40'
}
