import { cn } from '@/lib/utils'

interface DeltaBadgeProps {
  /** our_rank - consensus_rank_median. Positive = we rank them lower than consensus. */
  delta: number
  className?: string
}

/**
 * Shows the size and direction of a projection/consensus disagreement.
 *
 * The sign carries the meaning, so it's always rendered as text (+/-) rather
 * than relying on colour alone. Colour only encodes magnitude and direction as
 * a secondary cue.
 */
export default function DeltaBadge({ delta, className }: DeltaBadgeProps) {
  const magnitude = Math.abs(delta)

  const color =
    magnitude < 5
      ? 'bg-muted text-muted-foreground border-border'
      : delta > 0
        ? 'bg-negative-light text-negative-foreground border-negative-border'
        : 'bg-positive-light text-positive-foreground border-positive-border'

  const title =
    magnitude < 5
      ? 'We and the consensus rank this player about the same.'
      : delta > 0
        ? `We rank this player ${magnitude.toFixed(1)} spots lower than the consensus does.`
        : `We rank this player ${magnitude.toFixed(1)} spots higher than the consensus does.`

  return (
    <span
      className={cn(
        'text-xs px-1.5 py-0.5 rounded border font-mono tabular-nums whitespace-nowrap',
        color,
        className
      )}
      title={title}
    >
      {delta > 0 ? '+' : delta < 0 ? '−' : ''}
      {magnitude.toFixed(1)}
    </span>
  )
}
