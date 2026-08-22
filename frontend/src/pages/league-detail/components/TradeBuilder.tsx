import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { SelectControl } from '@/components/ui/filter-chip'
import {
  createLeagueTrade, getLeagueRosters, getLeagueDraftPicks, getLeagueSettings,
  type Team, type TradeAsset,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  leagueId: number
  teams: Team[]
  onClose: () => void
}

type SideAsset = { key: string; kind: 'player' | 'pick'; gsisId?: string; pickId?: number; label: string }

/**
 * A straight two-team swap — everything Team A picks send moves to Team B
 * and vice versa. The backend trade endpoint supports arbitrary N-team,
 * N-asset trades, but a 2-team swap is what actually gets used; the UI is
 * scoped to that instead of building a picker for a shape nobody needs yet.
 */
export function TradeBuilder({ leagueId, teams, onClose }: Props) {
  const qc = useQueryClient()
  const [teamAId, setTeamAId] = useState(teams[0]?.id ?? 0)
  const [teamBId, setTeamBId] = useState(teams[1]?.id ?? teams[0]?.id ?? 0)
  const [fromA, setFromA] = useState<Set<string>>(new Set())
  const [fromB, setFromB] = useState<Set<string>>(new Set())
  // Executing immediately with no confirmation was flagged as the only
  // irreversible action with no gate — "trade it back" isn't a real undo
  // here, it re-executes contracts and leaves two extra rows in the
  // activity log that misdescribe what happened. So: a review step, not a
  // second modal — the same dialog swaps its body for a plain summary
  // before the mutation actually fires (design review open item 5).
  const [reviewing, setReviewing] = useState(false)
  // Mobile only — desktop shows both sides side-by-side, but there's no room
  // for that below md, so a tab toggle picks which side is visible.
  const [activeTeam, setActiveTeam] = useState<'A' | 'B'>('A')

  const { data: rosters = [] } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
  })
  const { data: picks = [] } = useQuery({
    queryKey: keys.leaguePicks(leagueId),
    queryFn: () => getLeagueDraftPicks(leagueId),
  })
  const { data: settings } = useQuery({
    queryKey: keys.leagueSettings(leagueId),
    queryFn: () => getLeagueSettings(leagueId),
  })

  const assetsFor = (teamId: number): SideAsset[] => {
    const players: SideAsset[] = rosters
      .filter((r) => r.team_id === teamId)
      .map((r) => ({ key: `player:${r.gsis_id}`, kind: 'player', gsisId: r.gsis_id, label: `${r.name} (${r.position})` }))
    const teamPicks: SideAsset[] = picks
      .filter((p) => p.current_team_id === teamId && !p.used_on_gsis_id)
      .map((p) => ({ key: `pick:${p.id}`, kind: 'pick', pickId: p.id, label: `${p.season} Round ${p.round} pick` }))
    return [...players, ...teamPicks]
  }

  const assetsA = useMemo(() => assetsFor(teamAId), [rosters, picks, teamAId])
  const assetsB = useMemo(() => assetsFor(teamBId), [rosters, picks, teamBId])

  const toggle = (set: Set<string>, setSet: (s: Set<string>) => void, key: string) => {
    const next = new Set(set)
    if (next.has(key)) next.delete(key)
    else next.add(key)
    setSet(next)
  }

  const mutation = useMutation({
    mutationFn: () => {
      const assets: TradeAsset[] = []
      for (const a of assetsA) {
        if (!fromA.has(a.key)) continue
        assets.push(a.kind === 'player'
          ? { kind: 'player', gsis_id: a.gsisId, to_team_id: teamBId }
          : { kind: 'pick', pick_id: a.pickId, to_team_id: teamBId })
      }
      for (const a of assetsB) {
        if (!fromB.has(a.key)) continue
        assets.push(a.kind === 'player'
          ? { kind: 'player', gsis_id: a.gsisId, to_team_id: teamAId }
          : { kind: 'pick', pick_id: a.pickId, to_team_id: teamAId })
      }
      return createLeagueTrade(leagueId, assets)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leaguePicks(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
      onClose()
    },
  })

  const canSubmit = teamAId !== teamBId && (fromA.size > 0 || fromB.size > 0)

  const capAfter = (teamId: number, sending: Set<string>, sendingAssets: SideAsset[], receivingAssets: SideAsset[], receiving: Set<string>) => {
    const spent = rosters.filter((r) => r.team_id === teamId).reduce((sum, r) => sum + r.salary, 0)
    const outgoingSalary = sendingAssets
      .filter((a) => sending.has(a.key) && a.kind === 'player')
      .reduce((sum, a) => sum + (rosters.find((r) => r.gsis_id === a.gsisId)?.salary ?? 0), 0)
    const incomingSalary = receivingAssets
      .filter((a) => receiving.has(a.key) && a.kind === 'player')
      .reduce((sum, a) => sum + (rosters.find((r) => r.gsis_id === a.gsisId)?.salary ?? 0), 0)
    const budget = settings?.budget ?? 0
    return budget - (spent - outgoingSalary + incomingSalary)
  }

  const teamName = (id: number) => teams.find((t) => t.id === id)?.name ?? `Team ${id}`
  const label = (asset: SideAsset) => asset.label
  const sentA = assetsA.filter((a) => fromA.has(a.key))
  const sentB = assetsB.filter((a) => fromB.has(a.key))
  const capA = capAfter(teamAId, fromA, assetsA, assetsB, fromB)
  const capB = capAfter(teamBId, fromB, assetsB, assetsA, fromA)

  const Side = ({
    label, teamId, setTeamId, otherTeamId, assets, selected, setSelected,
  }: {
    label: string
    teamId: number
    setTeamId: (id: number) => void
    otherTeamId: number
    assets: SideAsset[]
    selected: Set<string>
    setSelected: (s: Set<string>) => void
  }) => (
    <div className="flex-1 min-w-0">
      <SelectControl label={label} value={teamId} onChange={(e) => setTeamId(Number(e.target.value))}>
        {teams.filter((t) => t.id !== otherTeamId).map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
      </SelectControl>
      <div className="mt-2 max-h-56 overflow-y-auto rounded-md border border-border">
        {assets.length === 0 ? (
          <p className="p-3 text-xs text-muted-foreground">No tradable assets.</p>
        ) : (
          assets.map((a) => (
            <label key={a.key} className="flex items-center gap-2 border-b border-border px-2.5 py-1.5 text-xs last:border-0 hover:bg-muted">
              <input
                type="checkbox"
                checked={selected.has(a.key)}
                onChange={() => toggle(selected, setSelected, a.key)}
              />
              <span className="truncate text-foreground">{a.label}</span>
            </label>
          ))
        )}
      </div>
    </div>
  )

  if (reviewing) {
    return (
      <div className="flex flex-col gap-4">
        <p className="text-xs text-muted-foreground">Executes immediately — both sides are yours.</p>
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div className="rounded-lg border border-border p-4">
            <div className="font-display text-sm font-bold text-foreground">{teamName(teamAId)} sends</div>
            <ul className="mt-2 flex flex-col gap-1 text-xs text-foreground">
              {sentA.length === 0 ? <li className="text-muted-foreground">Nothing</li> : sentA.map((a) => <li key={a.key}>{label(a)}</li>)}
            </ul>
            <div className={`mt-3 font-mono text-[11px] ${capA < 0 ? 'text-negative' : 'text-muted-foreground'}`}>
              Cap after: ${capA}{capA < 0 ? ' · over budget' : ''}
            </div>
          </div>
          <div className="rounded-lg border border-border p-4">
            <div className="font-display text-sm font-bold text-foreground">{teamName(teamBId)} sends</div>
            <ul className="mt-2 flex flex-col gap-1 text-xs text-foreground">
              {sentB.length === 0 ? <li className="text-muted-foreground">Nothing</li> : sentB.map((a) => <li key={a.key}>{label(a)}</li>)}
            </ul>
            <div className={`mt-3 font-mono text-[11px] ${capB < 0 ? 'text-negative' : 'text-muted-foreground'}`}>
              Cap after: ${capB}{capB < 0 ? ' · over budget' : ''}
            </div>
          </div>
        </div>

        {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}

        <div className="flex justify-between gap-2 border-t border-border pt-4">
          <Button variant="outline" onClick={() => setReviewing(false)}>Back</Button>
          <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            {mutation.isPending ? 'Trading…' : 'Confirm trade'}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-4">
      {/* Mobile only — desktop shows both sides at once, so there's nothing
       *  to switch between at md+. */}
      <div className="flex gap-1 rounded-lg bg-muted p-1 md:hidden">
        <button
          onClick={() => setActiveTeam('A')}
          className={`flex-1 rounded-md px-3 py-2 font-display text-xs font-semibold ${
            activeTeam === 'A' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'
          }`}
        >
          {teamName(teamAId)} sends
        </button>
        <button
          onClick={() => setActiveTeam('B')}
          className={`flex-1 rounded-md px-3 py-2 font-display text-xs font-semibold ${
            activeTeam === 'B' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground'
          }`}
        >
          {teamName(teamBId)} sends
        </button>
      </div>

      <div className="flex flex-col gap-4 md:flex-row">
        <div className={activeTeam === 'A' ? 'block' : 'hidden md:block'}>
          <Side label="Team A sends" teamId={teamAId} setTeamId={setTeamAId} otherTeamId={teamBId} assets={assetsA} selected={fromA} setSelected={setFromA} />
        </div>
        <div className={activeTeam === 'B' ? 'block' : 'hidden md:block'}>
          <Side label="Team B sends" teamId={teamBId} setTeamId={setTeamBId} otherTeamId={teamAId} assets={assetsB} selected={fromB} setSelected={setFromB} />
        </div>
      </div>

      {/* Mobile only — desktop's cap math only shows up at the review step;
       *  below md, both teams' running cap stays visible regardless of which
       *  tab is active, so switching tabs never hides the numbers you're
       *  deciding against. */}
      <div className="flex flex-col gap-1 border-t border-border pt-3 font-mono text-[11px] md:hidden">
        <div className={capA < 0 ? 'text-negative' : 'text-muted-foreground'}>
          {teamName(teamAId)}: cap after ${capA}{capA < 0 ? ' · over budget' : ''}
        </div>
        <div className={capB < 0 ? 'text-negative' : 'text-muted-foreground'}>
          {teamName(teamBId)}: cap after ${capB}{capB < 0 ? ' · over budget' : ''}
        </div>
      </div>

      <div className="flex justify-end gap-2 border-t border-border pt-4">
        <Button variant="outline" onClick={onClose}>Cancel</Button>
        <Button onClick={() => setReviewing(true)} disabled={!canSubmit}>
          Review trade →
        </Button>
      </div>
    </div>
  )
}
