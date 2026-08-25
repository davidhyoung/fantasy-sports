import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { SidePanel } from '@/components/ui/side-panel'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import { Button } from '@/components/ui/button'
import { getNFLPlayer, dropLeagueRoster } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { EditContractForm } from '@/pages/league-detail/components/EditContractForm'
import PlayerDetailBody from './PlayerDetailBody'

interface PlayerDetailPanelProps {
  /** The player to show, or null when closed. Both shells key off this
   *  rather than a separate `open` flag, since there's never a meaningful
   *  "open with no player" state. */
  gsisId: string | null
  leagueId?: number
  onClose: () => void
  /** The signed-in user's own claimed team in this league, if any — decides
   *  whether the fixed action bar's trade button reads "Trade" (the player
   *  is already on this team) or "Trade for" (on someone else's), and gates
   *  the trade action entirely when no team is claimed — there's no "my
   *  side" to seed the builder with otherwise. */
  myTeamId?: number
  /** Opens the league's trade builder pre-seeded around this player.
   *  `playerTeamId` is whichever team currently rosters them. Only rendered
   *  (and so only ever called) when `myTeamId` is set. */
  onTradeFor?: (playerTeamId: number, gsisId: string) => void
}

/**
 * In-place player detail — a right-docked SidePanel on desktop, a MobileSheet
 * on mobile — used where a click on a player's name should keep the caller's
 * table/page in view rather than navigating to `/players/:gsisId` (the full
 * page still exists for direct links/other entry points; this is an
 * alternate presentation of the same `PlayerDetailBody` content). Both shells
 * always mount (each a no-op while `gsisId` is null); the inactive one is
 * `display:none`'d via the wrapping div, same convention as ResponsiveDialog.
 *
 * `key={gsisId}` on the body remounts it on every player switch, so the
 * query and the projection-format toggle reset cleanly instead of carrying
 * over from whichever player was open before.
 *
 * The fixed action bar (Edit contract / Drop / Trade) lives outside the
 * body's scroll area, in the shell's `footer` slot, so it stays reachable on
 * a long profile without scrolling. It needs the same contract data the body
 * fetches for its own Contract card — rather than plumbing that up via a
 * callback, this runs an identical `useQuery` (same key as PlayerDetailBody's)
 * so TanStack Query dedupes it to the same cached request instead of a
 * second fetch.
 */
export function PlayerDetailPanel({ gsisId, leagueId, onClose, myTeamId, onTradeFor }: PlayerDetailPanelProps) {
  const qc = useQueryClient()
  const open = gsisId != null
  const [editing, setEditing] = useState(false)
  const [confirmDrop, setConfirmDrop] = useState(false)

  const { data } = useQuery({
    queryKey: keys.nflPlayer(gsisId ?? '', leagueId),
    queryFn: () => getNFLPlayer(gsisId ?? '', leagueId),
    enabled: !!gsisId,
    staleTime: 5 * 60 * 1000,
  })

  const dropMutation = useMutation({
    mutationFn: () => dropLeagueRoster(leagueId!, gsisId!),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId!) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
      qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId!) })
      setConfirmDrop(false)
      onClose()
    },
  })

  const handleClose = () => {
    setEditing(false)
    setConfirmDrop(false)
    onClose()
  }

  const contract = data?.contract
  // Rostered in *this* league (not just any league) — a player's contract
  // is scoped to one league's roster, same rule the Contract card already
  // follows.
  const canManage = !!(leagueId && gsisId && contract)
  const isMyTeam = canManage && myTeamId != null && contract!.team_id === myTeamId

  const footer = canManage ? (
    confirmDrop ? (
      <div className="flex items-center gap-2">
        <span className="mr-auto text-xs text-muted-foreground">Drop {data!.player.name}?</span>
        <Button variant="outline" size="sm" onClick={() => setConfirmDrop(false)}>Cancel</Button>
        <Button variant="destructive" size="sm" onClick={() => dropMutation.mutate()} disabled={dropMutation.isPending}>
          {dropMutation.isPending ? 'Dropping…' : 'Confirm drop'}
        </Button>
      </div>
    ) : (
      <div className="flex items-center gap-2">
        <Button variant="outline" size="sm" onClick={() => setEditing(true)}>Edit contract</Button>
        <Button variant="outline" size="sm" onClick={() => setConfirmDrop(true)}>Drop</Button>
        {myTeamId != null && onTradeFor && (
          <Button size="sm" className="ml-auto" onClick={() => onTradeFor(contract!.team_id, gsisId!)}>
            {isMyTeam ? 'Trade' : 'Trade for'}
          </Button>
        )}
      </div>
    )
  ) : undefined

  const body = gsisId ? <PlayerDetailBody key={gsisId} gsisId={gsisId} leagueId={leagueId} /> : null

  return (
    <>
      <div className="hidden md:contents">
        <SidePanel open={open} onClose={handleClose} footer={footer}>{body}</SidePanel>
      </div>
      <div className="contents md:hidden">
        <MobileSheet open={open} onClose={handleClose} footer={footer}>{body}</MobileSheet>
      </div>

      {canManage && (
        <ResponsiveDialog open={editing} onClose={() => setEditing(false)} title="Edit contract">
          <EditContractForm
            leagueId={leagueId!}
            entry={{ gsis_id: gsisId!, salary: contract!.salary, years_total: contract!.years_total }}
            onClose={() => {
              qc.invalidateQueries({ queryKey: keys.nflPlayer(gsisId!, leagueId) })
              setEditing(false)
            }}
          />
        </ResponsiveDialog>
      )}
    </>
  )
}
