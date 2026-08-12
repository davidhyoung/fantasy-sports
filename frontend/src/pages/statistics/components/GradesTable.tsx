import { GradePlayerItem } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort } from '@/components/ui/table-helpers'
import { gradeColorClass, trendIndicator, phaseLabel, phaseColor } from '@/lib/grades'

type SortKey = 'overall' | 'production' | 'efficiency' | 'usage' | 'durability' | 'name' | 'age' | 'trend'

function sortPlayers(players: GradePlayerItem[], col: SortKey, dir: 'asc' | 'desc'): GradePlayerItem[] {
  return [...players].sort((a, b) => {
    let cmp = 0
    switch (col) {
      case 'name': cmp = a.name.localeCompare(b.name); break
      case 'age': cmp = a.age - b.age; break
      case 'trend': cmp = (a.yoy_trend ?? -999) - (b.yoy_trend ?? -999); break
      default: cmp = (a[col] as number) - (b[col] as number)
    }
    return dir === 'desc' ? -cmp : cmp
  })
}

/** Real-life player grades — how good is this player at actual football? */
export default function GradesTable({ players }: { players: GradePlayerItem[] }) {
  const { sortCol, sortDir, handleSort } = useTableSort('overall', 'desc', ['name'])
  const sorted = sortPlayers(players, sortCol as SortKey, sortDir)

  if (players.length === 0) {
    return <p className="text-muted-foreground text-sm mt-6">No players match this filter.</p>
  }

  return (
    <div className="overflow-x-auto rounded-xl bg-card">
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
          {sorted.map((p, i) => {
            const trend = trendIndicator(p.yoy_trend)
            return (
              <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}`}>
                <TableCell className="text-center text-muted-foreground font-mono tabular-nums">
                  {sortCol === 'overall' ? p.overall_rank : i + 1}
                </TableCell>
                <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
                <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
                <TableCell className="text-right text-muted-foreground font-mono tabular-nums">{p.age || '—'}</TableCell>
                {(['overall', 'production', 'efficiency', 'usage', 'durability'] as const).map(key => (
                  <TableCell key={key} className="text-right tabular-nums font-mono">
                    <span className={gradeColorClass(p[key])}>{p[key].toFixed(0)}</span>
                  </TableCell>
                ))}
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
  )
}
