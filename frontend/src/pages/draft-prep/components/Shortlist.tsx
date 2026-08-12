import { useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer } from '@/api/client'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import type { useDraftPrep } from '../hooks/useDraftPrep'
import { INTEREST_LEVELS, interestTextClass } from '../lib/interest'

/**
 * Two columns rather than one per level: six headings for six levels would be
 * mostly empty, and the level is more useful attached to the player than as a
 * heading he sits under.
 */
const GROUPS: { key: 'up' | 'down'; label: string; accent: string; want: (n: number) => boolean }[] = [
  { key: 'up', label: 'Targets', accent: 'text-primary', want: (n) => n > 0 },
  { key: 'down', label: 'Fades', accent: 'text-secondary', want: (n) => n < 0 },
]

/** Note field that only writes on blur/Enter, so typing isn't a request per keystroke. */
function NoteField({
  value, onCommit, placeholder,
}: {
  value: string
  onCommit: (next: string) => void
  placeholder: string
}) {
  const [draft, setDraft] = useState(value)
  // Adopt server/optimistic changes that didn't come from this field.
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(value)
  }

  const commit = () => {
    if (draft !== value) onCommit(draft)
  }

  return (
    <input
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur()
        if (e.key === 'Escape') setDraft(value)
      }}
      placeholder={placeholder}
      maxLength={500}
      className="w-full bg-transparent text-xs text-muted-foreground placeholder:text-muted-foreground/50 focus:text-foreground focus-visible:outline-none"
    />
  )
}

interface Props {
  players: DraftPlayer[]
  prep: ReturnType<typeof useDraftPrep>
}

/**
 * Your shortlist: the players you've rated, split into targets and fades, with a
 * headshot and a note each. This is the part you actually read on draft night, so
 * it sits above the full board.
 */
export function Shortlist({ players, prep }: Props) {
  const rated = players.filter((p) => prep.entry(p.gsis_id).interest != null)
  if (!rated.length) {
    return (
      <p className="rounded-lg bg-card px-4 py-3 text-xs text-muted-foreground">
        Rate players on the board below — <span className="font-mono">+3</span> must draft down to{' '}
        <span className="font-mono">−3</span> do not draft — and they'll collect here with room for
        a note.
      </p>
    )
  }

  return (
    <div className="grid gap-4 rounded-lg bg-card px-4 py-3 sm:grid-cols-2">
      {GROUPS.map(({ key, label, accent, want }) => {
        // Strongest feeling first, so the top of each column is what matters most.
        const group = rated
          .filter((p) => want(prep.entry(p.gsis_id).interest ?? 0))
          .sort((a, b) =>
            Math.abs(prep.entry(b.gsis_id).interest ?? 0) - Math.abs(prep.entry(a.gsis_id).interest ?? 0))
        return (
          <div key={key}>
            <h3 className={`font-display text-[11px] font-semibold uppercase tracking-wide ${accent}`}>
              {label} <span className="font-mono text-muted-foreground">{group.length}</span>
            </h3>
            {group.length === 0 ? (
              <p className="mt-1 text-xs text-muted-foreground/60">None yet</p>
            ) : (
              <ul className="mt-1.5 space-y-1.5">
                {group.map((p) => (
                  <li key={p.gsis_id} className="flex gap-2 border-b border-border pb-1.5 last:border-0">
                    <RouterLink to={`/players/${p.gsis_id}`} className="mt-0.5 shrink-0">
                      <PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />
                    </RouterLink>
                    {/* min-w-0 so a long name truncates instead of pushing the price off. */}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline justify-between gap-2">
                        <span className="flex min-w-0 items-baseline gap-1.5">
                          <span
                            className={`shrink-0 font-mono text-[11px] tabular-nums ${interestTextClass(prep.entry(p.gsis_id).interest!)}`}
                            title={INTEREST_LEVELS.find((l) => l.level === prep.entry(p.gsis_id).interest)?.label}
                          >
                            {INTEREST_LEVELS.find((l) => l.level === prep.entry(p.gsis_id).interest)?.short}
                          </span>
                          <RouterLink
                            to={`/players/${p.gsis_id}`}
                            className="truncate font-display text-[13px] font-semibold text-foreground hover:underline"
                          >
                            {p.name}
                          </RouterLink>
                        </span>
                        <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
                          {p.position_group}
                          {p.tier > 0 && (
                            <span
                              className="text-foreground"
                              title={`${p.position_group.split(',')[0]} tier ${p.tier} — how many others are this good tells you whether you can wait`}
                            >
                              {' '}T{p.tier}
                            </span>
                          )}{' '}
                          · ${p.auction_value}
                        </span>
                      </div>
                      <NoteField
                        value={prep.entry(p.gsis_id).note}
                        onCommit={(next) => prep.setNote(p.gsis_id, next)}
                        placeholder="Add a note…"
                      />
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
        )
      })}
    </div>
  )
}
