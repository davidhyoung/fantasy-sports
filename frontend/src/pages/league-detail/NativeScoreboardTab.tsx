import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { getLeagueScoreboard, generateLeagueSchedule, scoreLeagueWeek, type MatchupTeam } from '@/api/client'
import { keys } from '@/api/queryKeys'

function TeamNameLink({ team }: { team: MatchupTeam | undefined }) {
  if (!team) return null
  if (!team.team_id) return <span className="font-display text-sm font-semibold text-foreground">{team.name}</span>
  return (
    <RouterLink to={`/teams/${team.team_id}`} className="font-display text-sm font-semibold text-foreground hover:text-primary">
      {team.name}
    </RouterLink>
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
        <div className="flex gap-2">
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
        <div className="flex flex-col gap-2.5">
          {scoreboard.matchups.map((m, i) => (
            <div key={i} className="rounded-lg bg-card px-4 py-3">
              <div className="flex items-center justify-between">
                <TeamNameLink team={m.teams[0]} />
                <span className="font-mono text-sm tabular-nums text-foreground">{m.teams[0]?.points}</span>
              </div>
              <div className="mt-1 flex items-center justify-between border-t border-border pt-1">
                <TeamNameLink team={m.teams[1]} />
                <span className="font-mono text-sm tabular-nums text-foreground">{m.teams[1]?.points}</span>
              </div>
              <div className="mt-1.5 text-xs text-muted-foreground">
                {m.status === 'postevent' ? 'Final' : 'Not yet scored'}
              </div>
            </div>
          ))}
        </div>
      )}
    </>
  )
}
