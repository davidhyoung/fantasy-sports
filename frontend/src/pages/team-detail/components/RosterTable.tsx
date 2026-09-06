import { useState } from 'react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, PlayerAvatar, ClickableRow, ZScoreCell, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { describeStat } from '@/lib/statDescriptions'
import { zScoreIndicator, zScoreColor } from '@/lib/utils'
import { PlayerDetailPanel } from '@/pages/player-detail/PlayerDetailPanel'
import type { RosterPlayer, RankedPlayer } from '../../../api/client'

interface Props {
  roster: RosterPlayer[]
  statLabels: string[]
  rankByPlayer?: Map<string, RankedPlayer>
}

/** Roster table with dynamic stat columns derived from the selected stat period. */
export function RosterTable({ roster, statLabels, rankByPlayer }: Props) {
  const hasRankings = rankByPlayer && rankByPlayer.size > 0
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)

  return (
    <>
      {/* Card list below md — face: Player, Slot, Value; expansion: Team, Pos,
          per-stat z-score cells. */}
      <div className="space-y-2 md:hidden">
        {roster.map((p) => {
          const rp = rankByPlayer?.get(p.player_key)
          const canLink = !!p.gsis_id
          const expanded: MobileStatField[] = [
            { label: 'Team', value: p.team_abbr },
            { label: 'Pos', value: p.display_position },
          ]
          for (const label of statLabels) {
            const stat = p.stats?.find((s) => s.label === label)
            const catScore = rp?.category_scores.find((c) => c.label === label)
            expanded.push({
              label,
              value: catScore ? (
                <>
                  {stat?.value ?? '—'}
                  <span className={`ml-0.5 text-[10px] ${zScoreColor(catScore.z_score)}`}>
                    {zScoreIndicator(catScore.z_score) || '●'}
                  </span>
                </>
              ) : (stat?.value ?? '—'),
            })
          }
          return (
            <MobileStatCard
              key={p.player_key}
              onClick={canLink ? () => setViewingPlayer(p.gsis_id!) : undefined}
              leading={<PlayerAvatar src={p.image_url} alt={p.name.full} size={32} />}
              title={p.name.full}
              subtitle={p.selected_position.position}
              face={
                hasRankings && rp ? (
                  <div className="text-right">
                    <div className="font-mono text-xs tabular-nums">
                      {rp.overall_score > 0 ? '+' : ''}{rp.overall_score.toFixed(1)}
                    </div>
                    <div className="mt-0.5 font-mono text-[11px] tabular-nums text-muted-foreground">
                      #{rp.overall_rank}
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
            <TableHead>Player</TableHead>
            <TableHead>Team</TableHead>
            <TableHead><HeaderTip description="Position eligibility">Pos</HeaderTip></TableHead>
            <TableHead><HeaderTip description="Roster slot this player currently fills">Slot</HeaderTip></TableHead>
            {hasRankings && (
              <TableHead className="text-right">
                <HeaderTip description="Overall value score and rank, then position rank on the line below">Value</HeaderTip>
              </TableHead>
            )}
            {statLabels.map((label) => (
              <TableHead key={label} className="text-right">
                <HeaderTip description={describeStat(label)}>{label}</HeaderTip>
              </TableHead>
            ))}
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {roster.map((p) => {
            const rp = rankByPlayer?.get(p.player_key)
            const canLink = !!p.gsis_id
            return (
              <ClickableRow key={p.player_key}>
                <PlayerCell
                  name={p.name.full}
                  imageUrl={p.image_url}
                  onClick={canLink ? () => setViewingPlayer(p.gsis_id!) : undefined}
                  avatarSize={32}
                  notes={p.notes}
                />
                <TableCell className="text-muted-foreground">{p.team_abbr}</TableCell>
                <TableCell className="text-muted-foreground">{p.display_position}</TableCell>
                <TableCell className="text-muted-foreground">{p.selected_position.position}</TableCell>
                {hasRankings && (
                  <TableCell className="text-right font-mono tabular-nums">
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

      <PlayerDetailPanel gsisId={viewingPlayer} onClose={() => setViewingPlayer(null)} />
    </>
  )
}
