import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, ZScoreCell, HeaderRow } from '@/components/ui/table-helpers'
import type { RosterPlayer, RankedPlayer } from '../../../api/client'

interface Props {
  roster: RosterPlayer[]
  statLabels: string[]
  rankByPlayer?: Map<string, RankedPlayer>
}

/** Roster table with dynamic stat columns derived from the selected stat period. */
export function RosterTable({ roster, statLabels, rankByPlayer }: Props) {
  const hasRankings = rankByPlayer && rankByPlayer.size > 0

  return (
    <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead>Player</TableHead>
            <TableHead>Team</TableHead>
            <TableHead>Pos</TableHead>
            <TableHead>Slot</TableHead>
            {hasRankings && <TableHead className="text-right">Value</TableHead>}
            {statLabels.map((label) => (
              <TableHead key={label} className="text-right">{label}</TableHead>
            ))}
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {roster.map((p) => {
            const rp = rankByPlayer?.get(p.player_key)
            const canLink = !!p.gsis_id
            return (
              <ClickableRow
                key={p.player_key}
                href={canLink ? `/players/${p.gsis_id}` : undefined}
              >
                <PlayerCell name={p.name.full} imageUrl={p.image_url} linked={canLink} avatarSize={32} />
                <TableCell className="text-muted-foreground">{p.team_abbr}</TableCell>
                <TableCell className="text-muted-foreground">{p.display_position}</TableCell>
                <TableCell className="text-muted-foreground">{p.selected_position.position}</TableCell>
                {hasRankings && (
                  <TableCell className="text-right tabular-nums">
                    {rp ? (
                      <div className="flex flex-col items-end gap-0.5">
                        <span className="text-xs text-muted-foreground">
                          {rp.overall_score > 0 ? '+' : ''}{rp.overall_score.toFixed(1)}
                          <span className="ml-1 text-foreground/60">#{rp.overall_rank}</span>
                        </span>
                        <span className="text-xs text-muted-foreground/60">
                          {p.display_position.split(',')[0]} #{rp.position_rank}
                        </span>
                      </div>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </TableCell>
                )}
                {statLabels.map((label) => {
                  const stat = p.stats?.find((s) => s.label === label)
                  const catScore = rp?.category_scores.find((c) => c.label === label)
                  if (catScore) {
                    return <ZScoreCell key={label} value={stat?.value ?? '—'} zScore={catScore.z_score} className="text-muted-foreground" />
                  }
                  return (
                    <TableCell key={label} className="text-right tabular-nums text-muted-foreground">
                      {stat?.value ?? '—'}
                    </TableCell>
                  )
                })}
              </ClickableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
