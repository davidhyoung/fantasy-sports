import { useQuery } from '@tanstack/react-query'
import { getLeagueScoreboard } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

/** Lazily fetches the current week's scoreboard; only fires when the scoreboard tab is active. */
export function useScoreboard(leagueId: number, active: boolean) {
  return useQuery({
    queryKey: keys.scoreboard(leagueId),
    queryFn: () => getLeagueScoreboard(leagueId),
    enabled: active,
  })
}
