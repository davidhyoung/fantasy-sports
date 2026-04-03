import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { getLeagueRankings } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

// Yahoo-supported stat periods for the rankings endpoint.
export const RANKING_PERIODS = [
  { label: 'Today',      value: 'today'     },
  { label: 'Last Week',  value: 'lastweek'  },
  { label: 'Last Month', value: 'lastmonth' },
  { label: 'Season',     value: 'season'    },
] as const

export type RankingPeriod = (typeof RANKING_PERIODS)[number]['value']

/**
 * Manages stat period state (preset or single custom date) and fetches league
 * rankings for the Rankings tab. Query is lazy-loaded when the tab is active.
 *
 * Custom date maps to Yahoo's date;date=YYYY-MM-DD format.
 * date_range is not supported by Yahoo's API.
 */
export function useRankings(leagueId: number, active: boolean) {
  const [searchParams, setSearchParams] = useSearchParams()

  // 'custom' is a UI mode — not sent to the backend directly.
  const periodMode = (searchParams.get('period') ?? 'season') as RankingPeriod | 'custom'
  const customDate = searchParams.get('date') ?? ''

  const setPeriodMode = (mode: RankingPeriod | 'custom') =>
    setSearchParams((prev) => {
      prev.set('period', mode)
      if (mode !== 'custom') prev.delete('date')
      return prev
    }, { replace: true })

  const setCustomDate = (date: string) =>
    setSearchParams((prev) => {
      if (date) prev.set('date', date)
      else prev.delete('date')
      return prev
    }, { replace: true })

  // The actual stat_type string sent to the API.
  // Empty string when custom mode is active but no date is chosen yet.
  const effectiveStatType = useMemo(() => {
    if (periodMode === 'custom') {
      return customDate ? `date;date=${customDate}` : ''
    }
    return periodMode
  }, [periodMode, customDate])

  const { data: rankings, isLoading } = useQuery({
    queryKey: keys.leagueRankings(leagueId, effectiveStatType),
    queryFn: () => getLeagueRankings(leagueId, effectiveStatType),
    enabled: active && !!effectiveStatType,
  })

  return {
    rankings, isLoading,
    periodMode, setPeriodMode,
    customDate, setCustomDate,
    effectiveStatType,
  }
}
