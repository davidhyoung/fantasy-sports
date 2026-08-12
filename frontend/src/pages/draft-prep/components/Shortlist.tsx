import { useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer, DraftPrepTag } from '@/api/client'
import type { useDraftPrep } from '../hooks/useDraftPrep'

const GROUPS: { tag: Exclude<DraftPrepTag, ''>; label: string; accent: string }[] = [
  { tag: 'target', label: 'Targets', accent: 'text-primary' },
  { tag: 'sleeper', label: 'Sleepers', accent: 'text-positive' },
  { tag: 'avoid', label: 'Avoid', accent: 'text-secondary' },
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
 * Your shortlist: the players you've tagged, grouped, with a note each. This is
 * the part you actually read on draft night, so it sits above the full board.
 */
export function Shortlist({ players, prep }: Props) {
  const tagged = players.filter((p) => prep.entry(p.gsis_id).tag)
  if (!tagged.length) {
    return (
      <p className="rounded-lg bg-card px-4 py-3 text-xs text-muted-foreground">
        Tag players on the board below — <span className="font-display font-semibold">T</span> target,{' '}
        <span className="font-display font-semibold">S</span> sleeper,{' '}
        <span className="font-display font-semibold">A</span> avoid — and they'll collect here with
        room for a note.
      </p>
    )
  }

  return (
    <div className="grid gap-4 rounded-lg bg-card px-4 py-3 sm:grid-cols-3">
      {GROUPS.map(({ tag, label, accent }) => {
        const group = tagged.filter((p) => prep.entry(p.gsis_id).tag === tag)
        return (
          <div key={tag}>
            <h3 className={`font-display text-[11px] font-semibold uppercase tracking-wide ${accent}`}>
              {label} <span className="font-mono text-muted-foreground">{group.length}</span>
            </h3>
            {group.length === 0 ? (
              <p className="mt-1 text-xs text-muted-foreground/60">None yet</p>
            ) : (
              <ul className="mt-1.5 space-y-1.5">
                {group.map((p) => (
                  <li key={p.gsis_id} className="border-b border-border pb-1.5 last:border-0">
                    <div className="flex items-baseline justify-between gap-2">
                      <RouterLink
                        to={`/players/${p.gsis_id}`}
                        className="font-display text-[13px] font-semibold text-foreground hover:underline"
                      >
                        {p.name}
                      </RouterLink>
                      <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
                        {p.position_group} · ${p.auction_value}
                      </span>
                    </div>
                    <NoteField
                      value={prep.entry(p.gsis_id).note}
                      onCommit={(next) => prep.setNote(p.gsis_id, next)}
                      placeholder="Add a note…"
                    />
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
