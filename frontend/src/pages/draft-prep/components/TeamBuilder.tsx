import { useMemo, useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer, DraftReplacementLevel } from '@/api/client'
import { SLOT_LABELS, type DraftSettings } from '@/pages/league-detail/hooks/useDraftSettings'
import { buildRoster, type PlannedPlayer } from '../lib/roster'
import type { useDraftPrep } from '../hooks/useDraftPrep'

interface Props {
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
  const commit = () => {
    const next = parseInt(draft, 10)
    if (Number.isNaN(next) || next < 0) {
      setDraft(String(value))
      return
    }
    if (next !== value) onCommit(Math.min(next, 10000))
  }
  return (
    <span className="inline-flex items-baseline">
      <span className="font-mono text-xs text-muted-foreground">$</span>
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') setDraft(String(value))
        }}
        inputMode="numeric"
        className="w-10 bg-transparent font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
      />
    </span>
  )
}

function Stat({ label, value, tone = '' }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div className={`font-mono text-lg tabular-nums ${tone || 'text-foreground'}`}>{value}</div>
    </div>
  )
}

/**
 * Assemble a team at the prices you expect to pay and see what it costs you.
 * Players come from the board's Plan column; everything here is what-if — no
 * pick is real until the draft.
 */
export function TeamBuilder({ players, replacementLevels, settings, prep }: Props) {
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

  if (!planned.length) {
    return (
      <div className="rounded-lg bg-card px-4 py-6">
        <h2 className="font-display text-sm font-semibold text-foreground">No team yet</h2>
        <p className="mt-1.5 max-w-xl text-sm text-muted-foreground">
          Add players from the Board's <span className="font-display font-semibold">Plan</span>{' '}
          column — the <span className="font-mono">+</span> starts them at our auction value, and you
          can change the price here. The roster, budget and projected points update as you go.
        </p>
      </div>
    )
  }

  const overBudget = roster.spent > roster.budget

  return (
    <div className="space-y-4">
      {/* Budget and lineup summary */}
      <div className="flex flex-wrap items-start gap-x-10 gap-y-4 rounded-lg bg-card px-4 py-3">
        <Stat
          label="Spent"
          value={`$${roster.spent} / $${roster.budget}`}
          tone={overBudget ? 'text-secondary' : 'text-foreground'}
        />
        <Stat
          label="Remaining"
          value={`$${roster.remaining}`}
          tone={roster.remaining < 0 ? 'text-secondary' : 'text-positive'}
        />
        <Stat label="Max bid" value={`$${roster.maxBid}`} />
        <Stat label="Roster" value={`${planned.length - roster.overflow.length} / ${roster.slots.length}`} />
        <Stat label="Starter pts" value={roster.starterPoints.toFixed(0)} />
        <Stat label="Starter VOR" value={roster.starterVOR.toFixed(0)} />
      </div>

      {roster.warnings.length > 0 && (
        <ul className="space-y-1 rounded-lg bg-negative-light px-4 py-3">
          {roster.warnings.map((w) => (
            <li key={w} className="font-display text-xs font-semibold text-negative-foreground">
              {w}
            </li>
          ))}
        </ul>
      )}

      {/* The roster itself */}
      <div className="rounded-lg bg-card">
        <table className="w-full text-[13px]">
          <thead className="border-b border-border">
            <tr>
              <th className="w-24 px-4 py-2 text-left font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Slot
              </th>
              <th className="px-2 py-2 text-left font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Player
              </th>
              <th className="px-2 py-2 text-right font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Proj
              </th>
              <th className="px-2 py-2 text-right font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Value
              </th>
              <th className="px-2 py-2 text-right font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Cost
              </th>
              <th className="w-8" />
            </tr>
          </thead>
          <tbody>
            {roster.slots.map((s, i) => {
              const p = s.player
              const bench = s.slot === 'BN'
              return (
                <tr key={`${s.slot}-${i}`} className="border-b border-border last:border-0">
                  <td className={`px-4 py-1.5 font-display text-xs font-semibold ${bench ? 'text-muted-foreground' : 'text-foreground'}`}>
                    {SLOT_LABELS[s.slot]}
                  </td>
                  <td className="px-2 py-1.5">
                    {p ? (
                      <RouterLink
                        to={`/players/${p.player.gsis_id}`}
                        className="font-display text-[13px] font-semibold text-foreground hover:underline"
                      >
                        {p.player.name}
                        <span className="ml-1.5 font-mono text-[11px] font-normal text-muted-foreground">
                          {p.player.position_group} · {p.player.team}
                        </span>
                      </RouterLink>
                    ) : (
                      <span className="text-xs text-muted-foreground/50">empty</span>
                    )}
                  </td>
                  <td className="px-2 py-1.5 text-right font-mono tabular-nums text-muted-foreground">
                    {p ? p.player.proj_league_fpts.toFixed(0) : '—'}
                  </td>
                  <td className="px-2 py-1.5 text-right font-mono tabular-nums text-muted-foreground">
                    {p ? `$${p.player.auction_value}` : '—'}
                  </td>
                  <td className="px-2 py-1.5 text-right">
                    {p ? (
                      <CostField
                        value={p.cost}
                        onCommit={(v) => prep.setPlannedCost(p.player.gsis_id, v)}
                      />
                    ) : (
                      <span className="font-mono text-xs text-muted-foreground/50">—</span>
                    )}
                  </td>
                  <td className="pr-3 text-right">
                    {p && (
                      <button
                        onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                        title={`Remove ${p.player.name}`}
                        className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                      >
                        ×
                      </button>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {roster.overflow.length > 0 && (
        <div className="rounded-lg bg-card px-4 py-3">
          <h3 className="font-display text-[11px] font-semibold uppercase tracking-wide text-secondary">
            No roster spot ({roster.overflow.length})
          </h3>
          <ul className="mt-1.5 space-y-1">
            {roster.overflow.map((p) => (
              <li key={p.player.gsis_id} className="flex items-baseline justify-between gap-3">
                <RouterLink
                  to={`/players/${p.player.gsis_id}`}
                  className="font-display text-[13px] font-semibold text-foreground hover:underline"
                >
                  {p.player.name}
                  <span className="ml-1.5 font-mono text-[11px] font-normal text-muted-foreground">
                    {p.player.position_group}
                  </span>
                </RouterLink>
                <span className="flex items-center gap-2">
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">${p.cost}</span>
                  <button
                    onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                    className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                  >
                    ×
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
