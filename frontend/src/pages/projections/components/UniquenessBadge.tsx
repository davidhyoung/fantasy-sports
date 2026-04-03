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

const colors: Record<Uniqueness, string> = {
  common:   'bg-highlight-light text-highlight-foreground border-highlight-border',
  moderate: 'bg-muted text-muted-foreground border-border',
  rare:     'bg-warning-light text-warning-foreground border-warning-border',
  unique:   'bg-highlight-light text-highlight-foreground border-highlight-border',
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
