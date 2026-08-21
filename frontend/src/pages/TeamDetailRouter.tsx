import { useParams, Navigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { getTeam, getLeague } from '../api/client'
import { keys } from '../api/queryKeys'
import TeamDetail from './team-detail'

/**
 * /teams/:id branches on the team's league source before rendering anything
 * Yahoo-shaped. The existing TeamDetail page (`useTeamDetail`) fires
 * getTeamRoster/getLeagueRankings unconditionally, both of which 422 for a
 * team with no yahoo_key — so a native team needs to be routed away before
 * that hook ever mounts, not branched inside it.
 *
 * A native team has no standalone page at all — its "full team page" and
 * its Roster-tab view are the same content (there's no reader who isn't the
 * commissioner in the single-user model), so this just redirects into the
 * league's Roster tab with that team selected. Anything the app itself
 * links to a native team goes straight to that URL already; this redirect
 * only matters for an old bookmark or stray `/teams/:id` reference.
 */
export default function TeamDetailRouter() {
  const { id } = useParams<{ id: string }>()
  const teamId = Number(id)

  const { data: team, error: teamError } = useQuery({
    queryKey: keys.team(teamId),
    queryFn: () => getTeam(teamId),
  })
  const { data: league, error: leagueError } = useQuery({
    queryKey: keys.league(team?.league_id ?? 0),
    queryFn: () => getLeague(team!.league_id),
    enabled: !!team,
  })

  const error = teamError ?? leagueError
  if (error) return <p className="text-destructive">{(error as Error).message}</p>
  if (!team || !league) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  return league.source === 'native'
    ? <Navigate to={`/leagues/${team.league_id}?tab=roster&team=${team.id}`} replace />
    : <TeamDetail />
}
