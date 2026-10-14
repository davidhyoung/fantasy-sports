import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort, ZScoreCell, type SortDir } from '@/components/ui/table-helpers'
import { FilterChip, SelectControl } from '@/components/ui/filter-chip'
import { Input } from '@/components/ui/input'
import {
  getLeagueRostersNBA, getLeagueFreeAgentsNBA, getLeagueSettings, NBA_CATEGORY_LABELS,
  NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS, type NBACategory, type NBAStatPeriod, type Team,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { NBAPlayerDetailPanel } from './components/NBAPlayerDetailPanel'
import {
  NBA_RANK_CATEGORIES as CATEGORIES, nbaStatValue as statValue, nbaRosteredPlayerPool as rosteredPlayerPool,
  nbaComputeCategoryStats as computeCategoryStats, nbaFantasyZScore as fantasyZScore, nbaComputeRanks as computeRanks,
  type NBAPlayerRow as PlayerRow,
} from './lib/nbaRanking'
import { loadPersistedFilters, savePersistedFilters } from '@/lib/persistedFilters'

const POSITIONS = ['G', 'F', 'C']
type ViewOption = 'free-agents' | 'rostered' | 'all'
const VIEW_OPTIONS: { value: ViewOption; label: string }[] = [
  { value: 'free-agents', label: 'Free Agents' },
  { value: 'rostered', label: 'Rostered' },
  { value: 'all', label: 'All' },
]

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
}

interface SavedFilters {
  view: ViewOption
  period: NBAStatPeriod
  positions: string[]
  search: string
  sortCol: string
  sortDir: SortDir
}

const filtersStorageKey = (leagueId: number) => `fs.players-nba.${leagueId}.filters`

function rowSearchText(row: PlayerRow, rosteredByLabel: string) {
  const statText = CATEGORIES.map((cat) => {
    const v = row.stats?.find((s) => s.category === cat)?.value
    if (v == null) return ''
    return cat === 'fg_pct' || cat === 'ft_pct' ? v.toFixed(3) : String(v)
  }).join(' ')
  return [row.name, row.team, row.position, rosteredByLabel, statText].join(' ').toLowerCase()
}

/**
 * Players tab for a native NBA league — Free Agents/Rostered/All toggle,
 * same "filled tray, not chips" convention as the NFL version, plus a
 * timeframe picker (see NBA_STAT_PERIODS), a cross-attribute search box, a
 * client-computed Rank column (see computeRanks, default sort), and sortable
 * columns throughout. Category columns are fixed (all 9 roto categories —
 * no per-league "relevant" filtering, unlike the NFL side's scored-stats-only
 * columns, since a native NBA league's scoring.json controls week-to-week
 * category inclusion, not whether a category has any meaning to show at all).
 */
export function NativePlayersTabNBA({ leagueId, active, teams }: Props) {
  // Read once on mount (leagueId is stable for the component's lifetime) —
  // every filter below seeds from whatever was last saved for this league.
  const saved = useMemo(() => loadPersistedFilters<SavedFilters>(filtersStorageKey(leagueId)), [leagueId])
  const [view, setView] = useState<ViewOption>(saved.view ?? 'free-agents')
  const [period, setPeriod] = useState<NBAStatPeriod>(saved.period ?? 'season')
  const [positions, setPositions] = useState<Set<string>>(new Set(saved.positions ?? []))
  const [search, setSearch] = useState(saved.search ?? '')
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)
  const [detailPeriod, setDetailPeriod] = useState<NBAStatPeriod>('season')
  const { sortCol, sortDir, handleSort } = useTableSort(
    saved.sortCol ?? 'rank', saved.sortDir ?? 'asc', ['rank', 'player', 'team', 'pos', 'rostered_by']
  )

  // Persisted per league — not the detail panel's own period, which is a
  // separate, per-player choice.
  useEffect(() => {
    savePersistedFilters<SavedFilters>(filtersStorageKey(leagueId), { view, period, positions: [...positions], search, sortCol, sortDir })
  }, [leagueId, view, period, positions, search, sortCol, sortDir])

  // Both lists are always fetched (not gated by `view`) because the
  // "average rostered player" baseline below needs the full league-wide
  // pool regardless of which view the user happens to be browsing.
  const { data: freeAgents, isFetching: loadingFA } = useQuery({
    queryKey: keys.freeAgentsNBAView(leagueId, '', '', period),
    queryFn: () => getLeagueFreeAgentsNBA(leagueId, '', 500, 0, '', period),
    enabled: active,
  })
  const { data: roster, isFetching: loadingRoster } = useQuery({
    queryKey: keys.leagueRostersNBAView(leagueId, period),
    queryFn: () => getLeagueRostersNBA(leagueId, period),
    enabled: active,
  })
  const { data: settings } = useQuery({
    queryKey: keys.leagueSettings(leagueId),
    queryFn: () => getLeagueSettings(leagueId),
    enabled: active,
  })
  // Every roster slot (BN/IR included — a real rostered player, not just a
  // starter) × num_teams.
  const rosterPoolSize = settings
    ? settings.num_teams * Object.values(settings.slots).reduce((sum, n) => sum + n, 0)
    : 0

  const teamName = (id: number) => teams.find((t) => t.id === id)?.name ?? `Team ${id}`
  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }
  const matchesPosition = (position: string) => positions.size === 0 || positions.has(position)

  const loading = loadingFA || loadingRoster
  // Free agents and rostered players are mutually exclusive sets by
  // definition (free-agents-nba is nba_players minus league_rosters_nba),
  // so concatenating them is never a dedup concern.
  const allRows: PlayerRow[] = useMemo(() => [...(roster ?? []), ...(freeAgents ?? [])], [roster, freeAgents])
  const baseRows: PlayerRow[] = useMemo(() =>
    view === 'free-agents' ? (freeAgents ?? [])
    : view === 'rostered' ? (roster ?? [])
    : allRows,
    [view, freeAgents, roster, allRows]
  )
  const showRosteredBy = view === 'rostered' || view === 'all'
  // Baseline is always the league-wide rostered-player pool (see
  // rosteredPlayerPool), independent of `view` — switching to the Free
  // Agents tab shouldn't change what "average" means.
  const catStats = useMemo(() => computeCategoryStats(rosteredPlayerPool(allRows, rosterPoolSize)), [allRows, rosterPoolSize])
  const ranks = useMemo(() => computeRanks(baseRows, catStats), [baseRows, catStats])

  const search_ = search.trim().toLowerCase()
  const filteredRows = baseRows.filter((p) => {
    if (!matchesPosition(p.position)) return false
    if (!search_) return true
    const rosteredByLabel = showRosteredBy ? (p.team_id != null ? teamName(p.team_id) : 'Free Agent') : ''
    return rowSearchText(p, rosteredByLabel).includes(search_)
  })

  const rows = useMemo(() => {
    const dirMul = sortDir === 'asc' ? 1 : -1
    return [...filteredRows].sort((a, b) => {
      let cmp: number
      switch (sortCol) {
        case 'rank': cmp = (ranks.get(a.espn_id) ?? Infinity) - (ranks.get(b.espn_id) ?? Infinity); break
        case 'player': cmp = a.name.localeCompare(b.name); break
        case 'team': cmp = (a.team || '').localeCompare(b.team || ''); break
        case 'pos': cmp = (a.position || '').localeCompare(b.position || ''); break
        case 'rostered_by': {
          const an = a.team_id != null ? teamName(a.team_id) : 'Free Agent'
          const bn = b.team_id != null ? teamName(b.team_id) : 'Free Agent'
          cmp = an.localeCompare(bn)
          break
        }
        default: cmp = statValue(a, sortCol as NBACategory) - statValue(b, sortCol as NBACategory)
      }
      return cmp * dirMul
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filteredRows, sortCol, sortDir, ranks])

  if (!active) return null

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <SelectControl value={view} onChange={(e) => setView(e.target.value as ViewOption)}>
          {VIEW_OPTIONS.map((s) => <option key={s.value} value={s.value}>{s.label}</option>)}
        </SelectControl>
        <SelectControl value={period} onChange={(e) => setPeriod(e.target.value as NBAStatPeriod)}>
          {NBA_STAT_PERIODS.map((p) => <option key={p} value={p}>{NBA_STAT_PERIOD_LABELS[p]}</option>)}
        </SelectControl>
        <div className="flex flex-none gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>{p}</FilterChip>
          ))}
        </div>
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search players…"
          className="w-48"
        />
        {!loading && <span className="text-xs text-muted-foreground ml-1">{rows.length} players</span>}
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">{search_ ? 'No players match this search.' : view === 'free-agents' ? 'No free agents match this filter.' : view === 'rostered' ? 'No players rostered yet.' : 'No players match this filter.'}</p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort}>Rank</SortableHead>
                <SortableHead col="player" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>Team</SortableHead>
                <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort}>Pos</SortableHead>
                {CATEGORIES.map((cat) => (
                  <SortableHead key={cat} col={cat} current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    {NBA_CATEGORY_LABELS[cat]}
                  </SortableHead>
                ))}
                {showRosteredBy && (
                  <SortableHead col="rostered_by" current={sortCol} dir={sortDir} onSort={handleSort}>Rostered By</SortableHead>
                )}
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <ClickableRow key={p.espn_id}>
                  <TableCell className="font-mono tabular-nums text-muted-foreground">{ranks.get(p.espn_id) ?? '—'}</TableCell>
                  <PlayerCell
                    name={p.name}
                    imageUrl={p.headshot_url}
                    onClick={() => { setDetailPeriod(period); setViewingPlayer(p.espn_id) }}
                  />
                  <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                  <TableCell className="text-muted-foreground">{p.position}</TableCell>
                  {CATEGORIES.map((cat) => {
                    const v = p.stats?.find((s) => s.category === cat)?.value
                    const isPct = cat === 'fg_pct' || cat === 'ft_pct'
                    const isAvg = period === 'season_avg' || period === 'last_season_avg'
                    return (
                      <ZScoreCell
                        key={cat}
                        value={v != null ? v.toFixed(isPct ? 3 : isAvg ? 1 : 0) : '—'}
                        zScore={v != null ? fantasyZScore(p, cat, catStats) : 0}
                      />
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

      <NBAPlayerDetailPanel
        espnId={viewingPlayer}
        leagueId={leagueId}
        period={detailPeriod}
        onPeriodChange={setDetailPeriod}
        onClose={() => setViewingPlayer(null)}
        onSelectPlayer={setViewingPlayer}
      />
    </>
  )
}
