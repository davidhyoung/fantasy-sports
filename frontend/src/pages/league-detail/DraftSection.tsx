import { useSearchParams } from 'react-router-dom'
import type { Team } from '@/api/client'
import { DraftTab } from './DraftTab'
import { KeepersTab } from './KeepersTab'

interface Props {
  leagueId: number
  active: boolean
  sport: string
  season: string
  teams: Team[]
  myTeam: Team | undefined
}

/**
 * The Draft tab groups everything draft-related. Keepers are a sub-section here
 * (`?tab=draft&sub=keepers`) since keeper picks are draft picks. Draft values are
 * NFL-only, so non-NFL leagues get keepers alone with no sub-tab switcher.
 */
export function DraftSection({ leagueId, active, sport, season, teams, myTeam }: Props) {
  const [searchParams, setSearchParams] = useSearchParams()

  const hasValues = sport === 'nfl'
  const sub = hasValues ? searchParams.get('sub') ?? 'values' : 'keepers'
  const setSub = (next: string) =>
    setSearchParams((prev) => { prev.set('sub', next); return prev }, { replace: true })

  return (
    <div className="space-y-4">
      {hasValues && (
        <div className="flex w-fit rounded-lg bg-muted overflow-hidden">
          {[
            { value: 'values', label: 'Draft Values' },
            { value: 'keepers', label: 'Keepers' },
          ].map((s) => (
            <button
              key={s.value}
              onClick={() => setSub(s.value)}
              className={`px-3 py-1.5 font-display text-xs font-semibold ${
                sub === s.value
                  ? 'bg-foreground text-background'
                  : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      )}

      {sub === 'keepers' ? (
        <KeepersTab
          leagueId={leagueId}
          active={active}
          teams={teams}
          myTeam={myTeam}
          season={season}
        />
      ) : (
        <DraftTab leagueId={leagueId} active={active} season={season} />
      )}
    </div>
  )
}
