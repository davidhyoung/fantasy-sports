import { SidePanel } from '@/components/ui/side-panel'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import PlayerDetailBody from './PlayerDetailBody'

interface PlayerDetailPanelProps {
  /** The player to show, or null when closed. Both shells key off this
   *  rather than a separate `open` flag, since there's never a meaningful
   *  "open with no player" state. */
  gsisId: string | null
  leagueId?: number
  onClose: () => void
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
 */
export function PlayerDetailPanel({ gsisId, leagueId, onClose }: PlayerDetailPanelProps) {
  const open = gsisId != null
  const body = gsisId ? <PlayerDetailBody key={gsisId} gsisId={gsisId} leagueId={leagueId} /> : null

  return (
    <>
      <div className="hidden md:contents">
        <SidePanel open={open} onClose={onClose}>{body}</SidePanel>
      </div>
      <div className="contents md:hidden">
        <MobileSheet open={open} onClose={onClose}>{body}</MobileSheet>
      </div>
    </>
  )
}
