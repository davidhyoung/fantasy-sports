import { useQuery } from '@tanstack/react-query'
import { getProjectionDetail } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PROJECTION_SEASON } from '@/lib/constants'

export function useProjectionDetail(gsisId: string, season = PROJECTION_SEASON) {
  return useQuery({
    queryKey: keys.projectionDetail(gsisId, season),
    queryFn: () => getProjectionDetail(gsisId, season),
    enabled: !!gsisId,
    staleTime: 5 * 60 * 1000,
  })
}
