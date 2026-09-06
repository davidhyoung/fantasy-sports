import type { InterestLevel } from '@/api/client'

/**
 * Interest is binary: target him or avoid him. The sign is kept rather than a
 * boolean so the board can still sort on it, and so widening the scale again
 * would be a constraint change rather than a data model one.
 */
export const INTEREST_LEVELS: { level: InterestLevel; label: string; short: string }[] = [
  { level: 1, label: 'Target', short: '+' },
  { level: -1, label: 'Avoid', short: '−' },
]

export function interestLabel(level: InterestLevel | null): string {
  return INTEREST_LEVELS.find((l) => l.level === level)?.label ?? 'No opinion'
}

/** Positive takes the one accent, negative the cool secondary. */
export function interestIconClass(level: InterestLevel, selected: boolean): string {
  if (!selected) return 'text-muted-foreground/40'
  return level > 0 ? 'text-primary' : 'text-secondary'
}

/**
 * Left edge on a rated player's row, so targets and avoids are visible while
 * scanning the board itself rather than repeated in a panel above it. Written as
 * whole literal class names because Tailwind scans source text — a class built by
 * concatenation at runtime never gets generated.
 */
const ROW_EDGE: Record<InterestLevel, string> = {
  1: 'border-l-[3px] border-l-primary',
  '-1': 'border-l-[3px] border-l-secondary',
}

export function interestRowClass(level: InterestLevel | null): string {
  return level == null ? '' : ROW_EDGE[level]
}

/** Text-only variant for lists, where a filled chip would be too heavy. */
export function interestTextClass(level: InterestLevel): string {
  return level > 0 ? 'text-primary' : 'text-secondary'
}
