import { useSearchParams } from 'react-router-dom'
import type { Team } from '@/api/client'
import { DraftTab } from './DraftTab'
import { KeepersTab } from './KeepersTab'
import { NativeDraftPicksTab } from './NativeDraftPicksTab'

interface Props {
  leagueId: number
  active: boolean
  sport: string
  season: string
  teams: Team[]
  myTeam: Team | undefined
  isNative: boolean
}

/**
 * The Draft tab groups everything draft-related. Keepers are a sub-section here
 * (`?tab=draft&sub=keepers`) since keeper picks are draft picks. Draft values are
 * NFL-only, so non-NFL leagues get keepers alone with no sub-tab switcher.
 *
 * Native leagues get "Picks" instead of "Keepers" — the existing Keepers tab is
 * Yahoo-draft-result-shaped (`GetLeagueDraftResults`/`GetLeagueKeepers` both
 * 422 without a yahoo_key), the same class of bug the Players tab had before
 * `NativePlayersTab` existed. Native keeper-format leagues aren't supported
 * yet at all (see rollover's 501), so there's nothing "Keepers" could show
 * for a native league regardless of format.
 */
export function DraftSection({ leagueId, active, sport, season, teams, myTeam, isNative }: Props) {
  const [searchParams, setSearchParams] = useSearchParams()

  const hasValues = sport === 'nfl'
  const secondSub = isNative ? 'picks' : 'keepers'
  const secondLabel = isNative ? 'Picks' : 'Keepers'
  const sub = hasValues ? searchParams.get('sub') ?? 'values' : secondSub
  const setSub = (next: string) =>
    setSearchParams((prev) => { prev.set('sub', next); return prev }, { replace: true })

  return (
    <div className="space-y-4">
      {hasValues && (
        <div className="flex w-fit rounded-lg bg-muted overflow-hidden">
          {[
            { value: 'values', label: 'Draft Values' },
            { value: secondSub, label: secondLabel },
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

      {sub === 'picks' && isNative ? (
        <NativeDraftPicksTab leagueId={leagueId} active={active} teams={teams} season={season} />
      ) : sub === 'keepers' && !isNative ? (
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
