import { Link as RouterLink } from 'react-router-dom'
import type { MatchupTeam, Matchup } from '../../../api/client'

interface Props {
  matchup: Matchup
  thisTeam: MatchupTeam
  opponent: MatchupTeam
  matchupHref: string | null
  week: number
}

/** Card showing the team's current-week matchup score. Clicking through navigates to full matchup detail. */
export function MatchupCard({ matchup, thisTeam, opponent, matchupHref, week }: Props) {
  const cardContent = (
    <>
      <p className="text-xs text-muted-foreground mb-3">
        Week {week} · {matchup.week_start} – {matchup.week_end}
        {matchup.is_playoffs === '1' ? ' · Playoffs' : ''}
      </p>
      <div className="flex items-center gap-4">
        <div className="flex-1">
          <div className="flex items-center gap-2 mb-1">
            {thisTeam.logo_url && (
              <img src={thisTeam.logo_url} alt={thisTeam.name} className="h-7 w-7 rounded object-contain" />
            )}
            <p className="text-sm font-semibold text-foreground">{thisTeam.name}</p>
          </div>
          <p className="text-2xl font-bold tabular-nums">{thisTeam.points || '0'}</p>
          {thisTeam.projected_points && (
            <p className="text-xs text-muted-foreground">proj. {thisTeam.projected_points}</p>
          )}
        </div>

        <span className="text-lg font-bold text-muted-foreground">vs</span>

        <div className="flex-1 text-right">
          <div className="flex items-center justify-end gap-2 mb-1">
            <p className="text-sm font-semibold text-foreground">{opponent.name}</p>
            {opponent.logo_url && (
              <img src={opponent.logo_url} alt={opponent.name} className="h-7 w-7 rounded object-contain" />
            )}
          </div>
          <p className="text-2xl font-bold tabular-nums">{opponent.points || '0'}</p>
          {opponent.projected_points && (
            <p className="text-xs text-muted-foreground">proj. {opponent.projected_points}</p>
          )}
        </div>
      </div>
    </>
  )

  if (matchupHref) {
    return (
      <RouterLink
        to={matchupHref}
        className="block mb-6 bg-card rounded-lg border border-border/30 p-4 hover:bg-muted/30 transition-colors"
      >
        {cardContent}
      </RouterLink>
    )
  }

  return (
    <div className="bg-card rounded-lg border border-border/30 p-4 mb-6">
      {cardContent}
    </div>
  )
}
