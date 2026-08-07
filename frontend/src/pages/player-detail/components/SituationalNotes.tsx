import type { SituationalNote } from '@/api/client'
import { cn } from '@/lib/utils'

interface SituationalNotesProps {
  notes: SituationalNote[]
}

const CATEGORY_LABELS: Record<string, string> = {
  injury: 'Injury',
  depth_chart: 'Depth chart',
  scheme: 'Scheme',
  holdout: 'Holdout',
  suspension: 'Suspension',
  rookie_buzz: 'Rookie buzz',
  trade: 'Trade',
}

const DIRECTION_STYLES: Record<string, string> = {
  positive: 'bg-positive-light text-positive-foreground border-positive-border',
  negative: 'bg-negative-light text-negative-foreground border-negative-border',
  neutral: 'bg-muted text-muted-foreground border-border',
}

/**
 * External situational context (injuries, camp battles, trades) for a player.
 *
 * These are shown alongside the projection, never folded into it — the model
 * has no representation of this information, and deciding what a
 * "season-ending injury" should do to a number is a human judgment.
 */
export default function SituationalNotes({ notes }: SituationalNotesProps) {
  if (notes.length === 0) return null

  return (
    <section className="rounded-lg bg-card p-4 space-y-3">
      <div>
        <h2 className="text-sm font-semibold text-foreground">Situational Context</h2>
        <p className="text-xs text-muted-foreground mt-0.5">
          News the projection can't account for — shown for context, not applied to the
          numbers.
        </p>
      </div>

      <ul className="space-y-2.5">
        {notes.map((n, i) => (
          <li key={i} className="text-sm border-l-2 border-border pl-3">
            <div className="flex items-center gap-1.5 flex-wrap">
              <span
                className={cn(
                  'text-xs px-1.5 py-0.5 rounded border whitespace-nowrap',
                  DIRECTION_STYLES[n.impact_direction] ?? DIRECTION_STYLES.neutral
                )}
              >
                {CATEGORY_LABELS[n.category] ?? n.category} · {n.impact_magnitude}
              </span>

              {/* Team-scoped notes are about the team, not this player specifically —
                  label them so a QB competition isn't read as being about a receiver. */}
              {n.scope === 'team' && (
                <span className="text-xs px-1.5 py-0.5 rounded border bg-muted text-muted-foreground border-border whitespace-nowrap">
                  team context
                </span>
              )}

              {n.confidence === 'single_source' && (
                <span
                  className="text-xs text-muted-foreground/70"
                  title="Reported by a single outlet — treat as lower confidence."
                >
                  single source
                </span>
              )}
            </div>

            <p className="text-foreground mt-1 leading-snug">{n.summary}</p>

            {(n.source || n.reported_date) && (
              <p className="text-xs text-muted-foreground mt-0.5">
                {n.source}
                {n.source && n.reported_date ? ' · ' : ''}
                {n.reported_date}
              </p>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
