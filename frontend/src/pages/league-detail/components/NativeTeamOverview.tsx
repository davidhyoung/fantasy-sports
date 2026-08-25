import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import { TeamAvatar } from '@/components/ui/table-helpers'
import { getLeagueRosters, getLeagueScoreboard, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { NativeRosterTable } from './NativeRosterTable'
import { EditContractForm } from './EditContractForm'
import { PlayerDetailPanel } from '@/pages/player-detail/PlayerDetailPanel'

interface Props {
  leagueId: number
  teamId: number
  /** League's configured slot counts — lets the roster table show the full
   *  lineup shape (empty spots included), not just whoever's rostered. */
  slots?: Record<string, number>
  /** The signed-in user's own claimed team in this league, if any — passed
   *  through to the player-detail panel's action bar (see its own doc). */
  myTeamId?: number
  /** Opens the league's trade builder pre-seeded around a specific player;
   *  passed through to the player-detail panel's Trade/Trade for button. */
  onTradeFor?: (playerTeamId: number, gsisId: string) => void
}

/**
 * The current week's matchup card plus roster for one native-league team —
 * used by the Roster tab for whichever team is selected. There's no
 * separate read-only team page: a native league has no reader who isn't
 * the commissioner (single-user model), so this is always fully editable.
 */
export function NativeTeamOverview({ leagueId, teamId, slots, myTeamId, onTradeFor }: Props) {
  const [editing, setEditing] = useState<RosterEntry | null>(null)
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)

  // isLoading (not isFetching) — isFetching is also true during the
  // background refetch every roster mutation triggers, which used to
  // unmount the whole table behind a spinner on every single move. isLoading
  // is only true before the first successful fetch, so a reorder/drop/etc.
  // just swaps the updated rows in once the refetch resolves.
  const { data: rosters, isLoading: loadingRosters } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
  })
  const { data: scoreboard } = useQuery({
    queryKey: keys.scoreboard(leagueId),
    queryFn: () => getLeagueScoreboard(leagueId),
  })

  const roster = (rosters ?? []).filter((r) => r.team_id === teamId)
  const matchup = scoreboard?.matchups.find((m) => m.teams.some((t) => t.team_id === teamId))
  const me = matchup?.teams.find((t) => t.team_id === teamId)
  const opponent = matchup?.teams.find((t) => t.team_id !== teamId)

  return (
    <>
      {matchup && me && opponent ? (
        <div className="mb-6 rounded-lg bg-card px-4 py-3">
          <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Week {matchup.week} {matchup.status === 'postevent' ? '· Final' : '· Not yet scored'}
          </div>
          <div className="mt-1.5 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <TeamAvatar name={me.name} />
              <span className="font-display text-sm font-semibold text-foreground">{me.name}</span>
            </div>
            <span className="font-mono text-lg font-bold tabular-nums text-foreground">
              {me.points} <span className="text-muted-foreground">–</span> {opponent.points}
            </span>
            <div className="flex items-center gap-2">
              {opponent.team_id ? (
                <RouterLink
                  to={`/leagues/${leagueId}?tab=roster&team=${opponent.team_id}`}
                  className="font-display text-sm font-semibold text-foreground hover:text-primary"
                >
                  {opponent.name}
                </RouterLink>
              ) : (
                <span className="font-display text-sm font-semibold text-foreground">{opponent.name}</span>
              )}
              <TeamAvatar name={opponent.name} />
            </div>
          </div>
        </div>
      ) : (
        <p className="mb-6 text-sm text-muted-foreground">
          No matchup this week — either no schedule has been generated yet, or this team has a bye.
        </p>
      )}

      {loadingRosters ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : (
        <NativeRosterTable
          leagueId={leagueId}
          roster={roster}
          slots={slots}
          onEdit={setEditing}
          onPlayerClick={setViewingPlayer}
        />
      )}

      <ResponsiveDialog open={editing != null} onClose={() => setEditing(null)} title="Edit contract">
        {editing && <EditContractForm leagueId={leagueId} entry={editing} onClose={() => setEditing(null)} />}
      </ResponsiveDialog>

      <PlayerDetailPanel
        gsisId={viewingPlayer}
        leagueId={leagueId}
        onClose={() => setViewingPlayer(null)}
        myTeamId={myTeamId}
        onTradeFor={onTradeFor}
      />
    </>
  )
}
