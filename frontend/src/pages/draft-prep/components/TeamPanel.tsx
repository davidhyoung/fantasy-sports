import { useMemo, useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer, DraftReplacementLevel } from '@/api/client'
import { SLOT_LABELS, type DraftSettings } from '@/pages/league-detail/hooks/useDraftSettings'
import { buildRoster, type PlannedPlayer } from '../lib/roster'
import type { useDraftPrep } from '../hooks/useDraftPrep'

interface Props {
  open: boolean
  onToggle: () => void
  plannedCount: number
  players: DraftPlayer[]
  replacementLevels: DraftReplacementLevel[]
  settings: DraftSettings
  prep: ReturnType<typeof useDraftPrep>
}

/** Editable dollar figure that commits on blur, not on every keystroke. */
function CostField({ value, onCommit }: { value: number; onCommit: (v: number) => void }) {
  const [draft, setDraft] = useState(String(value))
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(String(value))
  }
  return (
    <span className="inline-flex items-baseline">
      <span className="font-mono text-[11px] text-muted-foreground">$</span>
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          const next = parseInt(draft, 10)
          if (Number.isNaN(next) || next < 0) {
            setDraft(String(value))
            return
          }
          if (next !== value) onCommit(Math.min(next, 10000))
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') setDraft(String(value))
        }}
        inputMode="numeric"
        aria-label="Planned cost"
        className="w-8 bg-transparent text-right font-mono text-xs tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
      />
    </span>
  )
}

function Stat({ label, value, tone = '' }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <div className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div className={`font-mono text-sm tabular-nums ${tone || 'text-foreground'}`}>{value}</div>
    </div>
  )
}

/**
 * The team you'd have if every planned price held, docked beside the board.
 *
 * It sits next to the board rather than replacing it because it's a running
 * total of decisions made over there — you add a player on the left and watch
 * the budget move on the right. Collapsed, it stays as a narrow rail so the
 * board gets the full width when you're just reading.
 */
export function TeamPanel({
  open, onToggle, plannedCount, players, replacementLevels, settings, prep,
}: Props) {
  const byId = useMemo(() => new Map(players.map((p) => [p.gsis_id, p])), [players])

  const planned: PlannedPlayer[] = useMemo(() => {
    const out: PlannedPlayer[] = []
    for (const [gsisId, entry] of prep.byPlayer) {
      if (entry.planned_cost == null) continue
      const player = byId.get(gsisId)
      if (player) out.push({ player, cost: entry.planned_cost })
    }
    return out
  }, [prep.byPlayer, byId])

  const replacementByPosition = useMemo(
    () => new Map(replacementLevels.map((r) => [r.position, r.points])),
    [replacementLevels],
  )

  const roster = useMemo(
    () => buildRoster(planned, settings, replacementByPosition),
    [planned, settings, replacementByPosition],
  )

  if (!open) {
    return (
      <button
        onClick={onToggle}
        aria-expanded={false}
        title="Show your team"
        className="flex w-full shrink-0 items-center justify-center gap-2 rounded-lg bg-card px-2 py-2 hover:bg-muted lg:sticky lg:top-[calc(var(--nav-height)+1rem)] lg:w-auto lg:flex-col lg:py-3"
      >
        <span className="font-mono text-xs text-muted-foreground">◂</span>
        <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground lg:[writing-mode:vertical-rl]">
          Team {plannedCount > 0 ? plannedCount : ''}
        </span>
      </button>
    )
  }

  const overBudget = roster.spent > roster.budget

  return (
    <aside className="w-full shrink-0 space-y-3 lg:sticky lg:top-[calc(var(--nav-height)+1rem)] lg:w-[300px] xl:w-[340px]">
      <div className="rounded-lg bg-card">
        <div className="flex items-center justify-between gap-2 px-3 py-2">
          <h2 className="font-display text-sm font-semibold text-foreground">Your team</h2>
          <button
            onClick={onToggle}
            aria-expanded
            title="Hide your team"
            className="font-mono text-xs text-muted-foreground hover:text-foreground"
          >
            ▸
          </button>
        </div>

        {planned.length === 0 ? (
          <p className="border-t border-border px-3 py-3 text-xs text-muted-foreground">
            Add players with the <span className="font-mono">+</span> in the board's Plan column —
            they start at our auction value and the price is editable here.
          </p>
        ) : (
          <>
            <div className="grid grid-cols-3 gap-2 border-t border-border px-3 py-2">
              <Stat
                label="Spent"
                value={`$${roster.spent}`}
                tone={overBudget ? 'text-secondary' : 'text-foreground'}
              />
              <Stat
                label="Left"
                value={`$${roster.remaining}`}
                tone={roster.remaining < 0 ? 'text-secondary' : 'text-positive'}
              />
              <Stat label="Max bid" value={`$${roster.maxBid}`} />
              <Stat label="Starters" value={roster.starterPoints.toFixed(0)} />
              <Stat label="VOR" value={roster.starterVOR.toFixed(0)} />
              <Stat
                label="Filled"
                value={`${planned.length - roster.overflow.length}/${roster.slots.length}`}
              />
            </div>

            <ul className="border-t border-border">
              {roster.slots.map((s, i) => {
                const p = s.player
                const bench = s.slot === 'BN'
                return (
                  <li
                    key={`${s.slot}-${i}`}
                    className="flex items-center gap-2 border-b border-border px-3 py-1 last:border-0"
                  >
                    <span
                      className={`w-11 shrink-0 font-display text-[10px] font-semibold uppercase ${
                        bench ? 'text-muted-foreground/60' : 'text-muted-foreground'
                      }`}
                    >
                      {SLOT_LABELS[s.slot]}
                    </span>
                    {p ? (
                      <>
                        <RouterLink
                          to={`/players/${p.player.gsis_id}`}
                          className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                          title={`${p.player.name} · ${p.player.proj_league_fpts.toFixed(0)} proj · our value $${p.player.auction_value}`}
                        >
                          {p.player.name}
                        </RouterLink>
                        <CostField
                          value={p.cost}
                          onCommit={(v) => prep.setPlannedCost(p.player.gsis_id, v)}
                        />
                        <button
                          onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                          title={`Remove ${p.player.name}`}
                          className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                        >
                          ×
                        </button>
                      </>
                    ) : (
                      <span className="flex-1 text-xs text-muted-foreground/40">empty</span>
                    )}
                  </li>
                )
              })}
            </ul>
          </>
        )}
      </div>

      {roster.warnings.length > 0 && (
        <ul className="space-y-1 rounded-lg bg-negative-light px-3 py-2">
          {roster.warnings.map((w) => (
            <li key={w} className="font-display text-[11px] font-semibold text-negative-foreground">
              {w}
            </li>
          ))}
        </ul>
      )}

      {roster.overflow.length > 0 && (
        <div className="rounded-lg bg-card px-3 py-2">
          <h3 className="font-display text-[10px] font-semibold uppercase tracking-wide text-secondary">
            No roster spot
          </h3>
          <ul className="mt-1 space-y-1">
            {roster.overflow.map((p) => (
              <li key={p.player.gsis_id} className="flex items-center gap-2">
                <RouterLink
                  to={`/players/${p.player.gsis_id}`}
                  className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                >
                  {p.player.name}
                </RouterLink>
                <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                  ${p.cost}
                </span>
                <button
                  onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                  className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </aside>
  )
}
