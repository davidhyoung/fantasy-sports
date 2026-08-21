import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { TeamAvatar } from '@/components/ui/table-helpers'
import { getLeagueScoreboard, generateLeagueSchedule, scoreLeagueWeek, type Matchup, type MatchupTeam } from '@/api/client'
import { keys } from '@/api/queryKeys'

function TeamNameLink({ leagueId, team, bold }: { leagueId: number; team: MatchupTeam | undefined; bold?: boolean }) {
  if (!team) return null
  const nameClass = `font-display text-sm ${bold ? 'font-bold' : 'font-semibold'} text-foreground`
  return (
    <div className="flex items-center gap-2">
      <TeamAvatar name={team.name} />
      {!team.team_id ? (
        <span className={nameClass}>{team.name}</span>
      ) : (
        <RouterLink to={`/leagues/${leagueId}?tab=roster&team=${team.team_id}`} className={`${nameClass} hover:text-primary`}>
          {team.name}
        </RouterLink>
      )}
    </div>
  )
}

/** Winner gets weight, not decoration — a tinted row, bold name + score, and
 *  a small "W" pill, all pulled from tokens already in the system (design
 *  review, 2026-08-21). Ties and unplayed matchups get no winner treatment. */
function MatchupCard({ leagueId, m }: { leagueId: number; m: Matchup }) {
  const [t1, t2] = m.teams
  const scored = m.status === 'postevent'
  const p1 = t1 ? parseFloat(t1.points) : 0
  const p2 = t2 ? parseFloat(t2.points) : 0
  const winner = scored && p1 !== p2 ? (p1 > p2 ? t1 : t2) : undefined

  const teamRow = (team: MatchupTeam | undefined) => {
    if (!team) return null
    const isWinner = winner && team.team_id === winner.team_id && !!team.team_id
    const scoreClass = isWinner
      ? 'font-mono text-lg font-bold tabular-nums text-primary'
      : scored
      ? 'font-mono text-base tabular-nums text-muted-foreground'
      : 'font-mono text-base tabular-nums text-muted-foreground'
    return (
      <div className={`flex items-center justify-between gap-3 ${isWinner ? 'bg-positive-light -mx-4 -mt-3 mb-1 rounded-t-lg px-4 pb-2 pt-3' : ''}`}>
        <div className="flex items-center gap-2">
          <TeamNameLink leagueId={leagueId} team={team} bold={isWinner} />
          {isWinner && <Badge className="px-1.5 py-0.5 text-[9px]">W</Badge>}
        </div>
        {scored ? (
          <span className={scoreClass}>{team.points}</span>
        ) : (
          <span className="font-mono text-base tabular-nums text-muted-foreground">
            {team.projected_points || '—'} <span className="text-[10px]">proj</span>
          </span>
        )}
      </div>
    )
  }

  return (
    <div className="rounded-lg bg-card px-4 py-3">
      {teamRow(t1)}
      <div className="mt-1 border-t border-border pt-1">{teamRow(t2)}</div>
      <div className="mt-1.5 text-xs text-muted-foreground">
        {scored ? 'Final' : `Awaiting scoring · week ${m.week}`}
      </div>
    </div>
  )
}

interface Props {
  leagueId: number
  active: boolean
}

/**
 * Scoreboard for a native league. Scoring is an explicit commissioner
 * action, not automatic — the underlying stats arrive via a manual batch
 * import (make import-nfl), never live, so "Score this week" is something
 * you click after importing that week's stats, not something that updates
 * during Sunday games.
 */
export function NativeScoreboardTab({ leagueId, active }: Props) {
  const qc = useQueryClient()
  const [week, setWeek] = useState<number | undefined>(undefined)

  const { data: scoreboard, isFetching } = useQuery({
    queryKey: keys.scoreboard(leagueId, week),
    queryFn: () => getLeagueScoreboard(leagueId, week),
    enabled: active,
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'scoreboard'] })
    qc.invalidateQueries({ queryKey: keys.standings(leagueId) })
  }

  const generateMutation = useMutation({
    mutationFn: () => generateLeagueSchedule(leagueId),
    onSuccess: invalidate,
  })

  const scoreMutation = useMutation({
    mutationFn: (w: number) => scoreLeagueWeek(leagueId, w),
    onSuccess: invalidate,
  })

  if (!active) return null

  const currentWeek = scoreboard?.week ?? 0
  const hasSchedule = currentWeek > 0

  return (
    <>
      <div className="mb-4 flex items-center justify-between">
        {hasSchedule ? (
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={currentWeek <= 1} onClick={() => setWeek(currentWeek - 1)}>← Prev</Button>
            <span className="font-display text-sm font-semibold text-foreground">Week {currentWeek}</span>
            <Button variant="outline" size="sm" onClick={() => setWeek(currentWeek + 1)}>Next →</Button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No schedule generated yet.</p>
        )}
        <div className="flex items-center gap-2">
          {hasSchedule && (
            <span className="text-xs text-muted-foreground">Scoring is manual — run the week's stat import first</span>
          )}
          {!hasSchedule && (
            <Button size="sm" onClick={() => generateMutation.mutate()} disabled={generateMutation.isPending}>
              {generateMutation.isPending ? 'Generating…' : 'Generate schedule'}
            </Button>
          )}
          {hasSchedule && (
            <Button
              size="sm" variant="outline"
              onClick={() => scoreMutation.mutate(currentWeek)}
              disabled={scoreMutation.isPending}
            >
              {scoreMutation.isPending ? 'Scoring…' : `Score week ${currentWeek}`}
            </Button>
          )}
        </div>
      </div>
      {generateMutation.error && <p className="mb-3 text-sm text-destructive">{(generateMutation.error as Error).message}</p>}
      {scoreMutation.error && <p className="mb-3 text-sm text-destructive">{(scoreMutation.error as Error).message}</p>}

      {isFetching ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : !scoreboard || scoreboard.matchups.length === 0 ? (
        <p className="text-muted-foreground">No matchups this week.</p>
      ) : (
        <div className="grid grid-cols-1 gap-2.5 md:grid-cols-2">
          {scoreboard.matchups.map((m, i) => (
            <MatchupCard key={i} leagueId={leagueId} m={m} />
          ))}
        </div>
      )}
    </>
  )
}
