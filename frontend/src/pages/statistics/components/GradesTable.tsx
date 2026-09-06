import { useState } from 'react'
import { GradePlayerItem } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, PlayerAvatar, ClickableRow, HeaderRow, SortableHead, useTableSort, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { MobileSortSheet, type MobileSortOption } from '@/components/ui/mobile-sort-sheet'
import { gradeColorClass, trendIndicator, phaseLabel, phaseColor } from '@/lib/grades'
import { PlayerDetailPanel } from '@/pages/player-detail/PlayerDetailPanel'

const SORT_OPTIONS: MobileSortOption[] = [
  { col: 'name', label: 'Player' },
  { col: 'age', label: 'Age' },
  { col: 'overall', label: 'Overall' },
  { col: 'production', label: 'Prod' },
  { col: 'efficiency', label: 'Eff' },
  { col: 'usage', label: 'Usage' },
  { col: 'durability', label: 'Dur' },
  { col: 'trend', label: 'Trend' },
]

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
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)

  if (players.length === 0) {
    return <p className="text-muted-foreground text-sm mt-6">No players match this filter.</p>
  }

  return (
    <>
      {/* Card list below md — face: Player, Overall, Phase; expansion: Prod, Eff,
          Usage, Dur, Age, Trend. */}
      <div className="space-y-2 md:hidden">
        <div className="flex justify-end">
          <MobileSortSheet options={SORT_OPTIONS} current={sortCol} dir={sortDir} onSort={handleSort} />
        </div>
        {sorted.map((p) => {
          const trend = trendIndicator(p.yoy_trend)
          const expanded: MobileStatField[] = [
            { label: 'Prod', value: <span className={gradeColorClass(p.production)}>{p.production.toFixed(0)}</span> },
            { label: 'Eff', value: <span className={gradeColorClass(p.efficiency)}>{p.efficiency.toFixed(0)}</span> },
            { label: 'Usage', value: <span className={gradeColorClass(p.usage)}>{p.usage.toFixed(0)}</span> },
            { label: 'Dur', value: <span className={gradeColorClass(p.durability)}>{p.durability.toFixed(0)}</span> },
            { label: 'Age', value: p.age || '—' },
            { label: 'Trend', value: trend.text ? <span className={trend.color}>{trend.text}</span> : '—' },
          ]
          return (
            <MobileStatCard
              key={p.gsis_id}
              onClick={() => setViewingPlayer(p.gsis_id)}
              leading={<PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />}
              title={p.name}
              subtitle={`${p.team ?? ''} · ${p.position_group}`}
              face={
                <div className="text-right">
                  <div className="font-mono text-xs tabular-nums">
                    <span className={gradeColorClass(p.overall)}>{p.overall.toFixed(0)}</span>
                  </div>
                  <div className={`mt-0.5 text-[11px] font-medium ${phaseColor(p.career_phase)}`}>
                    {phaseLabel(p.career_phase)}
                  </div>
                </div>
              }
              expanded={expanded}
            />
          )
        })}
      </div>

      <div className="hidden md:block overflow-x-auto rounded-xl bg-card">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead className="w-10 text-center">#</TableHead>
            <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
            <TableHead className="text-center"><HeaderTip description="Position group">Pos</HeaderTip></TableHead>
            <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Age</SortableHead>
            <SortableHead col="overall" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Overall real-life grade (0–100 percentile) — a blend of production, efficiency, usage, and durability">Overall</HeaderTip>
            </SortableHead>
            <SortableHead col="production" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Production — raw statistical output relative to position peers">Prod</HeaderTip>
            </SortableHead>
            <SortableHead col="efficiency" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Efficiency — output per opportunity (yards per touch, catch rate, etc.), not just volume">Eff</HeaderTip>
            </SortableHead>
            <SortableHead col="usage" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Share of the offense's opportunities — snaps, targets, or carries relative to teammates">Usage</HeaderTip>
            </SortableHead>
            <SortableHead col="durability" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Durability — games played and availability across recent seasons">Dur</HeaderTip>
            </SortableHead>
            <TableHead className="text-center">
              <HeaderTip description="Career stage inferred from age and grade trend (e.g. Ascending, Prime, Declining)">Phase</HeaderTip>
            </TableHead>
            <SortableHead col="trend" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Year-over-year change in overall grade">Trend</HeaderTip>
            </SortableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {sorted.map((p, i) => {
            const trend = trendIndicator(p.yoy_trend)
            return (
              <ClickableRow key={p.gsis_id}>
                <TableCell className="text-center text-muted-foreground font-mono tabular-nums">
                  {sortCol === 'overall' ? p.overall_rank : i + 1}
                </TableCell>
                <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} onClick={() => setViewingPlayer(p.gsis_id)} notes={p.notes} />
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

      <PlayerDetailPanel gsisId={viewingPlayer} onClose={() => setViewingPlayer(null)} />
    </>
  )
}
