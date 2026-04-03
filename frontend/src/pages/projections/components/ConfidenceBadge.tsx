import { cn } from '@/lib/utils'

interface ConfidenceBadgeProps {
  value: number // 0–1
  className?: string
}

export default function ConfidenceBadge({ value, className }: ConfidenceBadgeProps) {
  const pct = Math.round(value * 100)
  const color =
    value >= 0.70 ? 'bg-green-100 text-green-700 border-green-300 dark:bg-green-900/40 dark:text-green-400 dark:border-green-800' :
    value >= 0.45 ? 'bg-yellow-100 text-yellow-700 border-yellow-300 dark:bg-yellow-900/40 dark:text-yellow-400 dark:border-yellow-800' :
                    'bg-red-100 text-red-700 border-red-300 dark:bg-red-900/40 dark:text-red-400 dark:border-red-800'

  return (
    <span className={cn('text-xs px-1.5 py-0.5 rounded border font-mono', color, className)}>
      {pct}%
    </span>
  )
}
