import { useMemo, useState, useCallback } from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import type { SortDir } from '@/components/ui/table-helpers'
import { zScoreIndicator, zScoreColor } from '@/lib/utils'
import type { RankedPlayer, Team } from '../../api/client'
import { useRankings, RANKING_PERIODS } from './hooks/useRankings'
import { TrendSparkline } from './components/TrendSparkline'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
}

/** Text colour for the overall rank number. */
function rankColor(pct: number): string {
  if (pct >= 0.90) return 'text-amber-600 dark:text-amber-400 font-semibold'
  if (pct >= 0.75) return 'text-green-600 dark:text-green-400'
  return 'text-muted-foreground'
}

/** Text colour for a position rank label. */
function posRankColor(posRank: number): string {
  if (posRank === 1) return 'text-amber-600 dark:text-amber-400'
  if (posRank <= 3) return 'text-green-600 dark:text-green-400'
  return 'text-muted-foreground'
}

/** Format a numeric score with an explicit sign (+8.3 / -2.1). */
function fmtScore(n: number): string {
  return (n >= 0 ? '+' : '') + n.toFixed(1)
}

// Group filters: "G" = PG or SG, "F" = SF or PF (NBA only).
const GROUP_FILTERS: Record<string, string[]> = { G: ['PG', 'SG'], F: ['SF', 'PF'] }
// NFL positions first, then NBA — only positions present in data will appear as filter buttons.
const POSITION_ORDER = ['QB', 'RB', 'WR', 'TE', 'K', 'DEF', 'PG', 'SG', 'G', 'SF', 'PF', 'F', 'C']

// Today's date as a YYYY-MM-DD string for the max attribute on date inputs.
const TODAY = new Date().toISOString().slice(0, 10)

const STRING_COLS = ['name', 'position', 'team']

function playerSortValue(p: RankedPlayer, key: string): number | string {
  switch (key) {
    case 'rank': return p.overall_rank
    case 'name': return p.name.toLowerCase()
    case 'position': return p.position
    case 'team': return p.team_abbr
    case 'pts': return p.total_points ?? 0
    case 'score': return p.overall_score
    case 'pos_rank': return p.position_rank
    default: {
      const cs = p.category_scores.find((c) => c.label === key)
      return cs ? cs.z_score : -Infinity
    }
  }
}

export function RankingsTab({ leagueId, active, teams }: Props) {
  const {
    rankings, isLoading,
    periodMode, setPeriodMode,
    customDate, setCustomDate,
    effectiveStatType,
  } = useRankings(leagueId, active)

  const [posFilter, setPosFilter] = useState<string>('All')
  const [sortKey, setSortKey] = useState<string>('rank')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  const handleSort = useCallback((col: string) => {
    setSortKey((prev) => {
      if (prev === col) {
        setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
        return prev
      }
      setSortDir(STRING_COLS.includes(col) || col === 'rank' ? 'asc' : 'desc')
      return col
    })
  }, [])

  // Fantasy team name lookup: yahoo_key → team name
  const yahooKeyToName = useMemo(
    () => new Map(teams.map((t) => [t.yahoo_key ?? '', t.name])),
    [teams],
  )

  const positions = useMemo(() => {
    if (!rankings) return []
    const seen = new Set<string>()
    for (const p of rankings.players) {
      for (const pos of p.position.split(',')) seen.add(pos.trim())
    }
    const relevant = POSITION_ORDER.filter((f) =>
      GROUP_FILTERS[f]
        ? GROUP_FILTERS[f].some((m) => seen.has(m))
        : seen.has(f)
    )
    return ['All', ...relevant]
  }, [rankings])

  const visiblePlayers = useMemo(() => {
    if (!rankings) return []
    let filtered = rankings.players
    if (posFilter !== 'All') {
      const matchSet = GROUP_FILTERS[posFilter] ?? [posFilter]
      filtered = filtered.filter((p) =>
        p.position.split(',').some((pos) => matchSet.includes(pos.trim()))
      )
    }
    const sorted = [...filtered].sort((a, b) => {
      const va = playerSortValue(a, sortKey)
      const vb = playerSortValue(b, sortKey)
      let cmp: number
      if (typeof va === 'string' && typeof vb === 'string') {
        cmp = va.localeCompare(vb)
      } else {
        cmp = (va as number) - (vb as number)
      }
      return sortDir === 'asc' ? cmp : -cmp
    })
    return sorted
  }, [rankings, posFilter, sortKey, sortDir])

  // ── Stat period controls ──────────────────────────────────────────────────
  const periodBar = (
    <div className="flex items-center gap-2 flex-wrap">
      {RANKING_PERIODS.map(({ label, value }) => (
        <Button
          key={value}
          variant={periodMode === value ? 'default' : 'outline'}
          size="sm"
          onClick={() => setPeriodMode(value)}
        >
          {label}
        </Button>
      ))}
      <Button
        variant={periodMode === 'custom' ? 'default' : 'outline'}
        size="sm"
        onClick={() => setPeriodMode('custom')}
      >
        Pick Date
      </Button>

      {periodMode === 'custom' && (
        <input
          type="date"
          aria-label="Select date for rankings"
          value={customDate}
          max={TODAY}
          onChange={(e) => setCustomDate(e.target.value)}
          className="ml-1 h-8 rounded-md border border-input bg-background text-foreground px-2 text-base focus:outline-none focus:ring-2 focus:ring-ring"
        />
      )}
    </div>
  )

  // ── Loading / empty states ──────────────────────────────────────────────
  if (periodMode === 'custom' && !effectiveStatType) {
    return (
      <div className="space-y-4">
        {periodBar}
        <p className="text-muted-foreground py-4">Pick a date above to load rankings for that day.</p>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-4">
        {periodBar}
        <div className="flex items-center gap-2 text-muted-foreground py-8">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading rankings…
        </div>
      </div>
    )
  }

  if (!rankings || rankings.players.length === 0) {
    return (
      <div className="space-y-4">
        {periodBar}
        <p className="text-muted-foreground py-8">No rankings available for this period.</p>
      </div>
    )
  }

  const cats = rankings.categories
  const isPoints = rankings.scoring_mode === 'points'

  return (
    <div className="space-y-4">
      {periodBar}

      {/* ── Replacement levels legend (points leagues) ───────────────────── */}
      {isPoints && rankings.replacement_levels && rankings.replacement_levels.length > 0 && (
        <div className="rounded-lg bg-card p-3">
          <p className="text-xs text-muted-foreground mb-2 uppercase tracking-wide font-medium">
            Replacement Level (VORP baseline)
          </p>
          <div className="flex flex-wrap gap-x-4 gap-y-1">
            {rankings.replacement_levels.map((rl) => (
              <span key={rl.position} className="flex items-baseline gap-1 text-sm">
                <span className="font-medium">{rl.position}</span>
                <span className="text-xs font-mono text-muted-foreground">
                  {rl.points.toFixed(1)} pts
                </span>
                <span className="text-xs text-muted-foreground/60">
                  (top {rl.threshold})
                </span>
              </span>
            ))}
          </div>
        </div>
      )}

      {/* ── Category weights legend (category leagues only) ───────────────── */}
      {!isPoints && cats.length > 0 && (
        <div className="rounded-lg bg-card p-3">
          <p className="text-xs text-muted-foreground mb-2 uppercase tracking-wide font-medium">
            Category Weights
          </p>
          <div className="flex flex-wrap gap-x-4 gap-y-1">
            {cats.map((cat) => {
              const weightColor =
                cat.weight >= 1.2
                  ? 'text-amber-600 dark:text-amber-400'
                  : cat.weight <= 0.8
                  ? 'text-muted-foreground'
                  : 'text-foreground'
              const sortIcon = cat.sort_order === '1' ? '↑' : '↓'
              return (
                <span key={cat.label} className="flex items-baseline gap-1 text-sm">
                  <span className="text-muted-foreground text-xs">{sortIcon}</span>
                  <span className="font-medium">{cat.label}</span>
                  <span className={`text-xs font-mono ${weightColor}`}>
                    {cat.weight.toFixed(1)}×
                  </span>
                </span>
              )
            })}
          </div>
        </div>
      )}

      {/* ── Position filter ──────────────────────────────────────────────── */}
      <div className="flex items-center gap-2 flex-wrap">
        {positions.map((pos) => (
          <Button
            key={pos}
            variant={posFilter === pos ? 'secondary' : 'ghost'}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setPosFilter(pos)}
          >
            {pos}
          </Button>
        ))}
      </div>

      {/* ── Rankings table ────────────────────────────────────────────────── */}
      <div className="rounded-lg bg-card">
        <Table>
          <TableHeader>
            <HeaderRow>
              <SortableHead col="rank" current={sortKey} dir={sortDir} onSort={handleSort} className="w-10 text-center">#</SortableHead>
              <SortableHead col="name" current={sortKey} dir={sortDir} onSort={handleSort}>Player</SortableHead>
              <TableHead className="w-20">Trend</TableHead>
              <SortableHead col="position" current={sortKey} dir={sortDir} onSort={handleSort} className="w-14">Pos</SortableHead>
              <SortableHead col="team" current={sortKey} dir={sortDir} onSort={handleSort} className="w-16">Team</SortableHead>
              <TableHead className="w-36">Fantasy Team</TableHead>
              {isPoints && <SortableHead col="pts" current={sortKey} dir={sortDir} onSort={handleSort} className="w-20 text-right">Pts</SortableHead>}
              <SortableHead col="score" current={sortKey} dir={sortDir} onSort={handleSort} className="w-24 text-right">{isPoints ? 'VORP' : 'Score'}</SortableHead>
              {!isPoints && cats.map((cat) => (
                <SortableHead key={cat.label} col={cat.label} current={sortKey} dir={sortDir} onSort={handleSort} className="text-center text-xs w-16">{cat.label}</SortableHead>
              ))}
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {visiblePlayers.map((player, idx) => {
              const pct = visiblePlayers.length > 1
                ? 1 - idx / (visiblePlayers.length - 1)
                : 1

              const catScoreByLabel = new Map(
                player.category_scores.map((cs) => [cs.label, cs]),
              )
              const fantasyTeam =
                yahooKeyToName.get(player.owner_team_key) ?? player.owner_team_key

              const canLink = !!player.gsis_id
              return (
                <ClickableRow
                  key={player.player_key}
                  href={canLink ? `/players/${player.gsis_id}` : undefined}
                >
                  {/* Rank — colour by percentile */}
                  <TableCell className={`text-center tabular-nums text-sm ${rankColor(pct)}`}>
                    {player.overall_rank}
                  </TableCell>

                  <PlayerCell name={player.name} imageUrl={player.headshot_url} linked={canLink} />

                  <TableCell>
                    <TrendSparkline points={player.trajectory ?? []} />
                  </TableCell>

                  <TableCell className="text-xs text-muted-foreground">
                    {player.position}
                  </TableCell>

                  <TableCell className="text-xs text-muted-foreground">
                    {player.team_abbr}
                  </TableCell>

                  <TableCell className="text-xs text-muted-foreground truncate max-w-[9rem]">
                    {fantasyTeam}
                  </TableCell>

                  {/* Pts cell (points mode only) */}
                  {isPoints && (
                    <TableCell className="text-right tabular-nums text-sm text-muted-foreground">
                      {(player.total_points ?? 0).toFixed(1)}
                    </TableCell>
                  )}

                  {/* VORP / Score cell */}
                  <TableCell className="text-right tabular-nums">
                    {isPoints ? (
                      <div className="text-sm font-medium leading-tight">
                        {fmtScore(player.overall_score)}
                        <div className={`text-xs leading-tight ${posRankColor(player.position_rank)}`}>
                          {player.position} #{player.position_rank}
                        </div>
                      </div>
                    ) : (
                      <div className="text-sm font-medium leading-tight">
                        {fmtScore(player.overall_score)}
                        <span className="text-muted-foreground text-xs ml-1">
                          #{player.overall_rank}
                        </span>
                        <div className={`text-xs leading-tight ${posRankColor(player.position_rank)}`}>
                          {player.position} #{player.position_rank}
                        </div>
                      </div>
                    )}
                  </TableCell>

                  {/* Category z-score cells (categories mode) */}
                  {!isPoints && cats.map((cat) => {
                    const cs = catScoreByLabel.get(cat.label)
                    if (!cs) {
                      return (
                        <TableCell key={cat.label} className="text-center text-xs text-muted-foreground">
                          —
                        </TableCell>
                      )
                    }
                    return (
                      <TableCell
                        key={cat.label}
                        className="text-center text-xs tabular-nums"
                      >
                        {cs.z_score >= 0 ? '+' : ''}{cs.z_score.toFixed(1)}
                        <span className={`ml-0.5 text-[10px] ${zScoreColor(cs.z_score)}`} aria-label={cs.z_score > 0 ? 'Above average' : cs.z_score < 0 ? 'Below average' : 'Average'}>
                          {zScoreIndicator(cs.z_score) || '●'}
                        </span>
                      </TableCell>
                    )
                  })}
                </ClickableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>

      <p className="text-xs text-muted-foreground">
        {visiblePlayers.length} player{visiblePlayers.length !== 1 ? 's' : ''}
        {posFilter !== 'All' ? ` · ${posFilter}` : ''} · {rankings.stat_type}
      </p>
    </div>
  )
}
