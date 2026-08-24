import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import PlayerDetailBody from './PlayerDetailBody'

export default function PlayerDetail() {
  const { gsisId } = useParams<{ gsisId: string }>()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // Present when arriving from a native league's Roster/Players tab —
  // that's what lets the backend attach this player's contract in that
  // league, which the page can't otherwise know about (a player's contract
  // isn't a global attribute, it's scoped to one league's roster).
  const leagueId = Number(searchParams.get('league')) || undefined

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      {/* Back */}
      <button
        onClick={() => navigate(-1)}
        className="text-sm text-muted-foreground hover:text-primary"
      >
        ← Back
      </button>

      <PlayerDetailBody gsisId={gsisId ?? ''} leagueId={leagueId} />
    </div>
  )
}
