import { Link as RouterLink } from 'react-router-dom'
import type { Team } from '@/api/client'
import { NativeTeamOverview } from './components/NativeTeamOverview'

interface Props {
  leagueId: number
  active: boolean
  myTeam: Team
}

/**
 * Native-league My Team tab — your matchup for the current week plus your
 * roster, with the same inline-editable Slot dropdown as the commissioner
 * Roster tab (that IS the lineup-setting mechanism — see NativeRosterTable,
 * shared via NativeTeamOverview). A separate component from the
 * commissioner Roster tab, not a lock-to-team mode of it: this is meant to
 * feel like "my team", not "the league viewed through one team's eyes" (no
 * team switcher, no trade/assign/rollover controls — those stay on the
 * Roster tab). Mirrors the Yahoo MyTeamTab convention of also linking out to
 * the standalone /teams/:id page even though the content is already inline.
 */
export function NativeMyTeamTab({ leagueId, active, myTeam }: Props) {
  if (!active) return null

  return (
    <>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-display text-lg font-bold text-foreground">{myTeam.name}</h2>
        <RouterLink to={`/teams/${myTeam.id}`} className="font-display text-xs font-semibold text-primary hover:text-primary-hover">
          Full team page →
        </RouterLink>
      </div>
      <NativeTeamOverview leagueId={leagueId} teamId={myTeam.id} />
    </>
  )
}
