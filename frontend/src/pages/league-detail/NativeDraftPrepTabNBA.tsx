import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort, ZScoreCell, type SortDir } from '@/components/ui/table-helpers'
import { FilterChip, SelectControl } from '@/components/ui/filter-chip'
import { Input } from '@/components/ui/input'
import { INTEREST_LEVELS, interestIconClass, interestRowClass } from '@/pages/draft-prep/lib/interest'
import {
  getLeagueRostersNBA, getLeagueFreeAgentsNBA, getLeagueSettings, getDraftPrepNBA, setDraftPrepPlayerNBA,
  NBA_CATEGORY_LABELS, NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS,
  type NBACategory, type NBAStatPeriod, type InterestLevel, type DraftPrepEntryNBA, type Team,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { NBAPlayerDetailPanel } from './components/NBAPlayerDetailPanel'
import { NBAConsensusRankings } from './components/NBAConsensusRankings'
import { NBAAuctionValues } from './components/NBAAuctionValues'
import { NBAKeepers } from './components/NBAKeepers'
import {
  NBA_RANK_CATEGORIES as CATEGORIES, nbaStatValue as statValue, nbaRosteredPlayerPool as rosteredPlayerPool,
  nbaComputeCategoryStats as computeCategoryStats, nbaFantasyZScore as fantasyZScore, nbaComputeRanks as computeRanks,
  type NBAPlayerRow as PlayerRow,
} from './lib/nbaRanking'
import { loadPersistedFilters, savePersistedFilters } from '@/lib/persistedFilters'

const POSITIONS = ['G', 'F', 'C']
type StatMode = 'stats' | 'zscores'
const STAT_MODES: { value: StatMode; label: string }[] = [
  { value: 'stats', label: 'Stats' },
  { value: 'zscores', label: 'Z-Scores' },
]

type BoardMode = 'board' | 'consensus' | 'auction' | 'keepers'
const BOARD_MODES: { value: BoardMode; label: string }[] = [
  { value: 'board', label: 'My Board' },
  { value: 'consensus', label: 'Consensus' },
  { value: 'auction', label: 'Auction $' },
  { value: 'keepers', label: 'Keepers' },
]

interface Props {
  leagueId: number
  active: boolean
  season: number
  teams: Team[]
  myTeam: Team | undefined
}

function formatZScore(z: number) {
  return (z >= 0 ? '+' : '') + z.toFixed(2)
}

const EMPTY_PREP: DraftPrepEntryNBA = { espn_id: '', interest: null, note: '' }

function rowSearchText(row: PlayerRow, note: string) {
  const statText = CATEGORIES.map((cat) => {
    const v = row.stats?.find((s) => s.category === cat)?.value
    return v == null ? '' : (cat === 'fg_pct' || cat === 'ft_pct' ? v.toFixed(3) : String(v))
  }).join(' ')
  return [row.name, row.team, row.position, note, statText].join(' ').toLowerCase()
}

/**
 * Personal NBA draft board — who's still undrafted (nba_players minus
 * league_rosters_nba, same universe the Players tab's Free Agents view
 * shows), ranked by the same z-score system, with a target/avoid flag and a
 * note persisted per (user, league, season). Deliberately smaller than the
 * NFL draft-prep page: no drag-reorder, team/plan panel, or planned-cost
 * column — there's still no roster-budget mechanism to plan against (see
 * .claude/plans). The Auction $ view (NBAAuctionValues) adds a read-only
 * valuation board instead: our own league-category-weighted dollar price
 * plus a derived consensus price, see docs/stats/nba-auction-values.md.
 * Sort/search/filter/color-coding/period-picker/detail-panel all reuse
 * exactly what the Players tab already built (see lib/nbaRanking).
 */
interface SavedFilters {
  boardMode: BoardMode
  period: NBAStatPeriod
  statMode: StatMode
  positions: string[]
  search: string
  sortCol: string
  sortDir: SortDir
}

const filtersStorageKey = (leagueId: number) => `fs.draft-prep-nba.${leagueId}.filters`

export function NativeDraftPrepTabNBA({ leagueId, active, season, teams, myTeam }: Props) {
  const qc = useQueryClient()
  const saved = useMemo(() => loadPersistedFilters<SavedFilters>(filtersStorageKey(leagueId)), [leagueId])
  const [boardMode, setBoardMode] = useState<BoardMode>(saved.boardMode ?? 'board')
  const [period, setPeriod] = useState<NBAStatPeriod>(saved.period ?? 'season')
  const [statMode, setStatMode] = useState<StatMode>(saved.statMode ?? 'stats')
  const [positions, setPositions] = useState<Set<string>>(new Set(saved.positions ?? []))
  const [search, setSearch] = useState(saved.search ?? '')
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)
  const [detailPeriod, setDetailPeriod] = useState<NBAStatPeriod>('season')
  const [noteDrafts, setNoteDrafts] = useState<Record<string, string>>({})
  const { sortCol, sortDir, handleSort } = useTableSort(
    saved.sortCol ?? 'rank', saved.sortDir ?? 'asc', ['rank', 'player', 'team', 'pos']
  )

  useEffect(() => {
    savePersistedFilters<SavedFilters>(filtersStorageKey(leagueId), { boardMode, period, statMode, positions: [...positions], search, sortCol, sortDir })
  }, [leagueId, boardMode, period, statMode, positions, search, sortCol, sortDir])

  const { data: freeAgents, isFetching: loadingFA } = useQuery({
    queryKey: keys.freeAgentsNBAView(leagueId, '', '', period),
    queryFn: () => getLeagueFreeAgentsNBA(leagueId, '', 500, 0, '', period),
    enabled: active,
  })
  // Rostered players are fetched only for the ranking baseline (see
  // lib/nbaRanking) — they're not draftable, so they never appear as rows.
  const { data: roster } = useQuery({
    queryKey: keys.leagueRostersNBAView(leagueId, period),
    queryFn: () => getLeagueRostersNBA(leagueId, period),
    enabled: active,
  })
  const { data: settings } = useQuery({
    queryKey: keys.leagueSettings(leagueId),
    queryFn: () => getLeagueSettings(leagueId),
    enabled: active,
  })
  const { data: prep } = useQuery({
    queryKey: keys.draftPrepNBA(leagueId, season),
    queryFn: () => getDraftPrepNBA(leagueId, season),
    enabled: active,
  })

  const rosterPoolSize = settings
    ? settings.num_teams * Object.values(settings.slots).reduce((sum, n) => sum + n, 0)
    : 0

  const prepByPlayer = useMemo(() => {
    const map = new Map<string, DraftPrepEntryNBA>()
    for (const e of prep?.players ?? []) map.set(e.espn_id, e)
    return map
  }, [prep])
  const prepFor = (espnId: string) => prepByPlayer.get(espnId) ?? { ...EMPTY_PREP, espn_id: espnId }

  const setPlayerPrep = useMutation({
    mutationFn: (v: { espnId: string; interest: InterestLevel | null; note: string }) =>
      setDraftPrepPlayerNBA(leagueId, season, v.espnId, { interest: v.interest, note: v.note }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.draftPrepNBA(leagueId, season) }),
  })
  const toggleInterest = (espnId: string, level: InterestLevel) => {
    const cur = prepFor(espnId)
    setPlayerPrep.mutate({ espnId, interest: cur.interest === level ? null : level, note: cur.note })
  }
  const commitNote = (espnId: string) => {
    const draft = noteDrafts[espnId]
    if (draft == null) return
    const cur = prepFor(espnId)
    if (draft !== cur.note) setPlayerPrep.mutate({ espnId, interest: cur.interest, note: draft })
    setNoteDrafts((prev) => { const next = { ...prev }; delete next[espnId]; return next })
  }

  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }
  const matchesPosition = (position: string) => positions.size === 0 || positions.has(position)

  const allRows: PlayerRow[] = useMemo(() => [...(roster ?? []), ...(freeAgents ?? [])], [roster, freeAgents])
  const baseRows: PlayerRow[] = freeAgents ?? []
  const catStats = useMemo(() => computeCategoryStats(rosteredPlayerPool(allRows, rosterPoolSize)), [allRows, rosterPoolSize])
  const ranks = useMemo(() => computeRanks(baseRows, catStats), [baseRows, catStats])

  const search_ = search.trim().toLowerCase()
  const filteredRows = baseRows.filter((p) => {
    if (!matchesPosition(p.position)) return false
    if (!search_) return true
    return rowSearchText(p, prepFor(p.espn_id).note).includes(search_)
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
        default:
          // In z-score mode, sort by the z-score actually shown — for
          // turnovers that's the fantasy-direction-corrected (negated)
          // value, so "higher" always means "better" consistently across
          // every column, including the one where a lower raw number wins.
          cmp = statMode === 'zscores'
            ? fantasyZScore(a, sortCol as NBACategory, catStats) - fantasyZScore(b, sortCol as NBACategory, catStats)
            : statValue(a, sortCol as NBACategory) - statValue(b, sortCol as NBACategory)
      }
      return cmp * dirMul
      // eslint-disable-next-line react-hooks/exhaustive-deps
    })
  }, [filteredRows, sortCol, sortDir, ranks, statMode, catStats])

  if (!active) return null
  const loading = loadingFA

  return (
    <>
      {/* Board and Consensus are genuinely different data sources (a
          personal, league-scoped draft board vs. a global projection-vs-
          market comparison) — not a filter on one dataset — so this sits
          above the filter row, same convention as NFL draft-prep's Board/
          Tiers toggle. */}
      <div className="flex w-fit rounded-lg bg-muted overflow-hidden mb-4">
        {BOARD_MODES.map((m) => (
          <button
            key={m.value}
            onClick={() => setBoardMode(m.value)}
            className={`px-4 py-2 font-display text-sm font-semibold ${
              boardMode === m.value
                ? 'bg-foreground text-background'
                : 'bg-card text-muted-foreground hover:bg-muted'
            }`}
          >
            {m.label}
          </button>
        ))}
      </div>

      {boardMode === 'consensus' ? (
        <NBAConsensusRankings
          season={season}
          onSelectPlayer={(espnId) => { setDetailPeriod(period); setViewingPlayer(espnId) }}
        />
      ) : boardMode === 'auction' ? (
        <NBAAuctionValues
          leagueId={leagueId}
          season={season}
          onSelectPlayer={(espnId) => { setDetailPeriod(period); setViewingPlayer(espnId) }}
        />
      ) : boardMode === 'keepers' ? (
        <NBAKeepers
          leagueId={leagueId}
          season={season}
          teams={teams}
          myTeam={myTeam}
          onSelectPlayer={(espnId) => { setDetailPeriod(period); setViewingPlayer(espnId) }}
        />
      ) : (
      <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <SelectControl value={period} onChange={(e) => setPeriod(e.target.value as NBAStatPeriod)}>
          {NBA_STAT_PERIODS.map((p) => <option key={p} value={p}>{NBA_STAT_PERIOD_LABELS[p]}</option>)}
        </SelectControl>
        <SelectControl value={statMode} onChange={(e) => setStatMode(e.target.value as StatMode)}>
          {STAT_MODES.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
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
        {!loading && <span className="text-xs text-muted-foreground ml-1">{rows.length} undrafted</span>}
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">{search_ ? 'No players match this search.' : 'No undrafted players match this filter.'}</p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <TableHead />
                <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort}>Rank</SortableHead>
                <SortableHead col="player" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>Team</SortableHead>
                <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort}>Pos</SortableHead>
                {CATEGORIES.map((cat) => (
                  <SortableHead key={cat} col={cat} current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    {NBA_CATEGORY_LABELS[cat]}
                  </SortableHead>
                ))}
                <TableHead>Note</TableHead>
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => {
                const mine = prepFor(p.espn_id)
                const noteValue = noteDrafts[p.espn_id] ?? mine.note
                return (
                  <ClickableRow key={p.espn_id} className={interestRowClass(mine.interest)}>
                    <TableCell>
                      <div className="flex items-center justify-center gap-1">
                        {INTEREST_LEVELS.map(({ level, label }) => {
                          const on = mine.interest === level
                          return (
                            <button
                              key={level}
                              onClick={() => toggleInterest(p.espn_id, level)}
                              title={label}
                              aria-label={`${label} — ${p.name}`}
                              aria-pressed={on}
                              className="flex h-5 w-5 items-center justify-center rounded font-mono text-sm hover:bg-muted"
                            >
                              <span className={interestIconClass(level, on)}>{level > 0 ? '△' : '▽'}</span>
                            </button>
                          )
                        })}
                      </div>
                    </TableCell>
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
                      const z = v != null ? fantasyZScore(p, cat, catStats) : 0
                      const isPct = cat === 'fg_pct' || cat === 'ft_pct'
                      const isAvg = period === 'season_avg' || period === 'last_season_avg'
                      const value = v == null ? '—'
                        : statMode === 'zscores' ? formatZScore(z)
                        : v.toFixed(isPct ? 3 : isAvg ? 1 : 0)
                      return <ZScoreCell key={cat} value={value} zScore={z} />
                    })}
                    <TableCell>
                      <Input
                        value={noteValue}
                        onChange={(e) => setNoteDrafts((prev) => ({ ...prev, [p.espn_id]: e.target.value }))}
                        onBlur={() => commitNote(p.espn_id)}
                        onKeyDown={(e) => { if (e.key === 'Enter') e.currentTarget.blur() }}
                        placeholder="Note…"
                        className="h-7 w-32 text-xs"
                      />
                    </TableCell>
                  </ClickableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
      </>
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
