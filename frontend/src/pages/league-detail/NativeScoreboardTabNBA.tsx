import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TeamAvatar } from '@/components/ui/table-helpers'
import {
  getLeagueScoreboardNBA, generateLeagueScheduleNBA, scoreLeagueWeekNBA,
  NBA_CATEGORY_LABELS, type NBAMatchup, type NBAMatchupTeam,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

function MatchupCardNBA({ m }: { m: NBAMatchup }) {
  const [t1, t2] = m.teams
  const scored = m.status === 'postevent'

  const teamHeader = (team: NBAMatchupTeam | undefined, align: 'left' | 'right') => {
    if (!team) return null
    return (
      <div className={`flex items-center gap-2 ${align === 'right' ? 'flex-row-reverse text-right' : ''}`}>
        <TeamAvatar name={team.name} />
        <span className="font-display text-sm font-semibold text-foreground">{team.name}</span>
        {scored && (
          <span className="font-mono text-xs tabular-nums text-muted-foreground">{team.categories_won}</span>
        )}
      </div>
    )
  }

  return (
    <div className="rounded-lg bg-card px-4 py-3">
      <div className="flex items-center justify-between">
        {teamHeader(t1, 'left')}
        <span className="text-xs text-muted-foreground">vs</span>
        {teamHeader(t2, 'right')}
      </div>

      {scored && t1?.categories && t2?.categories ? (
        <div className="mt-3 border-t border-border pt-2">
          {t1.categories.map((c1, i) => {
            const c2 = t2.categories![i]
            return (
              <div key={c1.category} className="flex items-center justify-between py-0.5 text-sm">
                <span className={`w-16 font-mono tabular-nums ${c1.won ? 'font-bold text-primary' : 'text-muted-foreground'}`}>
                  {c1.category === 'fg_pct' || c1.category === 'ft_pct' ? c1.value.toFixed(3) : c1.value}
                </span>
                <span className="font-display text-[11px] uppercase tracking-wide text-muted-foreground">
                  {NBA_CATEGORY_LABELS[c1.category]}
                </span>
                <span className={`w-16 text-right font-mono tabular-nums ${c2.won ? 'font-bold text-primary' : 'text-muted-foreground'}`}>
                  {c2.category === 'fg_pct' || c2.category === 'ft_pct' ? c2.value.toFixed(3) : c2.value}
                </span>
              </div>
            )
          })}
        </div>
      ) : (
        <div className="mt-1.5 text-xs text-muted-foreground">Awaiting scoring · week {m.week}</div>
      )}
    </div>
  )
}

interface Props {
  leagueId: number
  active: boolean
}

/**
 * Scoreboard for a native NBA league — a roto category grid per matchup
 * (GetLeagueScoreboardNBA), not a single point total. Scoring is an
 * explicit, re-runnable commissioner action (ScoreLeagueWeekNBA), same
 * philosophy as the NFL side — real stats come from a manual
 * `make import-nba -stats` batch import, never live, this phase.
 */
export function NativeScoreboardTabNBA({ leagueId, active }: Props) {
  const qc = useQueryClient()
  const [week, setWeek] = useState<number | undefined>(undefined)
  const [seasonStartDate, setSeasonStartDate] = useState('')

  const { data: scoreboard, isFetching } = useQuery({
    queryKey: keys.scoreboardNBA(leagueId, week),
    queryFn: () => getLeagueScoreboardNBA(leagueId, week),
    enabled: active,
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'scoreboard-nba'] })
    qc.invalidateQueries({ queryKey: keys.standingsNBA(leagueId) })
  }

  const generateMutation = useMutation({
    mutationFn: () => generateLeagueScheduleNBA(leagueId, seasonStartDate),
    onSuccess: invalidate,
  })

  const scoreMutation = useMutation({
    mutationFn: (w: number) => scoreLeagueWeekNBA(leagueId, w),
    onSuccess: invalidate,
  })

  if (!active) return null

  const currentWeek = scoreboard?.week ?? 0
  const hasSchedule = currentWeek > 0

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        {hasSchedule ? (
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" disabled={currentWeek <= 1} onClick={() => setWeek(currentWeek - 1)}>← Prev</Button>
            <span className="font-display text-sm font-semibold text-foreground">Week {currentWeek}</span>
            <Button variant="outline" size="sm" onClick={() => setWeek(currentWeek + 1)}>Next →</Button>
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground">No schedule generated yet.</span>
            <Input
              type="date" value={seasonStartDate}
              onChange={(e) => setSeasonStartDate(e.target.value)}
              className="w-40"
            />
          </div>
        )}
        <div className="flex items-center gap-2">
          {hasSchedule && (
            <span className="text-xs text-muted-foreground">Scoring is manual — run the week's stat import first</span>
          )}
          {!hasSchedule && (
            <Button size="sm" onClick={() => generateMutation.mutate()} disabled={generateMutation.isPending || !seasonStartDate}>
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
        <p className="text-muted-foreground">Loading…</p>
      ) : !scoreboard || scoreboard.matchups.length === 0 ? (
        <p className="text-muted-foreground">No matchups this week.</p>
      ) : (
        <div className="grid grid-cols-1 gap-2.5 md:grid-cols-2">
          {scoreboard.matchups.map((m, i) => (
            <MatchupCardNBA key={i} m={m} />
          ))}
        </div>
      )}
    </>
  )
}
