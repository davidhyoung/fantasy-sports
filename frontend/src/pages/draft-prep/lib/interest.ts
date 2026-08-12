import type { InterestLevel } from '@/api/client'

/**
 * The interest scale, strongest positive first. Ordering matters — the board
 * sorts on it, so the numbers are the source of truth and the words are labels.
 */
export const INTEREST_LEVELS: { level: InterestLevel; label: string; short: string }[] = [
  { level: 3, label: 'Must draft', short: '+3' },
  { level: 2, label: 'Love', short: '+2' },
  { level: 1, label: 'Like', short: '+1' },
  { level: -1, label: 'Dislike', short: '−1' },
  { level: -2, label: 'Hate', short: '−2' },
  { level: -3, label: 'Do not draft', short: '−3' },
]

/** Left-to-right on the control: most negative to most positive, like a number line. */
export const SCALE_ORDER: InterestLevel[] = [-3, -2, -1, 1, 2, 3]

export function interestLabel(level: InterestLevel | null): string {
  return INTEREST_LEVELS.find((l) => l.level === level)?.label ?? 'No opinion'
}

/**
 * Colour carries magnitude, not meaning — the sign is always shown as text, so
 * the scale is still readable without colour. Positive uses the one accent,
 * negative the cool secondary, each stepping up in weight with intensity.
 */
export function interestClass(level: InterestLevel, selected: boolean): string {
  if (!selected) return 'bg-muted text-muted-foreground hover:text-foreground'
  if (level > 0) {
    return level === 3
      ? 'bg-primary text-primary-foreground font-bold'
      : level === 2
        ? 'bg-primary text-primary-foreground'
        : 'bg-primary/60 text-primary-foreground'
  }
  return level === -3
    ? 'bg-secondary text-background font-bold'
    : level === -2
      ? 'bg-secondary text-background'
      : 'bg-secondary/60 text-background'
}

/** Text-only variant for lists, where a filled chip would be too heavy. */
export function interestTextClass(level: InterestLevel): string {
  return level > 0 ? 'text-primary' : 'text-secondary'
}
