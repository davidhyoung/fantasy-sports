import { useParams, Navigate } from 'react-router-dom'

/**
 * `/leagues/:id/my-team` — a stable deep link to your own team in a league.
 * Opens the league page's My Team tab, which resolves the owning team itself
 * (and falls back to Standings if you don't have one here).
 */
export default function MyTeamRedirect() {
  const { id } = useParams<{ id: string }>()
  return <Navigate to={`/leagues/${id}?tab=my-team`} replace />
}
