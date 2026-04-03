import { useParams, Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { useTeamDetail, PERIODS } from './hooks/useTeamDetail'
import { MatchupCard } from './components/MatchupCard'
import { RosterTable } from './components/RosterTable'

export default function TeamDetail() {
  const { id } = useParams<{ id: string }>()
  const teamId = Number(id)

  const {
    team, error,
    statPeriod, setStatPeriod,
    roster, rosterError, statLabels,
    scoreboard, matchup, thisTeam, opponent, matchupHref,
    rankByPlayer,
  } = useTeamDetail(teamId)

  if (error) return <p className="text-red-600 dark:text-red-400">{(error as Error).message}</p>
  if (!team) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  return (
    <div className="max-w-5xl">
      {/* Team header */}
      <div className="mb-6">
        <RouterLink to={`/leagues/${team.league_id}`} className="text-sm text-primary hover:underline">
          ← League
        </RouterLink>
        <div className="flex items-center gap-4 mt-2">
          {team.logo_url && (
            <img src={team.logo_url} alt={team.name} className="h-14 w-14 rounded object-contain shrink-0" />
          )}
          <div>
            <h1 className="text-2xl font-bold text-foreground">{team.name}</h1>
            {team.user_id && (
              <Badge className="mt-1 bg-purple-100 text-purple-700 border-purple-300 dark:bg-purple-900/40 dark:text-purple-300 dark:border-purple-700">Your team</Badge>
            )}
          </div>
        </div>
      </div>

      {/* Current week matchup */}
      {matchup && thisTeam && opponent && (
        <MatchupCard
          matchup={matchup}
          thisTeam={thisTeam}
          opponent={opponent}
          matchupHref={matchupHref}
          week={scoreboard!.week}
        />
      )}

      {/* Roster section */}
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-lg font-semibold text-foreground">Roster</h2>
        <div className="flex rounded-lg bg-muted overflow-hidden text-sm">
          {PERIODS.map((p) => (
            <button
              key={p.value}
              onClick={() => setStatPeriod(p.value)}
              className={`px-3 py-1.5 transition-colors ${
                statPeriod === p.value
                  ? 'bg-foreground text-background'
                  : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
      </div>

      {rosterError ? (
        <p className="text-red-600 dark:text-red-400 text-sm">{(rosterError as Error).message}</p>
      ) : roster === undefined ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : roster.length === 0 ? (
        <p className="text-muted-foreground">No players on roster.</p>
      ) : (
        <RosterTable roster={roster} statLabels={statLabels} rankByPlayer={rankByPlayer} />
      )}
    </div>
  )
}
