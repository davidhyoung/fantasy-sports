import { useQuery } from '@tanstack/react-query'
import { getDivergences, type RankingsFormat } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PROJECTION_SEASON } from '@/lib/constants'

export function useDivergences(params: {
  season?: number
  format: RankingsFormat
  position: string
}) {
  const season = params.season ?? PROJECTION_SEASON
  return useQuery({
    queryKey: keys.divergences(season, params.format, params.position),
    queryFn: () =>
      getDivergences({
        season,
        format: params.format,
        position: params.position || undefined,
        limit: 300,
      }),
    staleTime: 5 * 60 * 1000, // divergences are recomputed in batch, not live
  })
}
