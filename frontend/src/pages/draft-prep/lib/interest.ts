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
 * Whether a notch on the control reads as filled at the current level.
 *
 * Thumbs fill outward from the centre, the way a star rating fills from its
 * edge: at +2 the first two thumbs-up are solid and the third is an outline, so
 * magnitude is legible at a glance without a number on every button. A notch on
 * the opposite side of the scale is never filled.
 */
export function isFilled(notch: InterestLevel, current: InterestLevel | null): boolean {
  if (current == null || Math.sign(notch) !== Math.sign(current)) return false
  return Math.abs(notch) <= Math.abs(current)
}

/** Positive takes the one accent, negative the cool secondary. */
export function interestIconClass(level: InterestLevel, filled: boolean): string {
  if (!filled) return 'text-muted-foreground/40'
  return level > 0 ? 'fill-current text-primary' : 'fill-current text-secondary'
}

/** Text-only variant for lists, where a filled chip would be too heavy. */
export function interestTextClass(level: InterestLevel): string {
  return level > 0 ? 'text-primary' : 'text-secondary'
}
