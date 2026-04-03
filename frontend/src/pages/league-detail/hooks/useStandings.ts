import { useQuery } from '@tanstack/react-query'
import { getLeagueStandings } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

/** Lazily fetches league standings; only fires when the standings tab is active. */
export function useStandings(leagueId: number, active: boolean) {
  return useQuery({
    queryKey: keys.standings(leagueId),
    queryFn: () => getLeagueStandings(leagueId),
    enabled: active,
  })
}
