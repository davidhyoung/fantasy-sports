import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Dialog } from '@/components/ui/dialog'
import { getLeagueRosters, getLeagueScoreboard, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { NativeRosterTable } from './NativeRosterTable'
import { EditContractForm } from './EditContractForm'

interface Props {
  leagueId: number
  teamId: number
  /** Someone else's team, viewed from its /teams/:id page — matchup card
   *  stays, but the roster renders read-only (no slot editing, no Edit/Drop). */
  readOnly?: boolean
}

/**
 * The current week's matchup card plus roster for one native-league team —
 * shared by the personal My Team tab (editable) and the read-only
 * /teams/:id page anyone can reach by clicking a team name.
 */
export function NativeTeamOverview({ leagueId, teamId, readOnly = false }: Props) {
  const [editing, setEditing] = useState<RosterEntry | null>(null)

  const { data: rosters, isFetching: loadingRosters } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
  })
  const { data: scoreboard } = useQuery({
    queryKey: keys.scoreboard(leagueId),
    queryFn: () => getLeagueScoreboard(leagueId),
  })

  const roster = (rosters ?? []).filter((r) => r.team_id === teamId)
  const matchup = scoreboard?.matchups.find((m) => m.teams.some((t) => t.team_id === teamId))
  const me = matchup?.teams.find((t) => t.team_id === teamId)
  const opponent = matchup?.teams.find((t) => t.team_id !== teamId)

  return (
    <>
      {matchup && me && opponent ? (
        <div className="mb-6 rounded-lg bg-card px-4 py-3">
          <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Week {matchup.week} {matchup.status === 'postevent' ? '· Final' : '· Not yet scored'}
          </div>
          <div className="mt-1.5 flex items-center justify-between">
            <span className="font-display text-sm font-semibold text-foreground">{me.name}</span>
            <span className="font-mono text-lg font-bold tabular-nums text-foreground">
              {me.points} <span className="text-muted-foreground">–</span> {opponent.points}
            </span>
            {opponent.team_id ? (
              <RouterLink to={`/teams/${opponent.team_id}`} className="font-display text-sm font-semibold text-foreground hover:text-primary">
                {opponent.name}
              </RouterLink>
            ) : (
              <span className="font-display text-sm font-semibold text-foreground">{opponent.name}</span>
            )}
          </div>
        </div>
      ) : (
        <p className="mb-6 text-sm text-muted-foreground">
          No matchup this week — either no schedule has been generated yet, or this team has a bye.
        </p>
      )}

      {loadingRosters ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : (
        <NativeRosterTable leagueId={leagueId} roster={roster} onEdit={readOnly ? undefined : setEditing} readOnly={readOnly} />
      )}

      {!readOnly && (
        <Dialog open={editing != null} onClose={() => setEditing(null)} title="Edit contract">
          {editing && <EditContractForm leagueId={leagueId} entry={editing} onClose={() => setEditing(null)} />}
        </Dialog>
      )}
    </>
  )
}
