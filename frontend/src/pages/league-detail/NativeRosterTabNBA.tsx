import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { HeaderRow, PlayerCell, TeamAvatar } from '@/components/ui/table-helpers'
import { SelectControl } from '@/components/ui/filter-chip'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  getLeagueRostersNBA, getLeagueFreeAgentsNBA, assignLeagueRosterNBA, updateLeagueRosterNBA, dropLeagueRosterNBA,
  NBA_SLOT_POSITIONS, NBA_SLOT_LABELS, NBA_CATEGORY_LABELS, type NBASlotPosition, type Team,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  myTeam: Team | undefined
}

/**
 * Roster tab for a native NBA league — a team switcher (defaults to your
 * claimed team) plus that team's roster and a free-agent add panel. No
 * contracts/cap/trade this phase (deferred, see .claude/plans) — a plain
 * assign/drop roster is enough for category scoring to mean something.
 */
export function NativeRosterTabNBA({ leagueId, active, teams, myTeam }: Props) {
  const qc = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const teamParam = Number(searchParams.get('team'))
  const teamId = teams.some((t) => t.id === teamParam) ? teamParam : (myTeam?.id ?? teams[0]?.id ?? 0)
  const selectTeam = (id: number) =>
    setSearchParams((prev) => { prev.set('team', String(id)); return prev }, { replace: true })
  const [search, setSearch] = useState('')

  const { data: roster } = useQuery({
    queryKey: keys.leagueRostersNBA(leagueId),
    queryFn: () => getLeagueRostersNBA(leagueId),
    enabled: active,
  })
  const { data: freeAgents } = useQuery({
    queryKey: keys.freeAgentsNBA(leagueId, '', search),
    queryFn: () => getLeagueFreeAgentsNBA(leagueId, '', 50, 0, search),
    enabled: active,
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'rosters-nba'] })
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents-nba'] })
  }

  const assignMutation = useMutation({
    mutationFn: (espnId: string) => assignLeagueRosterNBA(leagueId, { espn_id: espnId, team_id: teamId, slot: 'BN', acquired_via: 'fa' }),
    onSuccess: invalidate,
  })
  const slotMutation = useMutation({
    mutationFn: ({ espnId, slot }: { espnId: string; slot: NBASlotPosition }) => updateLeagueRosterNBA(leagueId, espnId, { slot }),
    onSuccess: invalidate,
  })
  const dropMutation = useMutation({
    mutationFn: (espnId: string) => dropLeagueRosterNBA(leagueId, espnId),
    onSuccess: invalidate,
  })

  if (!active) return null

  const teamRoster = (roster ?? []).filter((r) => r.team_id === teamId)
  const selectedTeam = teams.find((t) => t.id === teamId)

  return (
    <>
      <div className="mb-4 flex items-center gap-2">
        {selectedTeam && <TeamAvatar name={selectedTeam.name} />}
        <SelectControl value={teamId} onChange={(e) => selectTeam(Number(e.target.value))}>
          {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </SelectControl>
      </div>

      <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
        <Table>
          <TableHeader style={{ top: 0 }}>
            <HeaderRow>
              <TableHead>Slot</TableHead>
              <TableHead>Player</TableHead>
              <TableHead>Team</TableHead>
              {Object.values(NBA_CATEGORY_LABELS).map((label) => (
                <TableHead key={label} className="text-right">{label}</TableHead>
              ))}
              <TableHead />
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {teamRoster.length === 0 ? (
              <TableRow><TableCell colSpan={13} className="text-muted-foreground">No players rostered yet.</TableCell></TableRow>
            ) : teamRoster.map((r) => (
              <TableRow key={r.espn_id}>
                <TableCell>
                  <select
                    value={r.slot}
                    onChange={(e) => slotMutation.mutate({ espnId: r.espn_id, slot: e.target.value as NBASlotPosition })}
                    className="h-7 rounded-md border border-input bg-background px-1.5 font-mono text-xs"
                  >
                    {NBA_SLOT_POSITIONS.map((s) => <option key={s} value={s}>{NBA_SLOT_LABELS[s]}</option>)}
                  </select>
                </TableCell>
                <PlayerCell name={r.name} imageUrl={r.headshot_url} />
                <TableCell className="text-muted-foreground">{r.team} · {r.position}</TableCell>
                {Object.keys(NBA_CATEGORY_LABELS).map((cat) => {
                  const v = r.stats?.find((s) => s.category === cat)?.value
                  return (
                    <TableCell key={cat} className="text-right font-mono tabular-nums">
                      {v != null ? (cat === 'fg_pct' || cat === 'ft_pct' ? v.toFixed(3) : v) : '—'}
                    </TableCell>
                  )
                })}
                <TableCell>
                  <Button variant="text" size="bare" onClick={() => dropMutation.mutate(r.espn_id)}>Drop</Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <div className="mt-6">
        <div className="mb-2 flex items-center justify-between">
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Free agents</span>
          <Input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search players…" className="w-48" />
        </div>
        <div className="flex flex-col gap-1.5">
          {(freeAgents ?? []).map((p) => (
            <div key={p.espn_id} className="flex items-center justify-between rounded-md bg-card px-3 py-1.5">
              <PlayerCell name={p.name} imageUrl={p.headshot_url} />
              <span className="text-xs text-muted-foreground">{p.team} · {p.position}</span>
              <Button variant="outline" size="sm" onClick={() => assignMutation.mutate(p.espn_id)} disabled={assignMutation.isPending}>
                + Add
              </Button>
            </div>
          ))}
        </div>
      </div>
    </>
  )
}
