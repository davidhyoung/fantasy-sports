import type { StatView } from '../hooks/useStatView'

interface Props {
  view: StatView
  week: number
  onViewChange: (v: StatView) => void
  onWeekChange: (w: number) => void
}

const OPTIONS: { value: StatView; label: string }[] = [
  { value: 'week', label: 'Week' },
  { value: 'season', label: 'Season' },
  { value: 'rest', label: 'Rest of Season' },
  { value: 'next4', label: 'Next 4' },
]

/**
 * The 4-way stat-view pill row shared by the Roster and Players tabs — see
 * useStatView for the vocabulary/state. Same bg-muted pill convention as
 * TeamPanel's period selector and NativePlayersTab's Free Agents/Rostered
 * toggle, so this doesn't read as a new UI pattern.
 *
 * The week arrows only appear once "Week" is the active view — no upper
 * bound check on "next" (same tolerant convention NativeScoreboardTab's
 * Prev/Next already uses; going past the last real week just renders dashes
 * downstream, which is fine).
 */
export function StatViewToggle({ view, week, onViewChange, onWeekChange }: Props) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="flex w-fit flex-none rounded-lg bg-muted overflow-hidden">
        {OPTIONS.map((o) => (
          <button
            key={o.value}
            onClick={() => onViewChange(o.value)}
            className={`px-3 py-1.5 font-display text-xs font-semibold whitespace-nowrap ${
              view === o.value ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
            }`}
          >
            {o.value === 'week' && view === 'week' ? `Week ${week}` : o.label}
          </button>
        ))}
      </div>
      {view === 'week' && (
        <div className="flex items-center gap-1">
          <button
            className="rounded px-1.5 py-1 text-xs text-muted-foreground hover:bg-muted disabled:opacity-40"
            disabled={week <= 1}
            onClick={() => onWeekChange(week - 1)}
          >
            ◀
          </button>
          <button
            className="rounded px-1.5 py-1 text-xs text-muted-foreground hover:bg-muted"
            onClick={() => onWeekChange(week + 1)}
          >
            ▶
          </button>
        </div>
      )}
    </div>
  )
}
