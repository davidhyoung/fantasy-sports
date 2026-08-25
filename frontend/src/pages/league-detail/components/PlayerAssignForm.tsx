import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { SelectControl } from '@/components/ui/filter-chip'
import {
  assignLeagueRoster, getLeagueFreeAgents, useLeagueDraftPick,
  type Team, type FreeAgent,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { ROSTER_SLOTS, isSlotEligible } from '../lib/nativeSlots'

const ACQUIRED_VIA = ['auction', 'draft', 'fa', 'waiver', 'trade', 'keeper'] as const

interface Props {
  leagueId: number
  teams: Team[]
  onClose: () => void
  /** Assign a free agent straight to a roster. */
  mode: 'assign'
  defaultTeamId: number
}

interface PickProps {
  leagueId: number
  teams: Team[]
  onClose: () => void
  /** Spend a draft pick on a player — team is fixed to the pick's current owner. */
  mode: 'pick'
  pickId: number
  teamId: number
}

/**
 * Shared by the roster tab's "Assign player" flow and the draft-picks tab's
 * "Use pick" flow — both are "search free agents, pick one, sign them to a
 * contract," just with a different write at the end (POST /rosters vs.
 * POST /picks/{id}/use) and acquired_via fixed to "draft" in pick mode.
 */
export function PlayerAssignForm(props: Props | PickProps) {
  const { leagueId, teams, onClose } = props
  const qc = useQueryClient()

  const [search, setSearch] = useState('')
  const [position, setPosition] = useState('')
  const [selected, setSelected] = useState<FreeAgent | null>(null)
  const [teamId, setTeamId] = useState(props.mode === 'assign' ? props.defaultTeamId : props.teamId)
  const [slot, setSlot] = useState<string>('BN')
  const [salary, setSalary] = useState(0)
  const [yearToYear, setYearToYear] = useState(true)
  const [yearsTotal, setYearsTotal] = useState(1)
  const [acquiredVia, setAcquiredVia] = useState<string>('auction')

  const { data: candidates = [], isFetching } = useQuery({
    queryKey: keys.freeAgents(leagueId, position, search),
    queryFn: () => getLeagueFreeAgents(leagueId, position, 25, 0, search),
    enabled: search.trim().length > 0 || position !== '',
  })

  const mutation = useMutation({
    mutationFn: () => {
      if (props.mode === 'pick') {
        // Salary and contract length are derived server-side from the
        // pick's own draft slot (the rookie scale) — see useLeagueDraftPick.
        return useLeagueDraftPick(leagueId, props.pickId, { gsis_id: selected!.gsis_id, slot })
      }
      const years = yearToYear ? null : yearsTotal
      return assignLeagueRoster(leagueId, {
        gsis_id: selected!.gsis_id, team_id: teamId, slot, acquired_via: acquiredVia, salary, years_total: years,
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
      if (props.mode === 'pick') qc.invalidateQueries({ queryKey: keys.leaguePicks(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'team'] })
      onClose()
    },
  })

  return (
    <div className="flex flex-col gap-4">
      {!selected ? (
        <>
          <Input
            autoFocus
            placeholder="Search free agents by name…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="flex flex-wrap gap-1.5">
            {['', 'QB', 'RB', 'WR', 'TE', 'K'].map((p) => (
              <button
                key={p || 'all'}
                onClick={() => setPosition(p)}
                className={`rounded-pill border px-2.5 py-1 font-display text-[11px] font-semibold ${
                  position === p ? 'border-positive-border bg-positive-light text-primary' : 'border-border text-muted-foreground'
                }`}
              >
                {p || 'All'}
              </button>
            ))}
          </div>
          <div className="max-h-64 overflow-y-auto rounded-md border border-border">
            {isFetching ? (
              <p className="p-3 text-xs text-muted-foreground">Searching…</p>
            ) : candidates.length === 0 ? (
              <p className="p-3 text-xs text-muted-foreground">
                {search.trim() || position ? 'No free agents match.' : 'Type a name or pick a position to search.'}
              </p>
            ) : (
              candidates.map((c) => (
                <button
                  key={c.gsis_id}
                  onClick={() => {
                    setSelected(c)
                    if (!isSlotEligible(slot, c.position)) setSlot('BN')
                  }}
                  className="flex w-full items-center gap-2.5 border-b border-border px-3 py-2 text-left last:border-0 hover:bg-muted"
                >
                  <PlayerAvatar src={c.headshot_url} alt={c.name} size={28} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-display text-[13px] font-semibold text-foreground">{c.name}</div>
                    <div className="text-xs text-muted-foreground">{c.position} · {c.team || 'FA'}</div>
                  </div>
                  {c.proj_fpts_ppr != null && (
                    <span className="font-mono text-xs tabular-nums text-muted-foreground">{c.proj_fpts_ppr.toFixed(1)}</span>
                  )}
                </button>
              ))
            )}
          </div>
        </>
      ) : (
        <>
          <div className="flex items-center gap-2.5 rounded-md border border-border px-3 py-2">
            <PlayerAvatar src={selected.headshot_url} alt={selected.name} size={32} />
            <div className="min-w-0 flex-1">
              <div className="font-display text-sm font-semibold text-foreground">{selected.name}</div>
              <div className="text-xs text-muted-foreground">{selected.position} · {selected.team || 'FA'}</div>
            </div>
            <Button variant="text" size="bare" onClick={() => setSelected(null)}>Change</Button>
          </div>

          <div className="grid grid-cols-2 gap-3">
            {props.mode === 'assign' && (
              <SelectControl label="Team" value={teamId} onChange={(e) => setTeamId(Number(e.target.value))}>
                {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </SelectControl>
            )}
            <SelectControl label="Slot" value={slot} onChange={(e) => setSlot(e.target.value)}>
              {ROSTER_SLOTS.filter((s) => isSlotEligible(s, selected.position)).map((s) => (
                <option key={s} value={s}>{s}</option>
              ))}
            </SelectControl>
            {props.mode === 'assign' && (
              <>
                <SelectControl label="Acquired via" value={acquiredVia} onChange={(e) => setAcquiredVia(e.target.value)}>
                  {ACQUIRED_VIA.map((a) => <option key={a} value={a}>{a}</option>)}
                </SelectControl>
                <label className="flex flex-col gap-1">
                  <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Salary $</span>
                  <input
                    type="number"
                    min={0}
                    value={salary}
                    onChange={(e) => setSalary(Math.max(0, parseInt(e.target.value, 10) || 0))}
                    className="h-8 w-full rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </label>
              </>
            )}
          </div>

          {props.mode === 'assign' && (
            <div className="flex items-center gap-3">
              <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <input type="checkbox" checked={yearToYear} onChange={(e) => setYearToYear(e.target.checked)} />
                Year-to-year
              </label>
              {!yearToYear && (
                <label className="flex items-center gap-1.5">
                  <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Years</span>
                  <input
                    type="number"
                    min={1}
                    max={10}
                    value={yearsTotal}
                    onChange={(e) => setYearsTotal(Math.max(1, parseInt(e.target.value, 10) || 1))}
                    className="h-8 w-16 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  />
                </label>
              )}
            </div>
          )}

          {props.mode === 'pick' && (
            <p className="text-xs text-muted-foreground">
              Salary and contract length are set by this pick's draft slot, not chosen here.
            </p>
          )}

          {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}

          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={onClose}>Cancel</Button>
            <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
              {mutation.isPending ? 'Saving…' : props.mode === 'pick' ? 'Use pick' : 'Assign'}
            </Button>
          </div>
        </>
      )}
    </div>
  )
}
