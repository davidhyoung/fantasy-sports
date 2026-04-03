import { Loader2 } from 'lucide-react'
import { Link as RouterLink, useNavigate } from 'react-router-dom'
import { useScoreboard } from './hooks/useScoreboard'

interface Props {
  leagueId: number
  active: boolean
  yahooKeyToId: Record<string, number>
}

/** Renders the current week's matchup cards; each card links to the full matchup detail page. */
export function ScoreboardTab({ leagueId, active, yahooKeyToId }: Props) {
  const navigate = useNavigate()
  const { data: scoreboard, error } = useScoreboard(leagueId, active)

  if (error) return <p className="text-red-600 dark:text-red-400 text-sm">{(error as Error).message}</p>
  if (!scoreboard) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  if (scoreboard.matchups.length === 0) return <p className="text-muted-foreground">No matchups available for this week.</p>

  return (
    <div>
      <p className="text-sm text-muted-foreground mb-4">Week {scoreboard.week}</p>
      <div className="flex flex-col gap-3">
        {scoreboard.matchups.map((m, i) => {
          const t1Id = yahooKeyToId[m.teams[0]?.team_key]
          const t2Id = yahooKeyToId[m.teams[1]?.team_key]
          const matchupHref = t1Id && t2Id
            ? `/leagues/${leagueId}/matchup/${scoreboard.week}/${t1Id}/${t2Id}`
            : null

          return (
            <div
              key={i}
              className={`bg-card rounded-lg border border-border/30 p-4${matchupHref ? ' hover:bg-muted/30 transition-colors cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring' : ''}`}
              onClick={() => matchupHref && navigate(matchupHref)}
              onKeyDown={matchupHref ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); navigate(matchupHref) } } : undefined}
              tabIndex={matchupHref ? 0 : undefined}
              role={matchupHref ? 'link' : undefined}
            >
              {m.teams.map((t, j) => {
                const teamId = yahooKeyToId[t.team_key]
                return (
                  <div
                    key={t.team_key}
                    className={`flex justify-between items-center gap-2${j === 0 ? ' pb-2 mb-2 border-b border-border' : ''}`}
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      {t.logo_url && (
                        <img src={t.logo_url} alt={t.name} className="h-7 w-7 rounded object-contain shrink-0" />
                      )}
                      {teamId ? (
                        <RouterLink
                          to={`/teams/${teamId}`}
                          onClick={(e) => e.stopPropagation()}
                          className="font-medium text-foreground hover:underline truncate"
                        >
                          {t.name}
                        </RouterLink>
                      ) : (
                        <p className="font-medium text-foreground truncate">{t.name}</p>
                      )}
                    </div>
                    <p className="text-lg font-bold tabular-nums shrink-0">{t.points || '—'}</p>
                  </div>
                )
              })}
              <p className="text-xs text-muted-foreground mt-2">
                {m.week_start} – {m.week_end}
                {m.is_playoffs === '1' ? ' · Playoffs' : ''}
              </p>
            </div>
          )
        })}
      </div>
    </div>
  )
}
