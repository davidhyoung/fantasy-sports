import { useCallback, useMemo } from 'react'
import { ProjPlayerListItem } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, ClickableRow, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from './ConfidenceBadge'
import UniquenessBadge from './UniquenessBadge'
import DeltaBadge from '../../divergences/components/DeltaBadge'

// Ascending-first: names read A→Z; rank 1 and consensus rank 1 are the *best*,
// not the least. Everything else (points, grade, confidence, |delta|) is a
// magnitude where bigger is more interesting, so it defaults descending.
const ASC_COLS = ['name', 'pos', 'rank', 'consensus']

interface ProjectionTableProps {
  players: ProjPlayerListItem[]
  scoringFormat: 'ppr' | 'half' | 'standard'
  /**
   * Consensus context keyed by gsis_id. Partial by design — only players
   * present in the imported consensus data appear here.
   */
  divergences?: Map<string, { consensusRank: number; delta: number }>
}

export default function ProjectionTable({
  players,
  scoringFormat,
  divergences,
}: ProjectionTableProps) {
  const { sortCol, sortDir, handleSort } = useTableSort('rank', 'asc', ASC_COLS)

  const projPts = useCallback((p: ProjPlayerListItem) => {
    switch (scoringFormat) {
      case 'ppr':      return p.proj_fpts_ppr
      case 'half':     return p.proj_fpts_half
      case 'standard': return p.proj_fpts
    }
  }, [scoringFormat])

  const sorted = useMemo(() => {
    return [...players].sort((a, b) => {
      let aVal: string | number
      let bVal: string | number

      switch (sortCol) {
        case 'rank':       aVal = a.overall_rank; bVal = b.overall_rank; break
        case 'name':       aVal = a.name; bVal = b.name; break
        case 'pos':        aVal = a.position_group; bVal = b.position_group; break
        case 'age':        aVal = a.age || 0; bVal = b.age || 0; break
        case 'grade':      aVal = a.player_grade ?? -1; bVal = b.player_grade ?? -1; break
        case 'pts':        aVal = projPts(a); bVal = projPts(b); break
        case 'ppg':        aVal = a.proj_fpts_ppr_pg; bVal = b.proj_fpts_ppr_pg; break
        case 'confidence': aVal = a.confidence; bVal = b.confidence; break
        // Players with no consensus data sort last in either direction rather
        // than clustering at one end as if they were rank 0.
        case 'consensus':
          aVal = divergences?.get(a.gsis_id)?.consensusRank ?? Number.MAX_SAFE_INTEGER
          bVal = divergences?.get(b.gsis_id)?.consensusRank ?? Number.MAX_SAFE_INTEGER
          break
        case 'delta':
          aVal = Math.abs(divergences?.get(a.gsis_id)?.delta ?? -1)
          bVal = Math.abs(divergences?.get(b.gsis_id)?.delta ?? -1)
          break
        default:           aVal = a.overall_rank; bVal = b.overall_rank; break
      }

      if (typeof aVal === 'string') {
        const cmp = aVal.localeCompare(bVal as string)
        return sortDir === 'asc' ? cmp : -cmp
      }
      return sortDir === 'asc'
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number)
    })
  }, [players, sortCol, sortDir, projPts, divergences])

  if (players.length === 0) {
    return <p className="text-muted-foreground text-sm mt-6">No projections found.</p>
  }

  return (
    <div className="overflow-x-auto rounded-lg bg-card">
      <Table>
        <TableHeader>
          <HeaderRow>
            <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort} className="w-10 text-center">
              <HeaderTip description="Overall projection rank">#</HeaderTip>
            </SortableHead>
            <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
            <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="Position group">Pos</HeaderTip>
            </SortableHead>
            <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Age</SortableHead>
            <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Real-life player grade (0–100 percentile), independent of fantasy scoring">Grade</HeaderTip>
            </SortableHead>
            <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Projected fantasy points for the season, in the selected scoring format">Proj Pts</HeaderTip>
            </SortableHead>
            <SortableHead col="ppg" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Projected points per game under full-PPR scoring, for cross-format comparison">Pts/G</HeaderTip>
            </SortableHead>
            <SortableHead col="confidence" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="How much the projection leans on close historical comps vs. league-average priors">Confidence</HeaderTip>
            </SortableHead>
            <SortableHead col="consensus" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right whitespace-nowrap">
              <HeaderTip description="Median rank across external expert/ADP sources, within position group">Cons.</HeaderTip>
            </SortableHead>
            <SortableHead col="delta" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Our rank minus the consensus rank. Positive means we rank him lower than the market, negative means higher">Δ</HeaderTip>
            </SortableHead>
            <TableHead>
              <HeaderTip description="How statistically unusual this player's profile is — fewer close comps means a less certain projection">Profile</HeaderTip>
            </TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {sorted.map((p) => (
            <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}`}>
              <TableCell className="text-center text-muted-foreground font-mono tabular-nums">
                {p.overall_rank}
              </TableCell>
              <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
              <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
              <TableCell className="text-center text-muted-foreground font-mono tabular-nums">{p.age || '—'}</TableCell>
              <TableCell className="text-right tabular-nums font-mono">
                {p.player_grade != null ? (
                  <span className={gradeColorClass(p.player_grade)}>{p.player_grade.toFixed(0)}</span>
                ) : (
                  <span className="text-muted-foreground/40">—</span>
                )}
              </TableCell>
              <TableCell className="text-right tabular-nums font-mono">
                {projPts(p).toFixed(1)}
              </TableCell>
              <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                {p.proj_fpts_ppr_pg.toFixed(1)}
              </TableCell>
              <TableCell className="text-center">
                <ConfidenceBadge value={p.confidence} />
              </TableCell>
              <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                {divergences?.get(p.gsis_id)
                  ? divergences.get(p.gsis_id)!.consensusRank.toFixed(1)
                  : <span className="text-muted-foreground/40">—</span>}
              </TableCell>
              <TableCell className="text-right">
                {divergences?.get(p.gsis_id)
                  ? <DeltaBadge delta={divergences.get(p.gsis_id)!.delta} />
                  : <span className="text-muted-foreground/40">—</span>}
              </TableCell>
              <TableCell>
                <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} />
              </TableCell>
            </ClickableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
