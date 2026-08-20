import { Link as RouterLink } from 'react-router-dom'
import type { Team } from '@/api/client'
import { NativeTeamOverview } from './components/NativeTeamOverview'

interface Props {
  team: Team
}

/**
 * Read-only team page for a native league — what clicking a team name
 * anywhere (Standings, Scoreboard, Draft Picks) lands on. Same content as
 * NativeMyTeamTab (matchup card + roster, via the shared
 * NativeTeamOverview), just without the editing affordances, since this is
 * someone else's team. Routed at /teams/:id via TeamDetailRouter, which
 * picks this over the Yahoo-shaped TeamDetail page based on league.source.
 */
export function NativeTeamDetail({ team }: Props) {
  return (
    <div className="max-w-5xl">
      <div className="mb-6">
        <RouterLink to={`/leagues/${team.league_id}`} className="text-sm text-primary hover:underline">
          ← League
        </RouterLink>
        <div className="flex items-center gap-4 mt-2">
          <h1 className="text-2xl font-bold text-foreground">{team.name}</h1>
        </div>
      </div>

      <NativeTeamOverview leagueId={team.league_id} teamId={team.id} readOnly />
    </div>
  )
}
