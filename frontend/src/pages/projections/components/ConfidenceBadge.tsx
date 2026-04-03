import { cn } from '@/lib/utils'

interface ConfidenceBadgeProps {
  value: number // 0–1
  className?: string
}

export default function ConfidenceBadge({ value, className }: ConfidenceBadgeProps) {
  const pct = Math.round(value * 100)
  const color =
    value >= 0.70 ? 'bg-positive-light text-positive-foreground border-positive-border' :
    value >= 0.45 ? 'bg-warning-light text-warning-foreground border-warning-border' :
                    'bg-negative-light text-negative-foreground border-negative-border'

  return (
    <span className={cn('text-xs px-1.5 py-0.5 rounded border font-mono', color, className)}>
      {pct}%
    </span>
  )
}
