import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { SelectControl } from '@/components/ui/filter-chip'
import { TeamAvatar } from '@/components/ui/table-helpers'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import { Badge } from '@/components/ui/badge'
import {
  getLeagueSettings, updateLeagueTeam, rolloverLeague,
  getLeagueTransactions, getTeamCap, type Team,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PlayerAssignForm } from './components/PlayerAssignForm'
import { TradeBuilder, type TradeInitialSelection } from './components/TradeBuilder'
import { NativeTeamOverview } from './components/NativeTeamOverview'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  myTeam: Team | undefined
  format: string
}

/**
 * The native league's Roster tab — one page for both "my team" and "every
 * team," since they were the same content twice, and also the destination
 * for clicking any team name anywhere in the app (Standings, Scoreboard,
 * Draft Picks, a teammate's matchup opponent) — there's no separate
 * "full team page," this is it. A team switcher (defaults to your claimed
 * team) picks who you're looking at; the commissioner tools
 * (assign/trade/rollover/claim/activity log) apply regardless of which team
 * is selected. The matchup card + roster for the selected team comes from
 * the shared NativeTeamOverview.
 *
 * The selected team lives in the `team` URL param, not component state —
 * every other page links here with `?tab=roster&team=<id>` directly, so
 * deriving from the URL is what makes those links actually land on the
 * right team, and it sidesteps the class of bug a `useState` initializer
 * has here: `teams` loads asynchronously, so a value computed once at
 * mount can freeze on a default before real teams (or the URL) are known.
 */
export function NativeRosterTab({ leagueId, active, teams, myTeam, format }: Props) {
  const qc = useQueryClient()
  const [searchParams, setSearchParams] = useSearchParams()
  const teamParam = Number(searchParams.get('team'))
  const teamId = teams.some((t) => t.id === teamParam) ? teamParam : (myTeam?.id ?? teams[0]?.id ?? 0)
  const selectTeam = (id: number) =>
    setSearchParams((prev) => { prev.set('team', String(id)); return prev }, { replace: true })
  const [assigning, setAssigning] = useState(false)
  const [trading, setTrading] = useState(false)
  // Set when the trade dialog is opened from a player's detail panel
  // (Trade/Trade for) instead of the plain "Trade" button, so the builder
  // opens pre-seeded around that player. Cleared on close so a later plain
  // "Trade" click doesn't inherit a stale seed.
  const [tradeSeed, setTradeSeed] = useState<TradeInitialSelection | undefined>(undefined)
  const [confirmRollover, setConfirmRollover] = useState(false)

  const { data: settings } = useQuery({
    queryKey: keys.leagueSettings(leagueId),
    queryFn: () => getLeagueSettings(leagueId),
    enabled: active,
  })
  const { data: transactions = [] } = useQuery({
    queryKey: keys.leagueTransactions(leagueId),
    queryFn: () => getLeagueTransactions(leagueId),
    enabled: active,
  })
  const { data: capResp } = useQuery({
    queryKey: keys.teamCap(leagueId, teamId),
    queryFn: () => getTeamCap(leagueId, teamId, 4),
    enabled: active && teamId > 0,
  })

  const claimMutation = useMutation({
    mutationFn: (id: number) => updateLeagueTeam(leagueId, id, { claim: true }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.leagueTeams(leagueId) }),
  })

  const rolloverMutation = useMutation({
    mutationFn: () => rolloverLeague(leagueId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.league(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leaguePicks(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'team'] })
      setConfirmRollover(false)
    },
  })

  const cap = capResp?.breakdowns[0]
  // Seasons after the current one that already carry a real cap hit worth
  // flagging (dead money from a cut, or a multi-year deal ending) — mostly
  // empty in a league's early days, since Phase 8's cap-space banking is
  // what makes every future season interesting on its own.
  const futureCommitments = (capResp?.breakdowns ?? []).slice(1).filter((b) => b.spend > 0)

  const teamName = useMemo(() => {
    const m = new Map(teams.map((t) => [t.id, t.name]))
    return (id: number) => m.get(id) ?? `Team ${id}`
  }, [teams])

  const summarize = (t: (typeof transactions)[number]) => {
    if (t.kind === 'trade') {
      const moves = (t.payload.moves as { asset: string; from_team_id: number; to_team_id: number }[] | undefined) ?? []
      const teamsInvolved = new Set(moves.flatMap((m) => [m.from_team_id, m.to_team_id]))
      return `Trade between ${[...teamsInvolved].map(teamName).join(' & ')} — ${moves.length} asset${moves.length === 1 ? '' : 's'}`
    }
    if (t.kind === 'rollover') {
      const to = t.payload.to_season as number | undefined
      return to ? `Season rolled over to ${to}` : 'Season rollover'
    }
    if (t.kind === 'draft') return 'Draft pick used'
    return t.kind
  }

  // Opens the trade builder pre-seeded around one player, from the
  // player-detail panel's Trade/Trade for action — `playerTeamId` is
  // whichever team currently rosters them. Only ever called when `myTeam`
  // is set (the panel doesn't render the button otherwise).
  const openTradeFor = (playerTeamId: number, gsisId: string) => {
    if (!myTeam) return
    const assetKey = `player:${gsisId}`
    setTradeSeed(
      playerTeamId === myTeam.id
        ? { teamAId: myTeam.id, side: 'A', assetKey }
        : { teamAId: myTeam.id, teamBId: playerTeamId, side: 'B', assetKey }
    )
    setTrading(true)
  }

  const closeTrade = () => {
    setTrading(false)
    setTradeSeed(undefined)
  }

  if (!active) return null

  const selectedTeam = teams.find((t) => t.id === teamId)

  return (
    <>
      <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
        <div className="flex items-center gap-2">
          {selectedTeam && <TeamAvatar name={selectedTeam.name} />}
          <SelectControl value={teamId} onChange={(e) => selectTeam(Number(e.target.value))}>
            {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
          </SelectControl>
        </div>
        <div className="flex items-center gap-2">
          {selectedTeam && (
            selectedTeam.user_id ? (
              <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Your team
              </span>
            ) : (
              <Button
                variant="text" size="bare"
                onClick={() => claimMutation.mutate(teamId)}
                disabled={claimMutation.isPending}
              >
                Claim as my team
              </Button>
            )
          )}
          <Button variant="outline" size="sm" onClick={() => { setTradeSeed(undefined); setTrading(true) }}>Trade</Button>
          <Button size="sm" onClick={() => setAssigning(true)}>+ Assign player</Button>
        </div>
      </div>

      {cap && (
        <div className="mb-4 rounded-lg bg-card px-4 py-3">
          <div className="flex flex-wrap gap-4">
            <div>
              <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Cap</div>
              <div className="font-mono text-sm tabular-nums text-foreground">
                ${cap.cap}
                {cap.banked !== 0 && <span className="text-muted-foreground"> ({cap.banked > 0 ? '+' : ''}{cap.banked} banked)</span>}
              </div>
            </div>
            <div>
              <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Active spend</div>
              <div className="font-mono text-sm tabular-nums text-foreground">${cap.active_spend}</div>
            </div>
            {cap.dead_money > 0 && (
              <div>
                <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Dead money</div>
                <div className="font-mono text-sm tabular-nums text-negative">${cap.dead_money}</div>
              </div>
            )}
            <div>
              <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Available</div>
              <div className={`font-mono text-sm tabular-nums ${cap.available < 0 ? 'text-negative' : 'text-foreground'}`}>${cap.available}</div>
            </div>
            <div>
              <div className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Roster</div>
              <div className={`font-mono text-sm tabular-nums ${cap.roster_count > cap.roster_max ? 'text-negative' : 'text-foreground'}`}>
                {cap.roster_count}/{cap.roster_max}
              </div>
            </div>
          </div>
          {futureCommitments.length > 0 && (
            <div className="mt-2.5 flex flex-wrap gap-x-4 gap-y-1 border-t border-border pt-2.5 text-xs text-muted-foreground">
              <span className="font-display font-semibold uppercase tracking-wide">Looking ahead</span>
              {futureCommitments.map((b) => (
                <span key={b.season} className="font-mono tabular-nums">
                  {b.season}: ${b.spend} committed
                  {b.dead_money > 0 && <span className="text-negative"> (${b.dead_money} dead)</span>}
                </span>
              ))}
            </div>
          )}
        </div>
      )}

      <NativeTeamOverview
        leagueId={leagueId}
        teamId={teamId}
        slots={settings?.slots}
        myTeamId={myTeam?.id}
        onTradeFor={openTradeFor}
      />

      {/* Single-user model: whoever's signed in manages every team, and the
          backend's requireCommissioner already gates the actual mutation —
          native league teams don't reliably carry a user_id yet (only
          claimed teams do), so this section shows unconditionally rather
          than hiding behind a client-side ownership check that would often
          be wrong. */}
      <div className="mt-6 flex items-center justify-between rounded-lg border border-border px-4 py-3">
        <div>
          <div className="font-display text-sm font-semibold text-foreground">Season rollover</div>
          <p className="text-xs text-muted-foreground">
            {format === 'dynasty'
              ? 'Contracts carry forward, expiring deals release to free agency, a new rookie draft class is generated.'
              : format === 'redraft'
              ? 'Every roster releases to free agency and a fresh draft class is generated.'
              : 'Rollover is not implemented for keeper-format native leagues yet.'}
          </p>
        </div>
        <Button
          variant="outline"
          disabled={format === 'keeper'}
          onClick={() => setConfirmRollover(true)}
        >
          Roll over season
        </Button>
      </div>

      {transactions.length > 0 && (
        <div className="mt-6">
          <div className="mb-2 font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Activity</div>
          <div className="flex flex-col gap-1.5">
            {transactions.slice(0, 10).map((t, i) => (
              <div key={i} className="flex items-center gap-2 text-xs">
                <Badge variant="neutral">{t.kind}</Badge>
                <span className="text-muted-foreground">{summarize(t)}</span>
                <span className="ml-auto font-mono text-muted-foreground">{new Date(t.created_at).toLocaleString()}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      <ResponsiveDialog open={assigning} onClose={() => setAssigning(false)} title="Assign player">
        <PlayerAssignForm leagueId={leagueId} teams={teams} mode="assign" defaultTeamId={teamId} onClose={() => setAssigning(false)} />
      </ResponsiveDialog>

      <ResponsiveDialog open={trading} onClose={closeTrade} title="Trade">
        <TradeBuilder leagueId={leagueId} teams={teams} onClose={closeTrade} initialSelection={tradeSeed} />
      </ResponsiveDialog>

      <ResponsiveDialog open={confirmRollover} onClose={() => setConfirmRollover(false)} title="Roll over season">
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            {format === 'dynasty'
              ? 'Every contract that reaches its final year releases to free agency; everyone else carries forward with years_used incremented. A new rookie draft class is generated for next season. This cannot be undone.'
              : 'Every rostered player releases to free agency and a fresh draft class is generated for next season. This cannot be undone.'}
          </p>
          {rolloverMutation.error && <p className="text-sm text-destructive">{(rolloverMutation.error as Error).message}</p>}
          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={() => setConfirmRollover(false)}>Cancel</Button>
            <Button variant="destructive" onClick={() => rolloverMutation.mutate()} disabled={rolloverMutation.isPending}>
              {rolloverMutation.isPending ? 'Rolling over…' : 'Roll over'}
            </Button>
          </div>
        </div>
      </ResponsiveDialog>
    </>
  )
}
