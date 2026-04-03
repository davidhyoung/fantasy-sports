import { useQuery } from '@tanstack/react-query'
import { getGrades } from '@/api/client'
import { keys } from '@/api/queryKeys'

export function useRankings(params: { season: number; position: string }) {
  return useQuery({
    queryKey: keys.grades(params.season, params.position),
    queryFn: () => getGrades({ season: params.season, position: params.position || undefined, limit: 300 }),
    staleTime: 5 * 60 * 1000,
  })
}
