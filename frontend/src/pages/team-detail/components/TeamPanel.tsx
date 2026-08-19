import { Loader2 } from 'lucide-react'
import { useTeamDetail, PERIODS } from '../hooks/useTeamDetail'
import { MatchupCard } from './MatchupCard'
import { RosterTable } from './RosterTable'

/**
 * A team's current matchup + roster. Shared by the standalone `/teams/:id` page
 * and the league page's "My Team" tab, which differ only in the header above this.
 */
export function TeamPanel({ teamId }: { teamId: number }) {
  const {
    statPeriod, setStatPeriod,
    roster, rosterError, statLabels,
    scoreboard, matchup, thisTeam, opponent, matchupHref,
    rankByPlayer,
  } = useTeamDetail(teamId)

  return (
    <>
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
      <div className="flex items-center justify-between gap-3 mb-3">
        <h2 className="text-lg font-semibold text-foreground shrink-0">Roster</h2>
        {/* Horizontal-scroll rather than wrap to two lines when the 4 options don't fit at 375px. */}
        <div className="flex rounded-lg bg-muted overflow-x-auto text-sm max-w-full">
          {PERIODS.map((p) => (
            <button
              key={p.value}
              onClick={() => setStatPeriod(p.value)}
              className={`px-3 py-1.5 whitespace-nowrap ${
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
        <p className="text-destructive text-sm">{(rosterError as Error).message}</p>
      ) : roster === undefined ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : roster.length === 0 ? (
        <p className="text-muted-foreground">No players on roster.</p>
      ) : (
        <RosterTable roster={roster} statLabels={statLabels} rankByPlayer={rankByPlayer} />
      )}
    </>
  )
}
