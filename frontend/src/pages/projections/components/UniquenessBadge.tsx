import { cn } from '@/lib/utils'

type Uniqueness = 'common' | 'moderate' | 'rare' | 'unique'

interface UniquenessBadgeProps {
  value: Uniqueness
  compCount: number
  className?: string
}

const labels: Record<Uniqueness, string> = {
  common:   'Common archetype',
  moderate: 'Some parallels',
  rare:     'Rare profile',
  unique:   'Unique',
}

/** Ordinal on how much comp support the projection has: a common archetype is
 *  well-supported (primary accent), a unique one is thinly supported (secondary). */
const colors: Record<Uniqueness, string> = {
  common:   'bg-positive-light text-positive-foreground border-positive-border',
  moderate: 'bg-muted text-muted-foreground border-border',
  rare:     'bg-negative-light text-negative-foreground border-negative-border',
  unique:   'bg-negative-light text-negative-foreground border-negative-border',
}

export default function UniquenessBadge({ value, compCount, className }: UniquenessBadgeProps) {
  return (
    <span
      className={cn('text-xs px-1.5 py-0.5 rounded border whitespace-nowrap', colors[value], className)}
      title={`${compCount} historical comp${compCount !== 1 ? 's' : ''} found`}
    >
      {labels[value]} ({compCount})
    </span>
  )
}
