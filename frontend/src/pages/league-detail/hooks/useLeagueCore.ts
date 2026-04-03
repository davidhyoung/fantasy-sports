import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getLeague, listLeagueTeams } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

/** Fetches the league and its teams; derives a yahooKey → internalId map. */
export function useLeagueCore(leagueId: number) {
  const { data: league, error } = useQuery({
    queryKey: keys.league(leagueId),
    queryFn: () => getLeague(leagueId),
  })

  const { data: teams = [] } = useQuery({
    queryKey: keys.leagueTeams(leagueId),
    queryFn: () => listLeagueTeams(leagueId),
    enabled: !!league,
  })

  const yahooKeyToId = useMemo(() => {
    const m: Record<string, number> = {}
    for (const t of teams) if (t.yahoo_key) m[t.yahoo_key] = t.id
    return m
  }, [teams])

  return { league, teams, yahooKeyToId, error }
}
