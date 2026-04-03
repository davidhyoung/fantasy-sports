import { useParams, Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { useMatchupDetail } from './hooks/useMatchupDetail'
import { CategoryTotalsTable } from './components/CategoryTotalsTable'
import { TeamRosterTable } from './components/TeamRosterTable'

export default function MatchupDetail() {
  const { leagueId: leagueIdStr, week: weekStr, t1: t1Str, t2: t2Str } = useParams<{
    leagueId: string
    week: string
    t1: string
    t2: string
  }>()
  const leagueId = Number(leagueIdStr)
  const week = Number(weekStr)
  const t1Id = Number(t1Str)
  const t2Id = Number(t2Str)

  const {
    loading, error,
    matchup, t1Score, t2Score, t1Name, t2Name,
    roster1, roster2,
    statLabels, accum1, accum2,
    teamValue, wins, fmt,
  } = useMatchupDetail(leagueId, week, t1Id, t2Id)

  return (
    <div className="max-w-6xl">
      <RouterLink to={`/leagues/${leagueId}`} className="text-sm text-primary hover:underline">
        ← League
      </RouterLink>

      {error && <p className="text-red-600 dark:text-red-400 text-sm mt-4">{(error as Error).message}</p>}

      {/* Matchup header with scores */}
      <div className="mt-4 mb-6">
        <p className="text-xs text-muted-foreground mb-2">
          Week {week}{matchup ? ` · ${matchup.week_start} – ${matchup.week_end}` : ''}
          {matchup?.is_playoffs === '1' ? ' · Playoffs' : ''}
        </p>
        <div className="flex items-center gap-6">
          <div className="flex-1">
            <div className="flex items-center gap-3 mb-1">
              {t1Score?.logo_url && (
                <img src={t1Score.logo_url} alt={t1Name} width={40} height={40} className="h-10 w-10 rounded object-contain shrink-0" />
              )}
              <p className="text-lg font-bold text-foreground">{t1Name}</p>
            </div>
            <p className="text-4xl font-bold tabular-nums mt-1">{t1Score?.points || '—'}</p>
            {t1Score?.projected_points && (
              <p className="text-xs text-muted-foreground mt-1">proj. {t1Score.projected_points}</p>
            )}
          </div>
          <span className="text-xl font-bold text-muted-foreground">vs</span>
          <div className="flex-1 text-right">
            <div className="flex items-center justify-end gap-3 mb-1">
              <p className="text-lg font-bold text-foreground">{t2Name}</p>
              {t2Score?.logo_url && (
                <img src={t2Score.logo_url} alt={t2Name} width={40} height={40} className="h-10 w-10 rounded object-contain shrink-0" />
              )}
            </div>
            <p className="text-4xl font-bold tabular-nums mt-1">{t2Score?.points || '—'}</p>
            {t2Score?.projected_points && (
              <p className="text-xs text-muted-foreground mt-1">proj. {t2Score.projected_points}</p>
            )}
          </div>
        </div>
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : (
        <>
          {statLabels.length > 0 && (
            <CategoryTotalsTable
              statLabels={statLabels}
              accum1={accum1}
              accum2={accum2}
              teamValue={teamValue}
              wins={wins}
              fmt={fmt}
              t1Name={t1Name}
              t2Name={t2Name}
            />
          )}

          {roster1 && <TeamRosterTable teamName={t1Name} roster={roster1} statLabels={statLabels} />}
          {roster2 && <TeamRosterTable teamName={t2Name} roster={roster2} statLabels={statLabels} />}
        </>
      )}
    </div>
  )
}
