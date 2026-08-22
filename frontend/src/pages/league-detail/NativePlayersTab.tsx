import { useMemo, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import { useQuery } from '@tanstack/react-query'
import { getLeagueRosters, getLeagueFreeAgents, type Team, type PlayerStat } from '../../api/client'
import { keys } from '../../api/queryKeys'
import { SCORING_STATS, SCORING_LABELS, type ScoringStat } from './hooks/useDraftSettings'
import { L4Sparkline } from './components/L4Sparkline'

const POSITIONS = ['QB', 'RB', 'WR', 'TE', 'K', 'DEF']

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
}

function statValue(stats: PlayerStat[] | undefined, stat: ScoringStat): number | undefined {
  return stats?.find((s) => s.stat === stat)?.value
}

/**
 * Players tab for native leagues. `/players` and `/rankings` are Yahoo-backed
 * (VORP, z-scores, real-life grades) and don't apply here — a native league's
 * only fantasy-context data is its own roster and free-agent pool, so this is
 * a simpler view over exactly that: who's rostered where, and who's still out
 * there, ranked by projection. The richer per-position value comparison
 * (VOR, tiers, auction price) already exists for native leagues on the Draft
 * tab — this view is for roster bookkeeping, not evaluation.
 *
 * Stat columns are dynamic — only the categories this league's own scoring
 * actually weights (nonzero) come back from the API at all (see
 * relevantPlayerStats in league_rosters.go), so "relevant" here means
 * "impacts this league's scoring," not just "exists." Column set and order
 * follow SCORING_STATS, same vocabulary the draft-settings panel edits.
 *
 * Free Agents/Rostered is a filled tray, not chips — per the design review's
 * codified rule (open item 8), a tray switches which dataset you're looking
 * at, chips filter the one you're in. Position is a multi-select chip row
 * for the same reason it stayed chips: several positions can be active at
 * once without changing which dataset is showing.
 */
export function NativePlayersTab({ leagueId, active, teams }: Props) {
  const [view, setView] = useState<'free-agents' | 'rostered'>('free-agents')
  const [positions, setPositions] = useState<Set<string>>(new Set())

  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p)
      else next.add(p)
      return next
    })
  }

  // The free-agents endpoint only supports one ?position= value server-side;
  // multi-select filtering happens client-side, same as the rostered view.
  const { data: freeAgents, isFetching: loadingFA } = useQuery({
    queryKey: keys.freeAgents(leagueId, ''),
    queryFn: () => getLeagueFreeAgents(leagueId, '', 200),
    enabled: active && view === 'free-agents',
  })

  const { data: roster, isFetching: loadingRoster } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
    enabled: active && view === 'rostered',
  })

  const teamName = useMemo(() => {
    const m = new Map(teams.map((t) => [t.id, t.name]))
    return (id: number) => m.get(id) ?? `Team ${id}`
  }, [teams])

  const matchesPosition = (position: string) => positions.size === 0 || positions.has(position)

  const faFiltered = (freeAgents ?? []).filter((p) => matchesPosition(p.position))
  const rosteredFiltered = (roster ?? []).filter((r) => matchesPosition(r.position))

  const loading = view === 'free-agents' ? loadingFA : loadingRoster
  const rows = view === 'free-agents' ? faFiltered : rosteredFiltered

  const statColumns = useMemo(() => {
    const present = new Set<string>()
    for (const p of rows) for (const s of p.stats ?? []) present.add(s.stat)
    return SCORING_STATS.filter((s) => present.has(s))
  }, [rows])

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <div className="flex w-fit rounded-lg bg-muted overflow-hidden">
          {([
            { value: 'free-agents' as const, label: 'Free Agents' },
            { value: 'rostered' as const, label: 'Rostered' },
          ]).map((s) => (
            <button
              key={s.value}
              onClick={() => setView(s.value)}
              className={`px-3 py-1.5 font-display text-xs font-semibold ${
                view === s.value ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>
              {positions.has(p) && <span className="mr-1 text-[10px]">✓</span>}
              {p}
            </FilterChip>
          ))}
        </div>
        {!loading && (
          <span className="text-xs text-muted-foreground ml-1">
            {rows.length} player{rows.length !== 1 ? 's' : ''}
          </span>
        )}
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">
          {view === 'free-agents' ? 'No free agents match this filter.' : 'No players rostered yet.'}
        </p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <TableHead>Player</TableHead>
                <TableHead>Team</TableHead>
                <TableHead>Pos</TableHead>
                {statColumns.map((s) => (
                  <TableHead key={s} className="text-right">{SCORING_LABELS[s]}</TableHead>
                ))}
                <TableHead className="text-center">L4 wks</TableHead>
                {view === 'free-agents' ? (
                  <TableHead className="text-right">Proj Pts</TableHead>
                ) : (
                  <>
                    <TableHead>Rostered By</TableHead>
                    <TableHead className="text-right">Salary</TableHead>
                    <TableHead className="text-right">Years</TableHead>
                  </>
                )}
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {view === 'free-agents'
                ? (rows as NonNullable<typeof freeAgents>).map((p) => (
                    <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}?league=${leagueId}`}>
                      <PlayerCell name={p.name} imageUrl={p.headshot_url} linked />
                      <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                      <TableCell className="text-muted-foreground">{p.position}</TableCell>
                      {statColumns.map((s) => {
                        const v = statValue(p.stats, s)
                        return (
                          <TableCell key={s} className="text-right font-mono tabular-nums">
                            {v != null ? v.toFixed(1) : '—'}
                          </TableCell>
                        )
                      })}
                      <TableCell className="text-center"><L4Sparkline trend={p.trend} /></TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {p.proj_fpts_ppr != null ? p.proj_fpts_ppr.toFixed(1) : '—'}
                      </TableCell>
                    </ClickableRow>
                  ))
                : (rows as NonNullable<typeof roster>).map((p) => (
                    <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}?league=${leagueId}`}>
                      <PlayerCell name={p.name} imageUrl={p.headshot_url} linked />
                      <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                      <TableCell className="text-muted-foreground">{p.position}</TableCell>
                      {statColumns.map((s) => {
                        const v = statValue(p.stats, s)
                        return (
                          <TableCell key={s} className="text-right font-mono tabular-nums">
                            {v != null ? v.toFixed(1) : '—'}
                          </TableCell>
                        )
                      })}
                      <TableCell className="text-center"><L4Sparkline trend={p.trend} /></TableCell>
                      <TableCell className="text-muted-foreground">{teamName(p.team_id)}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">${p.salary}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {p.years_total != null ? `${p.years_used}/${p.years_total}` : '—'}
                      </TableCell>
                    </ClickableRow>
                  ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
