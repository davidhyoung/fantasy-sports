import { ProjPlayerListItem } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from './ConfidenceBadge'
import UniquenessBadge from './UniquenessBadge'

interface ProjectionTableProps {
  players: ProjPlayerListItem[]
  scoringFormat: 'ppr' | 'half' | 'standard'
}

export default function ProjectionTable({ players, scoringFormat }: ProjectionTableProps) {
  const projPts = (p: ProjPlayerListItem) => {
    switch (scoringFormat) {
      case 'ppr':      return p.proj_fpts_ppr
      case 'half':     return p.proj_fpts_half
      case 'standard': return p.proj_fpts
    }
  }

  if (players.length === 0) {
    return <p className="text-muted-foreground text-sm mt-6">No projections found.</p>
  }

  return (
    <div className="overflow-x-auto rounded-lg bg-card">
      <Table>
        <TableHeader>
          <HeaderRow>
            <TableHead className="w-10 text-center">#</TableHead>
            <TableHead>Player</TableHead>
            <TableHead className="text-center">Pos</TableHead>
            <TableHead className="text-center">Age</TableHead>
            <TableHead className="text-right">Grade</TableHead>
            <TableHead className="text-right">Proj Pts</TableHead>
            <TableHead className="text-right">Pts/G</TableHead>
            <TableHead className="text-center">Confidence</TableHead>
            <TableHead>Profile</TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {players.map((p) => (
            <ClickableRow key={p.gsis_id} href={`/projections/${p.gsis_id}`}>
              <TableCell className="text-center text-muted-foreground tabular-nums">
                {p.overall_rank}
              </TableCell>
              <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
              <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
              <TableCell className="text-center text-muted-foreground tabular-nums">{p.age || '—'}</TableCell>
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
