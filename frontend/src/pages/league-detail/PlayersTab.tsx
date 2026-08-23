import { useCallback, useMemo } from 'react'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, PlayerAvatar, ClickableRow, ZScoreCell, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { MobileSortSheet, type MobileSortOption } from '@/components/ui/mobile-sort-sheet'
import { gradeColorClass, trendIndicator } from '@/lib/grades'
import { zScoreIndicator, zScoreColor } from '@/lib/utils'
import { describeStat } from '@/lib/statDescriptions'
import { usePlayers, STATUS_FILTERS } from './hooks/usePlayers'
import type { PlayerRow } from './hooks/usePlayers'

const POSITIONS_BY_SPORT: Record<string, string[]> = {
  nfl: ['QB', 'RB', 'WR', 'TE', 'K', 'DEF'],
  nba: ['PG', 'SG', 'SF', 'PF', 'C'],
}

interface Props {
  leagueId: number
  active: boolean
  sport: string
}

/** Format a raw stat value: percentages (< 1) get 3 decimals, large numbers no decimals, else 1. */
function fmtStat(v: number): string {
  if (Math.abs(v) < 1 && v !== 0) return v.toFixed(3)
  if (Math.abs(v) < 100) return v.toFixed(1)
  return Math.round(v).toLocaleString()
}

// Names read A→Z. Category columns are dynamic (NBA scoring categories), so
// their direction can't be hardcoded — a counting stat like turnovers is
// "lower is better" and should default ascending, unlike points or rebounds.
// The API's `sort_order` ("0" = lower is better) is the source of truth for that.
const ASC_COLS = ['name', 'team', 'position']

/** Player search and available-player browse with position filter, sortable columns, and stat columns.
 *
 * Browse mode sources data from the rankings endpoint (all rostered + top-100 FAs) so that
 * sorting always applies to the full relevant player universe with no pagination. */
export function PlayersTab({ leagueId, active, sport }: Props) {
  const {
    searchInput, setSearchInput,
    activeSearch,
    position, setPosition,
    statusFilter, setStatusFilter,
    isSearchMode,
    playerRows,
    loading, ready,
    rankings,
    handleSearch, clearSearch,
  } = usePlayers(leagueId, active)

  const isPoints  = rankings?.scoring_mode === 'points'
  const cats      = rankings?.categories ?? []

  const isAscCol = useCallback(
    (col: string) => ASC_COLS.includes(col) || cats.find((c) => c.label === col)?.sort_order === '0',
    [cats]
  )
  const { sortCol, sortDir, handleSort } = useTableSort('vorp', 'desc', isAscCol)

  const positions = POSITIONS_BY_SPORT[sport] ?? []

  const sortedRows = useMemo((): PlayerRow[] => {
    return [...playerRows].sort((a, b) => {
      let aVal: string | number
      let bVal: string | number

      switch (sortCol) {
        case 'name':
          aVal = a.name; bVal = b.name; break
        case 'team':
          aVal = a.teamAbbr; bVal = b.teamAbbr; break
        case 'position':
          aVal = a.position; bVal = b.position; break
        case 'grade':
          aVal = a.rp?.player_grade ?? -Infinity
          bVal = b.rp?.player_grade ?? -Infinity
          break
        case 'trend':
          aVal = a.rp?.yoy_trend ?? -Infinity
          bVal = b.rp?.yoy_trend ?? -Infinity
          break
        case 'vorp':
          aVal = a.rp?.overall_score ?? -Infinity
          bVal = b.rp?.overall_score ?? -Infinity
          break
        case 'pts':
          aVal = a.rp?.total_points ?? -Infinity
          bVal = b.rp?.total_points ?? -Infinity
          break
        default: {
          const getCat = (row: PlayerRow) =>
            row.rp?.category_scores?.find((c) => c.label === sortCol)?.value ?? -Infinity
          aVal = getCat(a)
          bVal = getCat(b)
        }
      }

      if (typeof aVal === 'string') {
        const cmp = aVal.localeCompare(bVal as string)
        return sortDir === 'asc' ? cmp : -cmp
      }
      return sortDir === 'asc'
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number)
    })
  }, [playerRows, sortCol, sortDir])

  return (
    <>
      {/* Search bar */}
      <form onSubmit={handleSearch} className="mb-4">
        <div className="flex items-center gap-2">
          <Input
            placeholder="Search by name…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="max-w-[280px] h-8 text-sm"
          />
          <Button type="submit" size="sm">Search</Button>
          {activeSearch && (
            <Button size="sm" variant="ghost" onClick={clearSearch}>Clear</Button>
          )}
        </div>
      </form>

      {/* Status + position filters — browse mode only */}
      {!isSearchMode && (
        <div className="flex items-center gap-3 mb-4 flex-wrap">
          <select
            aria-label="Filter by status"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as typeof statusFilter)}
            className="h-8 rounded-md border border-input bg-background text-foreground px-2 py-1 text-base cursor-pointer focus:outline-none focus:ring-2 focus:ring-ring"
          >
            {STATUS_FILTERS.map((f) => (
              <option key={f.value} value={f.value}>{f.label}</option>
            ))}
          </select>

          {positions.length > 0 && (
            <select
              aria-label="Filter by position"
              value={position}
              onChange={(e) => setPosition(e.target.value)}
              className="h-8 rounded-md border border-input bg-background text-foreground px-2 py-1 text-base cursor-pointer focus:outline-none focus:ring-2 focus:ring-ring"
            >
              <option value="">All positions</option>
              {positions.map((p) => <option key={p} value={p}>{p}</option>)}
            </select>
          )}

          {ready && (
            <span className="text-xs text-muted-foreground ml-1">
              {sortedRows.length} player{sortedRows.length !== 1 ? 's' : ''}
            </span>
          )}
        </div>
      )}

      {/* Results */}
      {loading || !ready ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : sortedRows.length === 0 ? (
        <p className="text-muted-foreground">
          {isSearchMode ? `No players found for "${activeSearch}".` : 'No players found.'}
        </p>
      ) : (
        <>
        {/* Card list below md — face: Player, Grade, VORP; expansion: Trend, Pts,
            per-category z-score cells, Avail. */}
        <div className="space-y-2 md:hidden">
          <div className="flex justify-end">
            <MobileSortSheet
              options={(() => {
                const opts: MobileSortOption[] = [
                  { col: 'name', label: 'Player' },
                  { col: 'team', label: 'Team' },
                  { col: 'position', label: 'Pos' },
                ]
                if (rankings) opts.push({ col: 'grade', label: 'Grade' }, { col: 'trend', label: 'Trend' })
                if (rankings) opts.push({ col: 'vorp', label: isPoints ? 'VORP' : 'Value' })
                if (isPoints) opts.push({ col: 'pts', label: 'Pts' })
                for (const cat of cats) opts.push({ col: cat.label, label: cat.label, description: describeStat(cat.label) })
                return opts
              })()}
              current={sortCol}
              dir={sortDir}
              onSort={handleSort}
            />
          </div>
          {sortedRows.map((row) => {
            const canLink = !!row.gsisId
            const isAvailable = row.ownerTeamKey === ''
            const trend = trendIndicator(row.rp?.yoy_trend)
            const expanded: MobileStatField[] = []
            if (rankings) {
              expanded.push({
                label: 'Trend',
                value: trend.text ? <span className={trend.color}>{trend.text}</span> : '—',
              })
            }
            if (isPoints) {
              expanded.push({
                label: 'Pts',
                value: row.rp?.total_points != null ? row.rp.total_points.toFixed(1) : '—',
              })
            }
            for (const cat of cats) {
              const cs = row.rp?.category_scores?.find((c) => c.label === cat.label)
              expanded.push({
                label: cat.label,
                value: cs ? (
                  <>
                    {fmtStat(cs.value)}
                    <span className={`ml-0.5 text-[10px] ${zScoreColor(cs.z_score)}`}>
                      {zScoreIndicator(cs.z_score) || '●'}
                    </span>
                  </>
                ) : '—',
              })
            }
            if (statusFilter === 'all' || isSearchMode) {
              expanded.push({ label: 'Avail', value: isAvailable ? <Badge variant="blue">FA</Badge> : '—' })
            }
            return (
              <MobileStatCard
                key={row.playerKey}
                href={canLink ? `/players/${row.gsisId}` : undefined}
                leading={<PlayerAvatar src={row.imageUrl} alt={row.name} size={28} />}
                title={row.name}
                subtitle={`${row.teamAbbr || '—'} · ${row.position}`}
                face={
                  rankings ? (
                    <div className="text-right">
                      <div className="font-mono text-xs tabular-nums">
                        {row.rp?.player_grade != null ? (
                          <span className={gradeColorClass(row.rp.player_grade)}>{row.rp.player_grade.toFixed(0)}</span>
                        ) : (
                          <span className="text-muted-foreground/40">—</span>
                        )}
                      </div>
                      <div className="mt-0.5 font-mono text-[11px] tabular-nums">
                        {row.rp ? (
                          <span className={row.rp.overall_score >= 0 ? 'text-positive-foreground' : 'text-destructive'}>
                            {row.rp.overall_score > 0 ? '+' : ''}{row.rp.overall_score.toFixed(1)}
                          </span>
                        ) : '—'}
                      </div>
                    </div>
                  ) : undefined
                }
                expanded={expanded}
              />
            )
          })}
        </div>

        <div className="hidden md:block rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>
                  Player
                </SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>
                  Team
                </SortableHead>
                <SortableHead col="position" current={sortCol} dir={sortDir} onSort={handleSort}>
                  <HeaderTip description="Position eligibility">Pos</HeaderTip>
                </SortableHead>
                {rankings && (
                  <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    <HeaderTip description="Real-life player grade (0–100 percentile), independent of fantasy scoring">Grade</HeaderTip>
                  </SortableHead>
                )}
                {rankings && (
                  <SortableHead col="trend" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    <HeaderTip description="Year-over-year change in the player's grade">Trend</HeaderTip>
                  </SortableHead>
                )}
                {rankings && (
                  <SortableHead col="vorp" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    <HeaderTip
                      description={
                        isPoints
                          ? 'Value Over Replacement Player — points above the last startable player at this position'
                          : 'Overall z-score value across your league’s scoring categories'
                      }
                    >
                      {isPoints ? 'VORP' : 'Value'}
                    </HeaderTip>
                  </SortableHead>
                )}
                {isPoints && (
                  <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    <HeaderTip description="Total fantasy points scored in this period">Pts</HeaderTip>
                  </SortableHead>
                )}
                {cats.map((cat) => (
                  <SortableHead
                    key={cat.label}
                    col={cat.label}
                    current={sortCol}
                    dir={sortDir}
                    onSort={handleSort}
                    className="text-right"
                  >
                    <HeaderTip description={describeStat(cat.label)}>{cat.label}</HeaderTip>
                  </SortableHead>
                ))}
                {(statusFilter === 'all' || isSearchMode) && (
                  <TableHead className="whitespace-nowrap">
                    <HeaderTip description="Player is currently a free agent in your league">Avail</HeaderTip>
                  </TableHead>
                )}
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {sortedRows.map((row) => {
                const canLink = !!row.gsisId
                const isAvailable = row.ownerTeamKey === ''
                return (
                  <ClickableRow
                    key={row.playerKey}
                    href={canLink ? `/players/${row.gsisId}` : undefined}
                  >
                    <PlayerCell name={row.name} imageUrl={row.imageUrl} linked={canLink} notes={row.rp?.notes} />
                    <TableCell className="text-muted-foreground">{row.teamAbbr || '—'}</TableCell>
                    <TableCell className="text-muted-foreground">{row.position}</TableCell>

                    {/* Grade */}
                    {rankings && (
                      <TableCell className="text-right tabular-nums font-mono">
                        {row.rp?.player_grade != null ? (
                          <span className={gradeColorClass(row.rp.player_grade)}>{row.rp.player_grade.toFixed(0)}</span>
                        ) : (
                          <span className="text-muted-foreground/40">—</span>
                        )}
                      </TableCell>
                    )}

                    {/* Trend */}
                    {rankings && (() => {
                      const t = trendIndicator(row.rp?.yoy_trend)
                      return (
                        <TableCell className="text-right tabular-nums font-mono text-xs">
                          {t.text ? (
                            <span className={t.color}>{t.text}</span>
                          ) : (
                            <span className="text-muted-foreground/40">—</span>
                          )}
                        </TableCell>
                      )
                    })()}

                    {/* VORP / Value */}
                    {rankings && (
                      <TableCell className="text-right font-mono tabular-nums">
                        {row.rp ? (
                          <span className="inline-flex items-center gap-1">
                            <span className={row.rp.overall_score >= 0 ? 'text-positive-foreground' : 'text-destructive'}>
                              {row.rp.overall_score > 0 ? '+' : ''}{row.rp.overall_score.toFixed(1)}
                            </span>
                            <span className="text-xs text-muted-foreground">#{row.rp.overall_rank}</span>
                          </span>
                        ) : '—'}
                      </TableCell>
                    )}

                    {/* Total points (NFL only) */}
                    {isPoints && (
                      <TableCell className="text-right font-mono tabular-nums text-muted-foreground text-sm">
                        {row.rp?.total_points != null ? row.rp.total_points.toFixed(1) : '—'}
                      </TableCell>
                    )}

                    {/* Per-category stat cells */}
                    {cats.map((cat) => {
                      const cs = row.rp?.category_scores?.find((c) => c.label === cat.label)
                      if (!cs) {
                        return <TableCell key={cat.label} className="text-right text-xs font-mono tabular-nums">—</TableCell>
                      }
                      return (
                        <ZScoreCell key={cat.label} value={fmtStat(cs.value)} zScore={cs.z_score} />
                      )
                    })}

                    {/* Availability */}
                    {(statusFilter === 'all' || isSearchMode) && (
                      <TableCell>
                        {isAvailable ? (
                          <Badge variant="blue">FA</Badge>
                        ) : null}
                      </TableCell>
                    )}
                  </ClickableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
        </>
      )}
    </>
  )
}
