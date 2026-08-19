import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, PlayerAvatar, ClickableRow, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { describeStat } from '@/lib/statDescriptions'
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

      {/* Card list below md — face: Player, Slot; expansion: per-category stat values. */}
      <div className="space-y-2 md:hidden">
        {roster.map((p) => {
          const canLink = !!p.gsis_id
          const expanded: MobileStatField[] = statLabels.map((label) => ({
            label,
            value: p.stats?.find((s: RosterStat) => s.label === label)?.value ?? '—',
          }))
          return (
            <MobileStatCard
              key={p.player_key}
              href={canLink ? `/players/${p.gsis_id}` : undefined}
              leading={<PlayerAvatar src={p.image_url} alt={p.name.full} size={28} />}
              title={p.name.full}
              subtitle={p.selected_position.position}
              expanded={expanded}
            />
          )
        })}
      </div>

      <div className="hidden md:block rounded-lg bg-card">
        <Table>
          <TableHeader>
            <HeaderRow>
              <TableHead>Player</TableHead>
              <TableHead><HeaderTip description="Roster slot this player currently fills">Slot</HeaderTip></TableHead>
              {statLabels.map((label) => (
                <TableHead key={label} className="text-right">
                  <HeaderTip description={describeStat(label)}>{label}</HeaderTip>
                </TableHead>
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
                      <TableCell key={label} className="text-right font-mono tabular-nums text-muted-foreground">
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
