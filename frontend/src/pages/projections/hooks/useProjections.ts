import { useQuery } from '@tanstack/react-query'
import { getProjections } from '@/api/client'
import { keys } from '@/api/queryKeys'

export function useProjections(params: {
  season?: number
  position: string
  sort: string
}) {
  const season = params.season ?? 2026
  return useQuery({
    queryKey: keys.projections(season, params.position, params.sort),
    queryFn: () => getProjections({ season, position: params.position || undefined, sort: params.sort }),
    staleTime: 5 * 60 * 1000, // projections are stable — 5 min
  })
}
