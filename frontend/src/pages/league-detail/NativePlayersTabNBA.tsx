import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import {
  getLeagueRostersNBA, getLeagueFreeAgentsNBA, NBA_CATEGORY_LABELS,
  NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS, type NBAStatPeriod, type NBARosterEntry, type NBAFreeAgent, type Team,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

const POSITIONS = ['G', 'F', 'C']
type ViewOption = 'free-agents' | 'rostered' | 'all'
const VIEW_OPTIONS: { value: ViewOption; label: string }[] = [
  { value: 'free-agents', label: 'Free Agents' },
  { value: 'rostered', label: 'Rostered' },
  { value: 'all', label: 'All' },
]
// A free agent has no team_id; normalizing it onto both shapes lets "All"
// concatenate them into one typed, uniformly-rendered row list.
type PlayerRow = (NBARosterEntry | (NBAFreeAgent & { team_id?: undefined })) & { team_id?: number }

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
}

/**
 * Players tab for a native NBA league — Free Agents/Rostered/All toggle,
 * same "filled tray, not chips" convention as the NFL version, plus a
 * timeframe picker (see NBA_STAT_PERIODS) that drives both queries. Category
 * columns are fixed (all 9 roto categories — no per-league "relevant"
 * filtering, unlike the NFL side's scored-stats-only columns, since a native
 * NBA league's scoring.json controls week-to-week category inclusion, not
 * whether a category has any meaning to show at all).
 */
export function NativePlayersTabNBA({ leagueId, active, teams }: Props) {
  const [view, setView] = useState<ViewOption>('free-agents')
  const [period, setPeriod] = useState<NBAStatPeriod>('season')
  const [positions, setPositions] = useState<Set<string>>(new Set())

  const faEnabled = active && (view === 'free-agents' || view === 'all')
  const rosterEnabled = active && (view === 'rostered' || view === 'all')

  const { data: freeAgents, isFetching: loadingFA } = useQuery({
    queryKey: keys.freeAgentsNBAView(leagueId, '', '', period),
    queryFn: () => getLeagueFreeAgentsNBA(leagueId, '', 200, 0, '', period),
    enabled: faEnabled,
  })
  const { data: roster, isFetching: loadingRoster } = useQuery({
    queryKey: keys.leagueRostersNBAView(leagueId, period),
    queryFn: () => getLeagueRostersNBA(leagueId, period),
    enabled: rosterEnabled,
  })

  if (!active) return null

  const teamName = (id: number) => teams.find((t) => t.id === id)?.name ?? `Team ${id}`
  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }
  const matchesPosition = (position: string) => positions.size === 0 || positions.has(position)

  const loading = view === 'free-agents' ? loadingFA : view === 'rostered' ? loadingRoster : loadingFA || loadingRoster
  // Free agents and rostered players are mutually exclusive sets by
  // definition (free-agents-nba is nba_players minus league_rosters_nba),
  // so "All" is a plain concatenation — no dedup needed.
  const baseRows: PlayerRow[] =
    view === 'free-agents' ? (freeAgents ?? [])
    : view === 'rostered' ? (roster ?? [])
    : [...(roster ?? []), ...(freeAgents ?? [])]
  const rows = baseRows.filter((p) => matchesPosition(p.position))
  const showRosteredBy = view === 'rostered' || view === 'all'

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <div className="flex w-fit flex-none rounded-lg bg-muted overflow-hidden">
          {VIEW_OPTIONS.map((s) => (
            <button
              key={s.value}
              onClick={() => setView(s.value)}
              className={`px-3 py-1.5 font-display text-xs font-semibold whitespace-nowrap ${
                view === s.value ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex w-fit flex-none rounded-lg bg-muted overflow-hidden">
          {NBA_STAT_PERIODS.map((p) => (
            <button
              key={p}
              onClick={() => setPeriod(p)}
              className={`px-3 py-1.5 font-display text-xs font-semibold whitespace-nowrap ${
                period === p ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {NBA_STAT_PERIOD_LABELS[p]}
            </button>
          ))}
        </div>
        <div className="flex flex-none gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>{p}</FilterChip>
          ))}
        </div>
        {!loading && <span className="text-xs text-muted-foreground ml-1">{rows.length} players</span>}
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">{view === 'free-agents' ? 'No free agents match this filter.' : view === 'rostered' ? 'No players rostered yet.' : 'No players match this filter.'}</p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <TableHead>Player</TableHead>
                <TableHead>Team</TableHead>
                <TableHead>Pos</TableHead>
                {Object.values(NBA_CATEGORY_LABELS).map((label) => (
                  <TableHead key={label} className="text-right">{label}</TableHead>
                ))}
                {showRosteredBy && <TableHead>Rostered By</TableHead>}
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <ClickableRow key={p.espn_id}>
                  <PlayerCell name={p.name} imageUrl={p.headshot_url} />
                  <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                  <TableCell className="text-muted-foreground">{p.position}</TableCell>
                  {Object.keys(NBA_CATEGORY_LABELS).map((cat) => {
                    const v = p.stats?.find((s) => s.category === cat)?.value
                    const isPct = cat === 'fg_pct' || cat === 'ft_pct'
                    return (
                      <TableCell key={cat} className="text-right font-mono tabular-nums">
                        {v != null ? v.toFixed(isPct ? 3 : period === 'season_avg' ? 1 : 0) : '—'}
                      </TableCell>
                    )
                  })}
                  {showRosteredBy && (
                    <TableCell className="text-muted-foreground">
                      {p.team_id != null ? teamName(p.team_id) : 'Free Agent'}
                    </TableCell>
                  )}
                </ClickableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
