import { useState } from 'react'
import { useRankings } from './hooks/useRankings'
import { GradePlayerItem } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort } from '@/components/ui/table-helpers'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K', 'Flex', 'Superflex']
const POSITION_FILTER: Record<string, string> = {
  'Flex': 'RB,WR,TE',
  'Superflex': 'QB,RB,WR,TE',
}
const CURRENT_SEASON = 2025

function gradeColorClass(grade: number): string {
  if (grade >= 90) return 'text-emerald-600 dark:text-emerald-400 font-semibold'
  if (grade >= 70) return 'text-purple-600 dark:text-purple-400'
  if (grade >= 50) return ''
  return 'text-muted-foreground'
}

function trendIndicator(trend: number | null): { text: string; color: string } {
  if (trend == null) return { text: '', color: '' }
  const pct = Math.round(trend * 100)
  if (trend > 0.05) return { text: `+${pct}`, color: 'text-emerald-600 dark:text-emerald-400' }
  if (trend < -0.05) return { text: `${pct}`, color: 'text-red-600 dark:text-red-400' }
  return { text: `${pct >= 0 ? '+' : ''}${pct}`, color: 'text-muted-foreground' }
}

function phaseLabel(phase: string): string {
  switch (phase) {
    case 'developing': return 'Dev'
    case 'entering-prime': return 'Enter'
    case 'prime': return 'Prime'
    case 'post-prime': return 'Post'
    case 'late-career': return 'Late'
    default: return phase
  }
}

function phaseColor(phase: string): string {
  switch (phase) {
    case 'developing': return 'text-sky-600 dark:text-sky-400'
    case 'entering-prime': return 'text-emerald-600 dark:text-emerald-400'
    case 'prime': return 'text-emerald-700 dark:text-emerald-300'
    case 'post-prime': return 'text-amber-600 dark:text-amber-400'
    case 'late-career': return 'text-red-600 dark:text-red-400'
    default: return 'text-muted-foreground'
  }
}

type SortKey = 'overall' | 'production' | 'efficiency' | 'usage' | 'durability' | 'name' | 'age' | 'trend'

function sortPlayers(players: GradePlayerItem[], col: SortKey, dir: 'asc' | 'desc'): GradePlayerItem[] {
  const sorted = [...players].sort((a, b) => {
    let cmp = 0
    switch (col) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'age': cmp = a.age - b.age; break
      case 'trend': cmp = (a.yoy_trend ?? -999) - (b.yoy_trend ?? -999); break
      default: cmp = (a[col] as number) - (b[col] as number)
    }
    return dir === 'desc' ? -cmp : cmp
  })
  return sorted
}

export default function Rankings() {
  const [positionLabel, setPositionLabel] = useState('All')
  const [season, setSeason] = useState(CURRENT_SEASON)

  const position = positionLabel === 'All' ? '' : (POSITION_FILTER[positionLabel] ?? positionLabel)
  const { sortCol, sortDir, handleSort } = useTableSort('overall', 'desc', ['name'])
  const { data, isLoading, isError } = useRankings({ season, position })

  const players = data ? sortPlayers(data.players, sortCol as SortKey, sortDir) : []

  return (
    <div className="max-w-6xl mx-auto space-y-4">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Player Rankings</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Real-life player grades — how good is this player at actual football?
          </p>
        </div>

        {/* Season selector */}
        <div className="flex items-center gap-2 self-start sm:self-center">
          <label className="text-sm text-muted-foreground">Season</label>
          <select
            value={season}
            onChange={e => setSeason(Number(e.target.value))}
            className="rounded-md border border-border bg-card px-2 py-1 text-sm text-foreground"
          >
            {Array.from({ length: 6 }, (_, i) => CURRENT_SEASON - i).map(y => (
              <option key={y} value={y}>{y}</option>
            ))}
          </select>
        </div>
      </div>

      {/* Position filter */}
      <div className="flex gap-1 flex-wrap">
        {POSITIONS.map(pos => (
          <button
            key={pos}
            onClick={() => setPositionLabel(pos)}
            className={`px-3 py-1 rounded text-sm font-medium transition-colors border ${
              pos === positionLabel
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
        <p className="text-muted-foreground text-sm">Loading rankings...</p>
      ) : isError ? (
        <p className="text-red-600 dark:text-red-400 text-sm">Failed to load rankings.</p>
      ) : data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {data.total} player{data.total !== 1 ? 's' : ''} graded for {data.season}
            {positionLabel !== 'All' ? ` · ${positionLabel}` : ''}
          </p>

          <div className="overflow-x-auto rounded-lg bg-card">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead className="w-10 text-center">#</TableHead>
                  <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                  <TableHead className="text-center">Pos</TableHead>
                  <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Age</SortableHead>
                  <SortableHead col="overall" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Overall</SortableHead>
                  <SortableHead col="production" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Prod</SortableHead>
                  <SortableHead col="efficiency" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Eff</SortableHead>
                  <SortableHead col="usage" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Usage</SortableHead>
                  <SortableHead col="durability" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Dur</SortableHead>
                  <TableHead className="text-center">Phase</TableHead>
                  <SortableHead col="trend" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Trend</SortableHead>
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {players.map((p, i) => {
                  const trend = trendIndicator(p.yoy_trend)
                  return (
                    <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}`}>
                      <TableCell className="text-center text-muted-foreground tabular-nums">
                        {sortCol === 'overall' ? p.overall_rank : i + 1}
                      </TableCell>
                      <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
                      <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
                      <TableCell className="text-right text-muted-foreground tabular-nums">{p.age || '—'}</TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        <span className={gradeColorClass(p.overall)}>{p.overall.toFixed(0)}</span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        <span className={gradeColorClass(p.production)}>{p.production.toFixed(0)}</span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        <span className={gradeColorClass(p.efficiency)}>{p.efficiency.toFixed(0)}</span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        <span className={gradeColorClass(p.usage)}>{p.usage.toFixed(0)}</span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        <span className={gradeColorClass(p.durability)}>{p.durability.toFixed(0)}</span>
                      </TableCell>
                      <TableCell className="text-center">
                        <span className={`text-xs font-medium ${phaseColor(p.career_phase)}`}>
                          {phaseLabel(p.career_phase)}
                        </span>
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono text-xs">
                        {trend.text ? (
                          <span className={trend.color}>{trend.text}</span>
                        ) : (
                          <span className="text-muted-foreground/40">—</span>
                        )}
                      </TableCell>
                    </ClickableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        </>
      ) : null}
    </div>
  )
}
