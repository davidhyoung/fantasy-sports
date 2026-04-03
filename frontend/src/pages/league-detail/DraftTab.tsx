import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getDraftValues, DraftPlayer, DraftReplacementLevel } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from './components/TrendSparkline'
import { useState } from 'react'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K']
const STRING_COLS = ['name', 'pos']

function gradeColorClass(grade: number): string {
  if (grade >= 90) return 'text-emerald-600 dark:text-emerald-400 font-semibold'
  if (grade >= 70) return 'text-purple-600 dark:text-purple-400'
  if (grade >= 50) return ''
  return 'text-muted-foreground'
}

/** Shows when grade rank and fantasy rank diverge significantly. */
function DeltaBadge({ gradeRank, fantasyRank }: { gradeRank: number; fantasyRank: number }) {
  const diff = fantasyRank - gradeRank // positive = grade is better than fantasy rank
  if (Math.abs(diff) < 10) return null
  if (diff > 0) {
    return <span className="ml-1 text-[10px] px-1 py-0.5 rounded bg-emerald-100 text-emerald-700 dark:bg-emerald-950/50 dark:text-emerald-400">UV</span>
  }
  return <span className="ml-1 text-[10px] px-1 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-950/50 dark:text-amber-400">OV</span>
}

interface DraftTabProps {
  leagueId: number
  active: boolean
  season: string
}

export function DraftTab({ leagueId, active, season }: DraftTabProps) {
  const [position, setPosition] = useState('')
  const { sortCol, sortDir, handleSort } = useTableSort('rank', 'asc', STRING_COLS)

  // Draft prep is always for the season AFTER the completed league year.
  // e.g. a 2025 Yahoo league → draft for the 2026 NFL season.
  const seasonNum = (parseInt(season, 10) || 2025) + 1

  const { data, isLoading, isError } = useQuery({
    queryKey: keys.draftValues(leagueId, seasonNum, 'league', 200),
    queryFn: () => getDraftValues(leagueId, { season: seasonNum, budget: 200 }),
    enabled: active,
    staleTime: 5 * 60 * 1000,
  })

  // Compute grade ranks across ALL players (not filtered) for delta indicator
  const gradeRankMap = useMemo(() => {
    const map = new Map<string, number>()
    if (!data) return map
    const withGrade = data.players
      .filter(p => p.player_grade != null)
      .sort((a, b) => (b.player_grade ?? 0) - (a.player_grade ?? 0))
    withGrade.forEach((p, i) => map.set(p.gsis_id, i + 1))
    return map
  }, [data])

  const filtered = data?.players.filter(
    (p) => !position || p.position_group === position || p.position === position,
  ) ?? []

  const sorted = useMemo(() => {
    return [...filtered].sort((a, b) => {
      let aVal: string | number
      let bVal: string | number

      switch (sortCol) {
        case 'rank':      aVal = a.overall_rank; bVal = b.overall_rank; break
        case 'name':      aVal = a.name; bVal = b.name; break
        case 'pos':       aVal = a.position_group; bVal = b.position_group; break
        case 'age':       aVal = a.age || 0; bVal = b.age || 0; break
        case 'pts':       aVal = a.proj_league_fpts; bVal = b.proj_league_fpts; break
        case 'ppr':       aVal = a.proj_fpts_ppr_pg; bVal = b.proj_fpts_ppr_pg; break
        case 'vor':       aVal = a.vor; bVal = b.vor; break
        case 'auction':   aVal = a.auction_value; bVal = b.auction_value; break
        case 'grade':      aVal = a.player_grade ?? -1; bVal = b.player_grade ?? -1; break
        case 'confidence': aVal = a.confidence; bVal = b.confidence; break
        default:          aVal = a.overall_rank; bVal = b.overall_rank; break
      }

      if (typeof aVal === 'string') {
        const cmp = aVal.localeCompare(bVal as string)
        return sortDir === 'asc' ? cmp : -cmp
      }
      return sortDir === 'asc'
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number)
    })
  }, [filtered, sortCol, sortDir])

  const valueTier = (v: number) => {
    if (v >= 40) return 'text-yellow-600 dark:text-yellow-400 font-semibold'
    if (v >= 20) return 'text-green-600 dark:text-green-400'
    if (v >= 10) return 'text-purple-600 dark:text-purple-400'
    return 'text-muted-foreground'
  }

  return (
    <div className="space-y-4">
      {/* Header row + replacement levels */}
      <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-foreground">{seasonNum} Draft Rankings</h2>
          <p className="text-sm text-muted-foreground">
            League-specific projections · comp-based · auction values
          </p>
        </div>
        {data && data.replacement_levels.length > 0 && (
          <div className="flex flex-wrap gap-3 text-xs text-muted-foreground bg-card rounded-lg px-3 py-2 lg:max-w-[50%]">
            <span className="font-medium text-foreground">Replacement levels</span>
            <span className="text-muted-foreground">({data.num_teams} teams, ${data.budget_per_team}/team)</span>
            {data.replacement_levels
              .filter((rl: DraftReplacementLevel) => !position || rl.position === position)
              .map((rl: DraftReplacementLevel) => (
                <span key={rl.position}>
                  <span className="font-medium text-foreground">{rl.position}</span>{' '}
                  {rl.points.toFixed(0)} pts (top {rl.threshold})
                </span>
              ))}
          </div>
        )}
      </div>

      {/* Position filter */}
      <div className="flex gap-1 flex-wrap">
        {POSITIONS.map(pos => (
          <button
            key={pos}
            onClick={() => setPosition(pos === 'All' ? '' : pos)}
            className={`px-3 py-1 rounded text-sm font-medium transition-colors border ${
              (pos === 'All' && position === '') || pos === position
                ? 'bg-primary/20 text-primary border-primary/50'
                : 'text-muted-foreground border-border hover:text-foreground hover:border-muted-foreground'
            }`}
          >
            {pos}
          </button>
        ))}
      </div>

      {/* Table */}
      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading draft values...</p>
      ) : isError ? (
        <p className="text-red-600 dark:text-red-400 text-sm">Failed to load draft values. Make sure your league is synced.</p>
      ) : data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {sorted.length} player{sorted.length !== 1 ? 's' : ''}
            {position ? ` (${position})` : ''} · {data.scoring_format === 'league' ? 'league scoring' : data.scoring_format.toUpperCase()}
          </p>

          <div className="rounded-lg bg-card">
            <Table>
              <TableHeader>
                <HeaderRow>
                  <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort} className="w-10 text-center">#</SortableHead>
                  <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                  <TableHead>Trend</TableHead>
                  <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Pos</SortableHead>
                  <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Age</SortableHead>
                  <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Grade</SortableHead>
                  <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Proj Pts</SortableHead>
                  <SortableHead col="ppr" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">PPR/G</SortableHead>
                  <SortableHead col="vor" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">VOR</SortableHead>
                  <SortableHead col="auction" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Auction $</SortableHead>
                  <SortableHead col="confidence" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Confidence</SortableHead>
                  <TableHead>Profile</TableHead>
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {sorted.map((p: DraftPlayer) => (
                  <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}`}>
                    <TableCell className="text-center text-muted-foreground tabular-nums">
                      {p.overall_rank}
                    </TableCell>
                    <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
                    <TableCell>
                      <TrendSparkline points={p.trajectory ?? []} />
                    </TableCell>
                    <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
                    <TableCell className="text-center text-muted-foreground tabular-nums">{p.age || '\u2014'}</TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {p.player_grade != null ? (
                        <>
                          <span className={gradeColorClass(p.player_grade)}>{p.player_grade.toFixed(0)}</span>
                          {gradeRankMap.has(p.gsis_id) && (
                            <DeltaBadge gradeRank={gradeRankMap.get(p.gsis_id)!} fantasyRank={p.overall_rank} />
                          )}
                        </>
                      ) : (
                        <span className="text-muted-foreground/40">—</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {p.proj_league_fpts.toFixed(1)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                      {p.proj_fpts_ppr_pg.toFixed(1)}
                    </TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {p.vor.toFixed(1)}
                    </TableCell>
                    <TableCell className={`text-right tabular-nums font-mono ${valueTier(p.auction_value)}`}>
                      ${p.auction_value}
                    </TableCell>
                    <TableCell className="text-center">
                      <ConfidenceBadge value={p.confidence} />
                    </TableCell>
                    <TableCell>
                      <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} />
                    </TableCell>
                  </ClickableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      ) : null}
    </div>
  )
}
