import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { getDraftValues, DraftPlayer, DraftReplacementLevel } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PROJECTION_SEASON } from '@/lib/constants'
import { DraftBoardTable } from '@/pages/draft-prep/components/DraftBoardTable'
import { draftQuery, readSettings } from './hooks/useDraftSettings'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K']
const NO_PLAYERS: DraftPlayer[] = []

interface DraftTabProps {
  leagueId: number
  active: boolean
  season: string
}

/**
 * The league's draft board, read-only. Settings live on /draft-prep — this view
 * follows whatever was saved there, so the two never disagree, but editing them
 * (and the personal board: ranks, targets, sleepers) belongs to the prep page.
 */
export function DraftTab({ leagueId, active, season }: DraftTabProps) {
  const [position, setPosition] = useState('')

  // Draft prep is for the season AFTER the completed league year — e.g. a 2025
  // Yahoo league → draft for the 2026 NFL season. Clamped to PROJECTION_SEASON,
  // since a league already sitting on the upcoming season (the mock dev league is
  // seeded at 2026) would otherwise ask for a season we have no projections for.
  const seasonNum = Math.min((parseInt(season, 10) || 2025) + 1, PROJECTION_SEASON)

  // Settings saved on the prep page, if any — read once per league, not edited here.
  const saved = useMemo(() => readSettings(leagueId), [leagueId])
  const { params, key } = draftQuery(seasonNum, saved)

  const { data, isLoading, isError } = useQuery({
    queryKey: keys.draftValues(leagueId, seasonNum, key),
    queryFn: () => getDraftValues(leagueId, params),
    enabled: active,
    staleTime: 5 * 60 * 1000,
  })

  // Seasons with no projections yet come back with an empty (historically null) list.
  const allPlayers = data?.players ?? NO_PLAYERS
  const replacementLevels = data?.replacement_levels ?? []

  // Grade ranks across ALL players (not filtered), so the badge means the same thing
  // whichever position filter is on.
  const gradeRankMap = useMemo(() => {
    const map = new Map<string, number>()
    const withGrade = allPlayers
      .filter((p) => p.player_grade != null)
      .sort((a, b) => (b.player_grade ?? 0) - (a.player_grade ?? 0))
    withGrade.forEach((p, i) => map.set(p.gsis_id, i + 1))
    return map
  }, [allPlayers])

  const filtered = allPlayers.filter(
    (p) => !position || p.position_group === position || p.position === position,
  )

  return (
    <div className="space-y-4">
      {/* Header row + replacement levels */}
      <div className="flex flex-col lg:flex-row lg:items-start lg:justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-foreground">{seasonNum} Draft Rankings</h2>
          <p className="text-sm text-muted-foreground">
            League-specific projections · comp-based · auction values ·{' '}
            <RouterLink to="/draft-prep" className="text-primary hover:underline">
              Draft Prep →
            </RouterLink>
          </p>
        </div>
        {replacementLevels.length > 0 && (
          <div className="flex flex-wrap gap-3 text-xs text-muted-foreground bg-card rounded-lg px-3 py-2 lg:max-w-[50%]">
            <span className="font-medium text-foreground">Replacement levels</span>
            {/* From the response, not any local state — this describes the board on screen. */}
            <span className="text-muted-foreground">({data?.num_teams} teams, ${data?.budget_per_team}/team)</span>
            {replacementLevels
              .filter((rl: DraftReplacementLevel) => !position || rl.position === position)
              .map((rl: DraftReplacementLevel) => (
                <span key={rl.position}>
                  <span className="font-medium text-foreground">{rl.position}</span>{' '}
                  {rl.points.toFixed(0)} pts (top {rl.threshold})
                </span>
              ))}
          </div>
        )}
      </div>

      {/* Position filter */}
      <div className="flex gap-1 flex-wrap">
        {POSITIONS.map((pos) => (
          <button
            key={pos}
            onClick={() => setPosition(pos === 'All' ? '' : pos)}
            className={`px-3 py-1 rounded text-sm font-medium border ${
              (pos === 'All' && position === '') || pos === position
                ? 'bg-primary/20 text-primary border-primary/50'
                : 'text-muted-foreground border-border hover:text-foreground hover:border-muted-foreground'
            }`}
          >
            {pos}
          </button>
        ))}
      </div>

      {/* Table */}
      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading draft values...</p>
      ) : isError ? (
        <p className="text-destructive text-sm">Failed to load draft values. Make sure your league is synced.</p>
      ) : data && allPlayers.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No {seasonNum} projections available yet. Run{' '}
          <code className="font-mono text-xs">make project-nfl ARGS="-project -season {seasonNum}"</code> to generate them.
        </p>
      ) : data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {filtered.length} player{filtered.length !== 1 ? 's' : ''}
            {position ? ` (${position})` : ''} ·{' '}
            {data.scoring_format === 'league' ? 'league scoring' : `${data.scoring_format.toUpperCase()} scoring`}
            {data.settings?.overridden && ' · your Draft Prep settings'}
          </p>
          <DraftBoardTable players={filtered} gradeRankMap={gradeRankMap} />
        </>
      ) : null}
    </div>
  )
}
