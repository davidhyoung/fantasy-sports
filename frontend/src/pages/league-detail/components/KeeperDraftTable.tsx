import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { PlayerAvatar, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
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
    <>
      {/* Card list below md — face: Player, Keeper $, Status; expansion: Team,
          Draft $, Yrs Kept, Auction $, and the wishlist Keep? control — the
          same isMyTeamPick permission check as desktop gates the edit controls. */}
      <div className="space-y-2 md:hidden">
        {visiblePicks.map((p) => {
          const inWishlist = wishlistMap.has(p.player_key)
          const entry = wishlistMap.get(p.player_key)
          const isDesignated = designatedSet.has(p.player_key)
          const isMyTeamPick = p.team_key === myTeamYahooKey
          const av = auctionByName.get(p.player_name)
          const surplus = av != null && !p.not_keepable ? av - p.keeper_cost : 0

          const expanded: MobileStatField[] = []
          if (showTeamColumn) expanded.push({ label: 'Team', value: p.owner_team_name || p.team_key })
          expanded.push({
            label: 'Draft $',
            value: p.undrafted ? <><span className="text-muted-foreground">—</span> <Badge variant="neutral">FA</Badge></> : `$${p.draft_cost}`,
          })
          expanded.push({
            label: 'Yrs Kept',
            value: isMyTeamPick && inWishlist ? (
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
                className="h-11 w-16 text-sm px-2"
              />
            ) : (p.years_kept > 0 ? p.years_kept : '—'),
          })
          expanded.push({
            label: 'Auction $',
            value: av == null ? '—' : (
              <>
                ${av}
                {surplus !== 0 && (
                  <span className={surplus > 0 ? 'text-positive-foreground' : 'text-destructive'}>
                    {' '}{surplus > 0 ? '+' : ''}{surplus}
                  </span>
                )}
              </>
            ),
          })
          if (showWishlistColumn && isMyTeamPick) {
            expanded.push({
              label: 'Keep?',
              value: (
                <label className="flex h-11 items-center gap-2">
                  <input
                    type="checkbox"
                    aria-label={`Keep ${p.player_name}`}
                    checked={inWishlist}
                    disabled={p.not_keepable || isMutating || (!inWishlist && wishlistCount >= MAX_KEEPERS)}
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
                    className="h-5 w-5 cursor-pointer"
                  />
                </label>
              ),
            })
          }

          return (
            <MobileStatCard
              key={`${p.player_key}-${p.team_key}`}
              leading={<PlayerAvatar src={p.image_url} alt={p.player_name} />}
              title={p.player_name}
              subtitle={p.stats && p.stats.length > 0 ? (
                <span className="flex flex-wrap gap-x-2">
                  {p.stats.map((s) => {
                    const rp = rankByPlayer.get(p.player_key)
                    const catScore = rp?.category_scores.find((c) => c.label === s.label)
                    return (
                      <span key={s.label} className="whitespace-nowrap">
                        <span className="opacity-60">{s.label}</span> {s.value}
                        {catScore && (
                          <span className={`ml-0.5 text-[10px] ${zScoreColor(catScore.z_score)}`}>
                            {zScoreIndicator(catScore.z_score) || '●'}
                          </span>
                        )}
                      </span>
                    )
                  })}
                </span>
              ) : undefined}
              face={
                <div className="text-right">
                  <div className="font-mono text-xs tabular-nums">
                    {p.not_keepable ? <span className="text-negative-foreground text-[11px]">⚠ Max</span> : `$${p.keeper_cost}`}
                  </div>
                  {isDesignated && <Badge variant="pink" className="mt-0.5">Designated</Badge>}
                </div>
              }
              expanded={expanded}
            />
          )
        })}
      </div>

      <div className="hidden md:block rounded-lg bg-card">
      <Table>
        <TableHeader>
          <HeaderRow>
            {showTeamColumn && <TableHead>Team</TableHead>}
            <TableHead>Player</TableHead>
            <TableHead className="text-right"><HeaderTip description="What this player cost at the original draft">Draft $</HeaderTip></TableHead>
            <TableHead className="text-right"><HeaderTip description="Number of consecutive seasons this player has been kept">Yrs Kept</HeaderTip></TableHead>
            <TableHead className="text-right"><HeaderTip description="What it would cost to keep this player next season, per your league's keeper rules">Keeper $</HeaderTip></TableHead>
            <TableHead className="text-right"><HeaderTip description="Current-season auction value, for comparison against the keeper cost">Auction $</HeaderTip></TableHead>
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

                <TableCell className="text-right font-mono tabular-nums text-sm">
                  {p.undrafted ? (
                    <span className="text-muted-foreground">
                      — <Badge variant="neutral">FA</Badge>
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
                    <span className="font-mono tabular-nums text-sm text-muted-foreground">
                      {p.years_kept > 0 ? p.years_kept : '—'}
                    </span>
                  )}
                </TableCell>

                <TableCell className="text-right font-mono tabular-nums text-sm font-medium">
                  {p.not_keepable ? (
                    <span className="text-negative-foreground text-xs">⚠ Max</span>
                  ) : (
                    `$${p.keeper_cost}`
                  )}
                </TableCell>

                <TableCell className="text-right font-mono tabular-nums text-sm">
                  {(() => {
                    const av = auctionByName.get(p.player_name)
                    if (av == null) return <span className="text-muted-foreground">—</span>
                    const surplus = !p.not_keepable ? av - p.keeper_cost : 0
                    return (
                      <div>
                        <span className="font-medium">${av}</span>
                        {surplus !== 0 && (
                          <div className={`text-xs ${surplus > 0 ? 'text-positive-foreground' : 'text-destructive'}`}>
                            {surplus > 0 ? '+' : ''}{surplus}
                          </div>
                        )}
                      </div>
                    )
                  })()}
                </TableCell>

                <TableCell>
                  {isDesignated && (
                    <Badge variant="pink">
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
    </>
  )
}
