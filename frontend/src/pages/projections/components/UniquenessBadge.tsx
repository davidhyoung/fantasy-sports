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
  common:   'bg-purple-100 text-purple-700 border-purple-300 dark:bg-purple-900/40 dark:text-purple-400 dark:border-purple-800',
  moderate: 'bg-slate-200 text-slate-700 border-slate-300 dark:bg-slate-800/60 dark:text-slate-300 dark:border-slate-700',
  rare:     'bg-orange-100 text-orange-700 border-orange-300 dark:bg-orange-900/40 dark:text-orange-400 dark:border-orange-800',
  unique:   'bg-purple-100 text-purple-700 border-purple-300 dark:bg-purple-900/40 dark:text-purple-400 dark:border-purple-800',
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
