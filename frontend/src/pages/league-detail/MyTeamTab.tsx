import { Link as RouterLink } from 'react-router-dom'
import type { Team } from '@/api/client'
import { TeamPanel } from '@/pages/team-detail/components/TeamPanel'

interface Props {
  myTeam: Team
}

/** Your own roster, in league context. Same body as the `/teams/:id` page. */
export function MyTeamTab({ myTeam }: Props) {
  return (
    <div className="max-w-5xl">
      <div className="flex items-center justify-between gap-4 mb-4">
        <div className="flex items-center gap-3">
          {myTeam.logo_url && (
            <img src={myTeam.logo_url} alt={myTeam.name} className="h-10 w-10 rounded object-contain shrink-0" />
          )}
          <h2 className="text-lg font-semibold text-foreground">{myTeam.name}</h2>
        </div>
        <RouterLink to={`/teams/${myTeam.id}`} className="font-display text-[13px] text-primary hover:underline">
          Full team page →
        </RouterLink>
      </div>

      <TeamPanel teamId={myTeam.id} />
    </div>
  )
}
