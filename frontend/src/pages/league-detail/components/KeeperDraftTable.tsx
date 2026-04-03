import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { PlayerAvatar, HeaderRow } from '@/components/ui/table-helpers'
import { zScoreIndicator, zScoreColor } from '@/lib/utils'
import type { DraftPick, KeeperWishlistEntry, RankedPlayer } from '../../../api/client'
import type { WishlistPayload } from '../hooks/useKeepers'
import { MAX_KEEPERS } from '../hooks/useKeepers'

interface Props {
  visiblePicks: DraftPick[]
  showTeamColumn: boolean
  showWishlistColumn: boolean
  myTeamYahooKey: string | undefined
  wishlistMap: Map<string, KeeperWishlistEntry>
  designatedSet: Set<string>
  wishlistCount: number
  isMutating: boolean
  rankByPlayer: Map<string, RankedPlayer>
  auctionByName: Map<string, number>
  onAdd: (playerKey: string, payload: WishlistPayload) => void
  onRemove: (playerKey: string) => void
}

/** Table of draft picks enriched with keeper costs, wishlist checkboxes, and season stats. */
export function KeeperDraftTable({
  visiblePicks,
  showTeamColumn,
  showWishlistColumn,
  myTeamYahooKey,
  wishlistMap,
  designatedSet,
  wishlistCount,
  isMutating,
  rankByPlayer,
  auctionByName,
  onAdd,
  onRemove,
}: Props) {
  return (
    <div className="rounded-lg bg-card">
      <Table>
        <TableHeader>
          <HeaderRow>
            {showTeamColumn && <TableHead>Team</TableHead>}
            <TableHead>Player</TableHead>
            <TableHead className="text-right">Draft $</TableHead>
            <TableHead className="text-right">Yrs Kept</TableHead>
            <TableHead className="text-right">Keeper $</TableHead>
            <TableHead className="text-right">Auction $</TableHead>
            <TableHead>Status</TableHead>
            {showWishlistColumn && <TableHead className="text-center">Keep?</TableHead>}
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {visiblePicks.map((p) => {
            const inWishlist = wishlistMap.has(p.player_key)
            const entry = wishlistMap.get(p.player_key)
            const isDesignated = designatedSet.has(p.player_key)
            const isMyTeamPick = p.team_key === myTeamYahooKey

            return (
              <TableRow key={`${p.player_key}-${p.team_key}`}>
                {showTeamColumn && (
                  <TableCell className="text-xs text-muted-foreground max-w-[140px] truncate">
                    {p.owner_team_name || p.team_key}
                  </TableCell>
                )}

                <TableCell className="font-medium text-sm">
                  <div className="flex items-center gap-2">
                    <PlayerAvatar src={p.image_url} alt={p.player_name} />
                    <div>
                      <div>{p.player_name}</div>
                      {p.stats && p.stats.length > 0 && (
                        <div className="flex flex-wrap gap-x-2 gap-y-0.5 mt-0.5">
                          {p.stats.map((s) => {
                            const rp = rankByPlayer.get(p.player_key)
                            const catScore = rp?.category_scores.find((c) => c.label === s.label)
                            return (
                              <span key={s.label} className="text-xs whitespace-nowrap rounded px-0.5">
                                <span className="opacity-60">{s.label}</span>{' '}
                                <span className="text-foreground/80 font-medium">{s.value}</span>
                                {catScore && (
                                  <span className={`ml-0.5 text-[10px] ${zScoreColor(catScore.z_score)}`}>{zScoreIndicator(catScore.z_score) || '●'}</span>
                                )}
                              </span>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  </div>
                </TableCell>

                <TableCell className="text-right tabular-nums text-sm">
                  {p.undrafted ? (
                    <span className="text-muted-foreground">
                      — <Badge className="text-xs bg-muted text-muted-foreground border-border">FA</Badge>
                    </span>
                  ) : (
                    `$${p.draft_cost}`
                  )}
                </TableCell>

                <TableCell className="text-right">
                  {isMyTeamPick && inWishlist ? (
                    <Input
                      key={`${p.player_key}-${entry?.years_kept}`}
                      type="number"
                      min={1}
                      defaultValue={entry?.years_kept ?? 1}
                      onBlur={(e) => {
                        const years = Math.max(1, Number(e.target.value))
                        onAdd(p.player_key, {
                          player_name: p.player_name,
                          position: p.position,
                          draft_cost: p.undrafted ? null : p.draft_cost,
                          years_kept: years,
                        })
                      }}
                      className="w-16 h-7 text-sm px-2 text-right"
                    />
                  ) : (
                    <span className="tabular-nums text-sm text-muted-foreground">
                      {p.years_kept > 0 ? p.years_kept : '—'}
                    </span>
                  )}
                </TableCell>

                <TableCell className="text-right tabular-nums text-sm font-medium">
                  {p.not_keepable ? (
                    <span className="text-orange-400 text-xs">⚠ Max</span>
                  ) : (
                    `$${p.keeper_cost}`
                  )}
                </TableCell>

                <TableCell className="text-right tabular-nums text-sm">
                  {(() => {
                    const av = auctionByName.get(p.player_name)
                    if (av == null) return <span className="text-muted-foreground">—</span>
                    const surplus = !p.not_keepable ? av - p.keeper_cost : 0
                    return (
                      <div>
                        <span className="font-medium">${av}</span>
                        {surplus !== 0 && (
                          <div className={`text-xs ${surplus > 0 ? 'text-green-600 dark:text-green-400' : 'text-red-600 dark:text-red-400'}`}>
                            {surplus > 0 ? '+' : ''}{surplus}
                          </div>
                        )}
                      </div>
                    )
                  })()}
                </TableCell>

                <TableCell>
                  {isDesignated && (
                    <Badge className="bg-purple-100 text-purple-700 border-purple-300 dark:bg-purple-900/40 dark:text-purple-300 dark:border-purple-700 text-xs">
                      Designated
                    </Badge>
                  )}
                </TableCell>

                {showWishlistColumn && (
                  <TableCell className="text-center">
                    {isMyTeamPick && (
                      <input
                        type="checkbox"
                        aria-label={`Keep ${p.player_name}`}
                        checked={inWishlist}
                        disabled={
                          p.not_keepable ||
                          isMutating ||
                          (!inWishlist && wishlistCount >= MAX_KEEPERS)
                        }
                        onChange={(e) => {
                          if (e.target.checked) {
                            onAdd(p.player_key, {
                              player_name: p.player_name,
                              position: p.position,
                              draft_cost: p.undrafted ? null : p.draft_cost,
                              years_kept: 1,
                            })
                          } else {
                            onRemove(p.player_key)
                          }
                        }}
                        className="cursor-pointer"
                      />
                    )}
                  </TableCell>
                )}
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
