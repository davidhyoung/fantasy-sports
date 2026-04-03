import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import type { RosterPlayer, RosterStat } from '../../../api/client'

interface Props {
  teamName: string
  roster: RosterPlayer[]
  statLabels: string[]
}

/** Roster table for one side of a matchup, with stat columns matching the shared label set. */
export function TeamRosterTable({ teamName, roster, statLabels }: Props) {
  return (
    <div className="mb-8">
      <h2 className="text-base font-semibold text-foreground mb-3">{teamName}</h2>
      <div className="rounded-lg bg-card">
        <Table>
          <TableHeader>
            <HeaderRow>
              <TableHead>Player</TableHead>
              <TableHead>Slot</TableHead>
              {statLabels.map((label) => (
                <TableHead key={label} className="text-right">{label}</TableHead>
              ))}
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {roster.map((p) => {
              const canLink = !!p.gsis_id
              return (
                <ClickableRow
                  key={p.player_key}
                  href={canLink ? `/players/${p.gsis_id}` : undefined}
                >
                  <PlayerCell name={p.name.full} imageUrl={p.image_url} linked={canLink} />
                  <TableCell className="text-muted-foreground">{p.selected_position.position}</TableCell>
                  {statLabels.map((label) => {
                    const stat = p.stats?.find((s: RosterStat) => s.label === label)
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
    </div>
  )
}
