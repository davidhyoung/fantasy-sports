import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { SelectControl } from '@/components/ui/filter-chip'
import {
  createLeagueTrade, getLeagueRosters, getLeagueDraftPicks,
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

  const { data: rosters = [] } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
  })
  const { data: picks = [] } = useQuery({
    queryKey: keys.leaguePicks(leagueId),
    queryFn: () => getLeagueDraftPicks(leagueId),
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

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-4">
        <Side label="Team A sends" teamId={teamAId} setTeamId={setTeamAId} otherTeamId={teamBId} assets={assetsA} selected={fromA} setSelected={setFromA} />
        <Side label="Team B sends" teamId={teamBId} setTeamId={setTeamBId} otherTeamId={teamAId} assets={assetsB} selected={fromB} setSelected={setFromB} />
      </div>

      {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}

      <div className="flex justify-end gap-2 border-t border-border pt-4">
        <Button variant="outline" onClick={onClose}>Cancel</Button>
        <Button onClick={() => mutation.mutate()} disabled={!canSubmit || mutation.isPending}>
          {mutation.isPending ? 'Trading…' : 'Execute trade'}
        </Button>
      </div>
    </div>
  )
}
