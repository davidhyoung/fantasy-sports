import { useSearchParams } from 'react-router-dom'
import type { StatView } from '@/api/client'

export type { StatView }

/**
 * Shared state for the stat-view toggle, backed by `statView`/`statWeek` URL
 * params — same convention as team-detail's `period` param, and the reason
 * switching between the Roster and Players tabs preserves your chosen view
 * (both live under the same league-detail route, sharing search params).
 * The one place this is defined, so the Roster and Players tabs can't drift
 * on the vocabulary or the URL-param wiring.
 */
export function useStatView() {
  const [searchParams, setSearchParams] = useSearchParams()
  const view = (searchParams.get('statView') as StatView | null) ?? 'season'
  const week = Math.max(1, Number(searchParams.get('statWeek')) || 1)

  const setView = (v: StatView) => {
    setSearchParams((prev) => {
      prev.set('statView', v)
      if (v === 'week' && !prev.get('statWeek')) prev.set('statWeek', String(week))
      return prev
    }, { replace: true })
  }

  const setWeek = (w: number) => {
    setSearchParams((prev) => {
      prev.set('statView', 'week')
      prev.set('statWeek', String(Math.max(1, w)))
      return prev
    }, { replace: true })
  }

  return { view, week, setView, setWeek }
}
