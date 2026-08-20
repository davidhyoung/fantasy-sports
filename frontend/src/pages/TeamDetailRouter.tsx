import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { getTeam, getLeague } from '../api/client'
import { keys } from '../api/queryKeys'
import TeamDetail from './team-detail'
import { NativeTeamDetail } from './league-detail/NativeTeamDetail'

/**
 * /teams/:id branches on the team's league source before rendering anything
 * Yahoo-shaped. The existing TeamDetail page (`useTeamDetail`) fires
 * getTeamRoster/getLeagueRankings unconditionally, both of which 422 for a
 * team with no yahoo_key — so a native team needs to be routed away before
 * that hook ever mounts, not branched inside it.
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

  return league.source === 'native' ? <NativeTeamDetail team={team} /> : <TeamDetail />
}
