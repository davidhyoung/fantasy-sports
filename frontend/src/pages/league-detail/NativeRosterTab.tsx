import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import {
  getLeagueRosters, getLeagueSettings, dropLeagueRoster, updateLeagueRoster, rolloverLeague,
  getLeagueTransactions, type Team, type RosterEntry,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PlayerAssignForm } from './components/PlayerAssignForm'
import { TradeBuilder } from './components/TradeBuilder'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  myTeam: Team | undefined
  format: string
}

function EditContractForm({
  leagueId, entry, onClose,
}: { leagueId: number; entry: RosterEntry; onClose: () => void }) {
  const qc = useQueryClient()
  const [salary, setSalary] = useState(entry.salary)
  const [yearToYear, setYearToYear] = useState(entry.years_total == null)
  const [yearsTotal, setYearsTotal] = useState(entry.years_total ?? 1)

  const mutation = useMutation({
    mutationFn: () =>
      updateLeagueRoster(leagueId, entry.gsis_id, {
        salary,
        years_total: yearToYear ? null : yearsTotal,
        clear_years_total: yearToYear,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      onClose()
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-3">
        <label className="flex flex-col gap-1">
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Salary $</span>
          <input
            type="number" min={0} value={salary}
            onChange={(e) => setSalary(Math.max(0, parseInt(e.target.value, 10) || 0))}
            className="h-8 w-24 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </label>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground mt-4">
          <input type="checkbox" checked={yearToYear} onChange={(e) => setYearToYear(e.target.checked)} />
          Year-to-year
        </label>
        {!yearToYear && (
          <label className="flex flex-col gap-1">
            <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Years total</span>
            <input
              type="number" min={1} max={10} value={yearsTotal}
              onChange={(e) => setYearsTotal(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="h-8 w-16 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
        )}
      </div>
      {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}
      <div className="flex justify-end gap-2 border-t border-border pt-4">
        <Button variant="outline" onClick={onClose}>Cancel</Button>
        <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
          {mutation.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}

/**
 * The native league's roster/contract/trade/rollover surface — the piece
 * that actually lets a dynasty commissioner run their startup auction,
 * manage cap space, trade players and picks, and turn the season over,
 * instead of doing all of it through raw curl calls (which was the only
 * path before this tab existed).
 */
export function NativeRosterTab({ leagueId, active, teams, myTeam, format }: Props) {
  const qc = useQueryClient()
  const [teamId, setTeamId] = useState(myTeam?.id ?? teams[0]?.id ?? 0)
  // `teams` loads asynchronously — if this tab mounts before it resolves,
  // teamId initializes to 0 and never recovers on its own (useState's
  // initializer only runs once). Correct it once real teams arrive.
  useEffect(() => {
    if (teams.length === 0) return
    if (teams.some((t) => t.id === teamId)) return
    setTeamId(myTeam?.id ?? teams[0].id)
  }, [teams, myTeam, teamId])
  const [assigning, setAssigning] = useState(false)
  const [trading, setTrading] = useState(false)
  const [editing, setEditing] = useState<RosterEntry | null>(null)
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)
  const [confirmRollover, setConfirmRollover] = useState(false)

  const { data: rosters, isFetching: loadingRosters } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
    enabled: active,
  })
  const { data: settings } = useQuery({
    queryKey: keys.leagueSettings(leagueId),
    queryFn: () => getLeagueSettings(leagueId),
    enabled: active,
  })
  const { data: transactions = [] } = useQuery({
    queryKey: keys.leagueTransactions(leagueId),
    queryFn: () => getLeagueTransactions(leagueId),
    enabled: active,
  })

  const dropMutation = useMutation({
    mutationFn: (gsisId: string) => dropLeagueRoster(leagueId, gsisId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
      setConfirmDrop(null)
    },
  })

  const rolloverMutation = useMutation({
    mutationFn: () => rolloverLeague(leagueId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.league(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leaguePicks(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
      setConfirmRollover(false)
    },
  })

  const teamRoster = useMemo(
    () => (rosters ?? []).filter((r) => r.team_id === teamId),
    [rosters, teamId]
  )
  const spent = teamRoster.reduce((sum, r) => sum + r.salary, 0)
  const budget = settings?.budget ?? 0
  const remaining = budget - spent
  const rosterSpots = settings ? Object.values(settings.slots).reduce((a, b) => a + b, 0) : 0

  const teamName = useMemo(() => {
    const m = new Map(teams.map((t) => [t.id, t.name]))
    return (id: number) => m.get(id) ?? `Team ${id}`
  }, [teams])

  const summarize = (t: (typeof transactions)[number]) => {
    if (t.kind === 'trade') {
      const moves = (t.payload.moves as { asset: string; from_team_id: number; to_team_id: number }[] | undefined) ?? []
      const teamsInvolved = new Set(moves.flatMap((m) => [m.from_team_id, m.to_team_id]))
      return `Trade between ${[...teamsInvolved].map(teamName).join(' & ')} — ${moves.length} asset${moves.length === 1 ? '' : 's'}`
    }
    if (t.kind === 'rollover') {
      const to = t.payload.to_season as number | undefined
      return to ? `Season rolled over to ${to}` : 'Season rollover'
    }
    if (t.kind === 'draft') return 'Draft pick used'
    return t.kind
  }

  if (!active) return null

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
        <div className="flex flex-wrap gap-1.5">
          {teams.map((t) => (
            <FilterChip key={t.id} active={teamId === t.id} onClick={() => setTeamId(t.id)}>
              {t.name}
            </FilterChip>
          ))}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={() => setTrading(true)}>Trade</Button>
          <Button size="sm" onClick={() => setAssigning(true)}>+ Assign player</Button>
        </div>
      </div>

      {settings && (
        <div className="mb-4 flex flex-wrap gap-4 rounded-lg bg-card px-4 py-3">
          <div>
            <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Budget</div>
            <div className="font-mono text-sm tabular-nums text-foreground">${budget}</div>
          </div>
          <div>
            <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Spent</div>
            <div className="font-mono text-sm tabular-nums text-foreground">${spent}</div>
          </div>
          <div>
            <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Remaining</div>
            <div className={`font-mono text-sm tabular-nums ${remaining < 0 ? 'text-negative' : 'text-foreground'}`}>${remaining}</div>
          </div>
          <div>
            <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Roster</div>
            <div className="font-mono text-sm tabular-nums text-foreground">{teamRoster.length}/{rosterSpots}</div>
          </div>
        </div>
      )}

      {loadingRosters ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : teamRoster.length === 0 ? (
        <p className="text-muted-foreground">No players rostered yet.</p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <TableHead>Player</TableHead>
                <TableHead>Pos</TableHead>
                <TableHead>Slot</TableHead>
                <TableHead className="text-right">Salary</TableHead>
                <TableHead className="text-right">Years</TableHead>
                <TableHead />
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {teamRoster.map((r) => (
                <ClickableRow key={r.gsis_id} href={`/players/${r.gsis_id}`}>
                  <PlayerCell name={r.name} imageUrl={r.headshot_url} linked />
                  <TableCell className="text-muted-foreground">{r.position}</TableCell>
                  <TableCell className="text-muted-foreground">{r.slot}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">${r.salary}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">
                    {r.years_total != null ? `${r.years_used}/${r.years_total}` : 'Y2Y'}
                  </TableCell>
                  <TableCell className="text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                    <Button variant="text" size="bare" onClick={() => setEditing(r)}>Edit</Button>
                    {confirmDrop === r.gsis_id ? (
                      <Button
                        variant="destructive" size="sm" className="ml-2"
                        onClick={() => dropMutation.mutate(r.gsis_id)}
                        disabled={dropMutation.isPending}
                      >
                        Confirm drop
                      </Button>
                    ) : (
                      <Button variant="text" size="bare" className="ml-2 text-negative" onClick={() => setConfirmDrop(r.gsis_id)}>
                        Drop
                      </Button>
                    )}
                  </TableCell>
                </ClickableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Single-user model: whoever's signed in manages every team, and the
          backend's requireCommissioner already gates the actual mutation —
          native league teams don't reliably carry a user_id yet (only
          claimed teams do), so this section shows unconditionally rather
          than hiding behind a client-side ownership check that would often
          be wrong. */}
      <div className="mt-6 flex items-center justify-between rounded-lg border border-border px-4 py-3">
        <div>
          <div className="font-display text-sm font-semibold text-foreground">Season rollover</div>
          <p className="text-xs text-muted-foreground">
            {format === 'dynasty'
              ? 'Contracts carry forward, expiring deals release to free agency, a new rookie draft class is generated.'
              : format === 'redraft'
              ? 'Every roster releases to free agency and a fresh draft class is generated.'
              : 'Rollover is not implemented for keeper-format native leagues yet.'}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={format === 'keeper'}
          onClick={() => setConfirmRollover(true)}
        >
          Roll over season
        </Button>
      </div>

      {transactions.length > 0 && (
        <div className="mt-6">
          <div className="mb-2 font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Activity</div>
          <div className="flex flex-col gap-1.5">
            {transactions.slice(0, 10).map((t, i) => (
              <div key={i} className="flex items-center gap-2 text-xs">
                <Badge variant="neutral">{t.kind}</Badge>
                <span className="text-muted-foreground">{summarize(t)}</span>
                <span className="ml-auto font-mono text-muted-foreground">{new Date(t.created_at).toLocaleString()}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <Dialog open={assigning} onClose={() => setAssigning(false)} title="Assign player">
        <PlayerAssignForm leagueId={leagueId} teams={teams} mode="assign" defaultTeamId={teamId} onClose={() => setAssigning(false)} />
      </Dialog>

      <Dialog open={trading} onClose={() => setTrading(false)} title="Trade">
        <TradeBuilder leagueId={leagueId} teams={teams} onClose={() => setTrading(false)} />
      </Dialog>

      <Dialog open={editing != null} onClose={() => setEditing(null)} title="Edit contract">
        {editing && <EditContractForm leagueId={leagueId} entry={editing} onClose={() => setEditing(null)} />}
      </Dialog>

      <Dialog open={confirmRollover} onClose={() => setConfirmRollover(false)} title="Roll over season">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            {format === 'dynasty'
              ? 'Every contract that reaches its final year releases to free agency; everyone else carries forward with years_used incremented. A new rookie draft class is generated for next season. This cannot be undone.'
              : 'Every rostered player releases to free agency and a fresh draft class is generated for next season. This cannot be undone.'}
          </p>
          {rolloverMutation.error && <p className="text-sm text-destructive">{(rolloverMutation.error as Error).message}</p>}
          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={() => setConfirmRollover(false)}>Cancel</Button>
            <Button variant="destructive" onClick={() => rolloverMutation.mutate()} disabled={rolloverMutation.isPending}>
              {rolloverMutation.isPending ? 'Rolling over…' : 'Roll over'}
            </Button>
          </div>
        </div>
      </Dialog>
    </>
  )
}
