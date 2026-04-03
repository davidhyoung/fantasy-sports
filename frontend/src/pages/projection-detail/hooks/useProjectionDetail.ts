import { useQuery } from '@tanstack/react-query'
import { getProjectionDetail } from '@/api/client'
import { keys } from '@/api/queryKeys'

export function useProjectionDetail(gsisId: string, season = 2026) {
  return useQuery({
    queryKey: keys.projectionDetail(gsisId, season),
    queryFn: () => getProjectionDetail(gsisId, season),
    enabled: !!gsisId,
    staleTime: 5 * 60 * 1000,
  })
}
