import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Dialog } from '@/components/ui/dialog'
import { getLeagueRosters, getLeagueScoreboard, type Team, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { NativeRosterTable } from './components/NativeRosterTable'
import { EditContractForm } from './components/EditContractForm'

interface Props {
  leagueId: number
  active: boolean
  myTeam: Team
}

/**
 * Native-league My Team tab — your matchup for the current week (if a
 * schedule has been generated and this week isn't your bye) plus your
 * roster, with the same inline-editable Slot dropdown as the commissioner
 * Roster tab (that IS the lineup-setting mechanism — see NativeRosterTable).
 * A separate component from the commissioner Roster tab, not a lock-to-team
 * mode of it: this is meant to feel like "my team", not "the league viewed
 * through one team's eyes" (no team switcher, no trade/assign/rollover
 * controls — those stay on the Roster tab).
 */
export function NativeMyTeamTab({ leagueId, active, myTeam }: Props) {
  const [editing, setEditing] = useState<RosterEntry | null>(null)

  const { data: rosters, isFetching: loadingRosters } = useQuery({
    queryKey: keys.leagueRosters(leagueId),
    queryFn: () => getLeagueRosters(leagueId),
    enabled: active,
  })
  const { data: scoreboard } = useQuery({
    queryKey: keys.scoreboard(leagueId),
    queryFn: () => getLeagueScoreboard(leagueId),
    enabled: active,
  })

  if (!active) return null

  const myRoster = (rosters ?? []).filter((r) => r.team_id === myTeam.id)
  const matchup = scoreboard?.matchups.find((m) => m.teams.some((t) => t.team_id === myTeam.id))
  const me = matchup?.teams.find((t) => t.team_id === myTeam.id)
  const opponent = matchup?.teams.find((t) => t.team_id !== myTeam.id)

  return (
    <>
      <h2 className="mb-3 font-display text-lg font-bold text-foreground">{myTeam.name}</h2>

      {matchup && me && opponent ? (
        <div className="mb-6 rounded-lg bg-card px-4 py-3">
          <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Week {matchup.week} {matchup.status === 'postevent' ? '· Final' : '· Not yet scored'}
          </div>
          <div className="mt-1.5 flex items-center justify-between">
            <span className="font-display text-sm font-semibold text-foreground">{me.name}</span>
            <span className="font-mono text-lg font-bold tabular-nums text-foreground">
              {me.points} <span className="text-muted-foreground">–</span> {opponent.points}
            </span>
            <span className="font-display text-sm font-semibold text-foreground">{opponent.name}</span>
          </div>
        </div>
      ) : (
        <p className="mb-6 text-sm text-muted-foreground">
          No matchup this week — either no schedule has been generated yet, or your team has a bye.
        </p>
      )}

      {loadingRosters ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : (
        <NativeRosterTable leagueId={leagueId} roster={myRoster} onEdit={setEditing} />
      )}

      <Dialog open={editing != null} onClose={() => setEditing(null)} title="Edit contract">
        {editing && <EditContractForm leagueId={leagueId} entry={editing} onClose={() => setEditing(null)} />}
      </Dialog>
    </>
  )
}
