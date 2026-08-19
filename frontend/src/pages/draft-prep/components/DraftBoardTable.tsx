import { Fragment, useMemo, useState } from 'react'
import type { DraftPlayer, DraftPrepEntry, InterestLevel } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, ClickableRow, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from '@/pages/league-detail/components/TrendSparkline'
import { ThumbsUp, ThumbsDown, GripVertical } from 'lucide-react'
import { INTEREST_LEVELS, interestIconClass, interestRowClass } from '../lib/interest'
import { NoteField } from './NoteField'

// Ascending-first columns: names read A→Z, and board/rank/tier are all "1 is best."
// Everything else (points, dollars, grade, confidence…) is a magnitude where
// bigger is more interesting, so it defaults descending.
const ASC_COLS = ['board', 'rank', 'name', 'pos', 'tier']

/** Prep controls turn the read-only board into your editable one. */
export interface PrepControls {
  entry: (gsisId: string) => DraftPrepEntry
  /** Re-picking the current level clears it. */
  setInterest: (gsisId: string, level: InterestLevel) => void
  /** Sets the price you plan to pay; null takes the player out of the plan. */
  setPlannedCost: (gsisId: string, cost: number | null) => void
  setNote: (gsisId: string, note: string) => void
  /** Overrides the algorithm's tier for this player; null reverts to it. */
  setCustomTier: (gsisId: string, tier: number | null) => void
  /**
   * Moves a player next to the row above/below it. The neighbour is passed
   * explicitly rather than a direction alone: with a position filter on, the
   * visible neighbour is usually not the adjacent player in the full board, and
   * "move above the row above me" is the only reading that matches what you see.
   */
  onMove: (movingId: string, neighbourId: string, place: 'before' | 'after') => void
}

interface Props {
  players: DraftPlayer[]
  /** Grade ranks across the unfiltered pool, so the badge means the same everywhere. */
  gradeRankMap: Map<string, number>
  prep?: PrepControls
  /** Adds the consensus price and our edge over it — draft-prep only. */
  showConsensus?: boolean
}

/** Our price minus the market's. Positive = we're higher on him than the market. */
function edgeOf(p: DraftPlayer): number | null {
  return p.consensus_auction_value == null ? null : p.auction_value - p.consensus_auction_value
}

/** Position group can be a comma-separated eligibility list; tiers are per this first one. */
function primaryPos(p: DraftPlayer): string {
  return (p.position_group || p.position || '').split(',')[0]
}

/** A custom tier overrides the algorithm's own — same convention everywhere it's read. */
function effectiveTier(p: DraftPlayer, entry?: (gsisId: string) => DraftPrepEntry): number {
  const custom = entry?.(p.gsis_id).custom_tier
  return custom ?? p.tier
}

/**
 * Weight steps down as the tier does, so the top of a position reads as
 * heavier than its replacement-level tail without needing a different colour
 * per tier — there are more tiers than the palette has meanings. Shared by
 * the read-only badge and the editable field so an override doesn't suddenly
 * look different from an algorithm-assigned tier of the same number.
 */
function tierWeightClass(tier: number): string {
  if (tier <= 0) return 'text-muted-foreground/40'
  if (tier <= 2) return 'bg-primary/20 text-primary font-semibold'
  if (tier <= 4) return 'bg-muted text-foreground'
  return 'bg-muted/60 text-muted-foreground'
}

/** Tier within position, read-only. */
function TierBadge({ tier, position }: { tier: number; position: string }) {
  if (!tier) return <span className="text-muted-foreground/40">—</span>
  return (
    <span
      title={`${position.split(',')[0]} tier ${tier} — players in a tier are close enough in value to be interchangeable`}
      className={`inline-block rounded px-1.5 py-0.5 font-mono text-[11px] tabular-nums ${tierWeightClass(tier)}`}
    >
      {tier}
    </span>
  )
}

/**
 * Editable tier badge. Typing a number different from the algorithm's own
 * stores it as your override; typing the algorithm's number back clears it —
 * a value that matches the algorithm again carries no information of its
 * own, same convention the rest of the board uses for "no opinion."
 */
function TierField({
  tier, position, customTier, onCommit,
}: {
  tier: number
  position: string
  customTier: number | null
  onCommit: (next: number | null) => void
}) {
  const effective = customTier ?? tier
  const [draft, setDraft] = useState(effective ? String(effective) : '')
  // Adopt server/optimistic changes that didn't come from this field.
  const [seen, setSeen] = useState(effective)
  if (seen !== effective) {
    setSeen(effective)
    setDraft(effective ? String(effective) : '')
  }

  const commit = () => {
    const n = parseInt(draft, 10)
    if (!Number.isFinite(n) || n < 1 || n > 20) {
      setDraft(effective ? String(effective) : '')
      return
    }
    onCommit(n === tier ? null : n)
  }

  return (
    <input
      value={draft}
      onChange={(e) => setDraft(e.target.value.replace(/[^0-9]/g, '').slice(0, 2))}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === 'Enter') e.currentTarget.blur()
        if (e.key === 'Escape') setDraft(effective ? String(effective) : '')
      }}
      placeholder="—"
      title={
        customTier != null
          ? `Your tier override — the algorithm has him at ${position.split(',')[0]} tier ${tier || '—'}. Match that number to remove your override.`
          : `${position.split(',')[0]} tier ${tier || '—'} — type a number to set your own`
      }
      className={`w-7 rounded px-1 py-0.5 text-center font-mono text-[11px] tabular-nums bg-transparent focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary placeholder:text-muted-foreground/40 ${tierWeightClass(effective)}`}
    />
  )
}

/** Shows when grade rank and fantasy rank diverge significantly. */
function DeltaBadge({ gradeRank, fantasyRank }: { gradeRank: number; fantasyRank: number }) {
  const diff = fantasyRank - gradeRank // positive = grade is better than fantasy rank
  if (Math.abs(diff) < 10) return null
  if (diff > 0) {
    return <span className="ml-1 text-[10px] px-1 py-0.5 rounded bg-positive-light text-positive-foreground">UV</span>
  }
  return <span className="ml-1 text-[10px] px-1 py-0.5 rounded bg-negative-light text-negative-foreground">OV</span>
}

/**
 * Board order: your ranked players first in your order, then everyone else in
 * projection order. Exported because moving a player has to rewrite the whole
 * order, not just the rows currently on screen.
 */
export function boardOrder(players: DraftPlayer[], entry: (id: string) => DraftPrepEntry): DraftPlayer[] {
  return [...players].sort((a, b) => {
    const ra = entry(a.gsis_id).custom_rank
    const rb = entry(b.gsis_id).custom_rank
    if (ra != null && rb != null) return ra - rb
    if (ra != null) return -1
    if (rb != null) return 1
    return a.overall_rank - b.overall_rank
  })
}

export function DraftBoardTable({ players, gradeRankMap, prep, showConsensus }: Props) {
  const { sortCol, sortDir, handleSort } = useTableSort(prep ? 'board' : 'rank', 'asc', ASC_COLS)
  // Drag state lives here rather than per-row: only one row can be dragged (or
  // hovered as a drop target) at a time, and lifting it out of the row map
  // avoids re-registering handlers for every row on every hover.
  const [dragId, setDragId] = useState<string | null>(null)
  const [dragOverId, setDragOverId] = useState<string | null>(null)
  // Which half of the drop-target row the pointer is over, so the insertion
  // line can be drawn on the correct edge before the drop actually happens.
  const [dragOverPlace, setDragOverPlace] = useState<'before' | 'after' | null>(null)
  // A tier divider is its own drop target, separate from the row below it:
  // its top half joins the tier above the boundary, its bottom half joins the
  // tier below — the divider IS the boundary, so which half you're on is the
  // whole answer to "which tier."
  const [dragOverDividerId, setDragOverDividerId] = useState<string | null>(null)
  const [dragOverDividerHalf, setDragOverDividerHalf] = useState<'top' | 'bottom' | null>(null)

  const sorted = useMemo(() => {
    if (prep && sortCol === 'board') return boardOrder(players, prep.entry)
    return [...players].sort((a, b) => {
      let aVal: string | number
      let bVal: string | number

      switch (sortCol) {
        case 'rank':      aVal = a.overall_rank; bVal = b.overall_rank; break
        case 'name':      aVal = a.name; bVal = b.name; break
        case 'pos':       aVal = a.position_group; bVal = b.position_group; break
        // Tiers are per position, so this deliberately interleaves them: sorting
        // by tier answers "who is at the top of their own position?". Filter by
        // position to read one position's tiers in isolation.
        case 'tier':
          aVal = (prep?.entry(a.gsis_id).custom_tier ?? a.tier) || 99
          bVal = (prep?.entry(b.gsis_id).custom_tier ?? b.tier) || 99
          break
        case 'age':       aVal = a.age || 0; bVal = b.age || 0; break
        case 'pts':       aVal = a.proj_league_fpts; bVal = b.proj_league_fpts; break
        case 'ppg':       aVal = a.proj_league_ppg; bVal = b.proj_league_ppg; break
        case 'vor':       aVal = a.vor; bVal = b.vor; break
        case 'auction':   aVal = a.auction_value; bVal = b.auction_value; break
        case 'grade':      aVal = a.player_grade ?? -1; bVal = b.player_grade ?? -1; break
        case 'confidence': aVal = a.confidence; bVal = b.confidence; break
        // Uncovered players sort last in either direction rather than reading as $0.
        case 'cons':
          aVal = a.consensus_auction_value ?? (sortDir === 'asc' ? Infinity : -Infinity)
          bVal = b.consensus_auction_value ?? (sortDir === 'asc' ? Infinity : -Infinity)
          break
        case 'edge':
          aVal = edgeOf(a) ?? (sortDir === 'asc' ? Infinity : -Infinity)
          bVal = edgeOf(b) ?? (sortDir === 'asc' ? Infinity : -Infinity)
          break
        // Unrated players sit between like and dislike, where "no opinion" belongs.
        case 'interest':
          aVal = prep?.entry(a.gsis_id).interest ?? 0
          bVal = prep?.entry(b.gsis_id).interest ?? 0
          break
        default:          aVal = a.overall_rank; bVal = b.overall_rank; break
      }

      if (typeof aVal === 'string') {
        const cmp = aVal.localeCompare(bVal as string)
        return sortDir === 'asc' ? cmp : -cmp
      }
      return sortDir === 'asc'
        ? (aVal as number) - (bVal as number)
        : (bVal as number) - (aVal as number)
    })
  }, [players, sortCol, sortDir, prep])

  const valueTier = (v: number) => {
    if (v >= 40) return 'text-positive font-semibold'
    if (v >= 20) return 'text-positive-foreground'
    if (v >= 10) return 'text-foreground'
    return 'text-muted-foreground'
  }

  // Reordering only makes sense while the rows are in board order.
  const canMove = !!prep && sortCol === 'board' && sortDir === 'asc'

  // A divider marks where one position's tier changes from the last row of
  // that same position seen above it. That only reads coherently while the
  // list is roughly ordered by value — board order (drafting) or the
  // read-only tab's default rank order both put a position's best players
  // first. Any other sort scrambles tiers relative to position, so a "tier
  // changed" line there would be noise, not signal.
  const showTierLines = sortDir === 'asc' && (sortCol === 'board' || sortCol === 'rank')
  // Keyed by the row the divider sits above. `aboveTier` is the raw tier that
  // just ended — still needed by the drop handler, which assigns whatever
  // tier a neighbour actually has. `displayTier` is what gets printed: a
  // running per-position counter, not the raw number. A raw custom_tier
  // override (or a board reorder that leaves a position's rows out of tier
  // order) can otherwise make consecutive dividers read "Tier 2, Tier 1, Tier
  // 2" — confusing and, worse, not even unique. Counting transitions instead
  // of echoing the raw value makes every position's dividers strictly
  // 1, 2, 3, … by construction, no matter how scrambled the underlying
  // overrides are.
  const tierBoundaries = useMemo(() => {
    if (!showTierLines) return new Map<string, { aboveTier: number; displayTier: number }>()
    const lastRawByPos = new Map<string, number>()
    const displayByPos = new Map<string, number>()
    const boundaries = new Map<string, { aboveTier: number; displayTier: number }>()
    for (const p of sorted) {
      const pos = primaryPos(p)
      const tier = effectiveTier(p, prep?.entry)
      if (tier <= 0) continue
      const lastRaw = lastRawByPos.get(pos)
      if (lastRaw == null) {
        displayByPos.set(pos, 1)
      } else if (lastRaw !== tier) {
        const nextDisplay = (displayByPos.get(pos) ?? 1) + 1
        displayByPos.set(pos, nextDisplay)
        boundaries.set(p.gsis_id, { aboveTier: lastRaw, displayTier: nextDisplay })
      }
      lastRawByPos.set(pos, tier)
    }
    return boundaries
  }, [sorted, showTierLines, prep])

  return (
    // The consensus columns push this past a narrow viewport; scroll the table
    // rather than the page.
    <div className="rounded-lg bg-card overflow-x-auto">
      <Table>
        {/*
          This wrapper's overflow-x-auto unavoidably makes it a scroll container on
          both axes (CSS Overflow §3.5: an axis can't stay 'visible' once the other
          isn't), so it — not the page — becomes the containing block for the
          header's `position: sticky`. It never actually scrolls (its height just
          grows with content), so the inherited nav-height offset was pure overlap:
          the header rendered pushed down by that offset with nothing to stick to,
          landing on top of row 1. top: 0 keeps it flush with its real static
          position instead of pretending page-level stickiness works here.
        */}
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            {prep && (
              <SortableHead col="board" current={sortCol} dir={sortDir} onSort={handleSort} className="w-24 text-center">
                <HeaderTip description="Your personal draft order — ranked players come first in your order, then everyone else in projection order">
                  Board
                </HeaderTip>
              </SortableHead>
            )}
            <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort} className="w-10 text-center">
              <HeaderTip description="Overall projection rank">#</HeaderTip>
            </SortableHead>
            <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
            {prep && (
              <SortableHead col="interest" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
                <HeaderTip description="Your target/avoid flag — click a thumb to set it, click again to clear">Interest</HeaderTip>
              </SortableHead>
            )}
            {prep && (
              <TableHead className="text-center">
                <HeaderTip description="The price you're planning to pay for this player in your draft">Plan</HeaderTip>
              </TableHead>
            )}
            {prep && <TableHead className="w-40">Note</TableHead>}
            <TableHead>
              <HeaderTip description="Year-over-year trend in projected points per game, from the player's own recent seasons">Trend</HeaderTip>
            </TableHead>
            <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="Position group">Pos</HeaderTip>
            </SortableHead>
            <SortableHead col="tier" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="Players grouped by value within their position — everyone in a tier is close enough to be interchangeable">Tier</HeaderTip>
            </SortableHead>
            <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Age</SortableHead>
            <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Real-life player grade (0–100 percentile), independent of fantasy scoring">Grade</HeaderTip>
            </SortableHead>
            <SortableHead col="vor" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Value Over Replacement — projected points above the last startable player at this position">VOR</HeaderTip>
            </SortableHead>
            <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Projected fantasy points for the full season, in this league's scoring">Proj Pts</HeaderTip>
            </SortableHead>
            <SortableHead col="ppg" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Projected points per game in this league's scoring">Pts/G</HeaderTip>
            </SortableHead>
            <SortableHead col="auction" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Our auction value — this player's share of the league's budget, based on VOR">Auction $</HeaderTip>
            </SortableHead>
            {showConsensus && (
              <>
                <SortableHead col="cons" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                  <HeaderTip description="Consensus auction value — what our own price curve pays for the draft-position rank external sources give him">Cons $</HeaderTip>
                </SortableHead>
                <SortableHead col="edge" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                  <HeaderTip description="Our auction value minus the consensus value. Positive means we'd pay more than the market implies">Edge</HeaderTip>
                </SortableHead>
              </>
            )}
            <SortableHead col="confidence" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="How much the projection leans on close historical comps vs. league-average priors">Confidence</HeaderTip>
            </SortableHead>
            <TableHead>
              <HeaderTip description="How statistically unusual this player's profile is — fewer close comps means a less certain projection">Profile</HeaderTip>
            </TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {sorted.map((p: DraftPlayer, i) => {
            const mine = prep?.entry(p.gsis_id)
            return (
              <Fragment key={p.gsis_id}>
                {tierBoundaries.has(p.gsis_id) && (
                  <TableRow
                    className={[
                      'bg-muted/30 hover:bg-muted/30',
                      canMove && dragOverDividerId === p.gsis_id && dragOverDividerHalf === 'top'
                        ? 'border-t-2 border-t-primary'
                        : canMove && dragOverDividerId === p.gsis_id && dragOverDividerHalf === 'bottom'
                          ? 'border-t-2 border-t-border border-b-2 border-b-primary'
                          : 'border-t-2 border-t-border',
                    ].join(' ')}
                    onDragOver={
                      canMove
                        ? (e) => {
                            e.preventDefault()
                            const rect = e.currentTarget.getBoundingClientRect()
                            const half = e.clientY - rect.top < rect.height / 2 ? 'top' : 'bottom'
                            if (dragOverDividerId !== p.gsis_id) setDragOverDividerId(p.gsis_id)
                            setDragOverDividerHalf((cur) => (cur === half ? cur : half))
                            // The row below is a separate, overlapping drop target —
                            // don't let both light up at once.
                            if (dragOverId) setDragOverId(null)
                          }
                        : undefined
                    }
                    onDragLeave={
                      canMove
                        ? () => setDragOverDividerId((cur) => (cur === p.gsis_id ? null : cur))
                        : undefined
                    }
                    onDrop={
                      canMove
                        ? (e) => {
                            e.preventDefault()
                            const half = dragOverDividerHalf ?? 'bottom'
                            setDragOverDividerId(null)
                            setDragOverDividerHalf(null)
                            if (!dragId || dragId === p.gsis_id) { setDragId(null); return }
                            // The divider IS the boundary: its top half is the tier
                            // that just ended, its bottom half is the tier this row
                            // starts.
                            const aboveTier = tierBoundaries.get(p.gsis_id)!.aboveTier
                            const belowTier = effectiveTier(p, prep!.entry)
                            const targetTier = half === 'top' ? aboveTier : belowTier
                            const dragged = sorted.find((x) => x.gsis_id === dragId)
                            if (dragged) {
                              const next = targetTier === dragged.tier ? null : targetTier
                              if (next !== (prep!.entry(dragId).custom_tier ?? null)) prep!.setCustomTier(dragId, next)
                            }
                            prep!.onMove(dragId, p.gsis_id, 'before')
                            setDragId(null)
                          }
                        : undefined
                    }
                  >
                    <TableCell
                      colSpan={100}
                      className="py-1 font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground"
                    >
                      Tier {tierBoundaries.get(p.gsis_id)!.displayTier}
                    </TableCell>
                  </TableRow>
                )}
              <ClickableRow
                href={`/players/${p.gsis_id}`}
                // A rated player is marked on his own row rather than repeated in a
                // panel: an accent edge whose weight tracks how strongly you feel.
                // Dragging gets two static (no-transition) cues layered on top:
                // the lifted row dims, and the row underneath the pointer gets an
                // accent line on whichever edge it'll actually land on.
                className={[
                  interestRowClass(mine?.interest ?? null),
                  canMove && dragId === p.gsis_id ? 'opacity-40' : '',
                  canMove && dragOverId === p.gsis_id && dragOverPlace === 'before' ? 'border-t-2 border-primary' : '',
                  canMove && dragOverId === p.gsis_id && dragOverPlace === 'after' ? 'border-b-2 border-primary' : '',
                ].filter(Boolean).join(' ')}
                onDragOver={
                  canMove
                    ? (e) => {
                        e.preventDefault()
                        const rect = e.currentTarget.getBoundingClientRect()
                        const place = e.clientY - rect.top < rect.height / 2 ? 'before' : 'after'
                        if (dragOverId !== p.gsis_id) setDragOverId(p.gsis_id)
                        setDragOverPlace((cur) => (cur === place ? cur : place))
                        // The divider above this row is a separate, overlapping
                        // drop target — don't let both light up at once.
                        if (dragOverDividerId) setDragOverDividerId(null)
                      }
                    : undefined
                }
                onDragLeave={
                  canMove
                    ? () => setDragOverId((cur) => (cur === p.gsis_id ? null : cur))
                    : undefined
                }
                onDrop={
                  canMove
                    ? (e) => {
                        e.preventDefault()
                        const place = dragOverPlace ?? 'before'
                        setDragOverId(null)
                        setDragOverPlace(null)
                        if (!dragId || dragId === p.gsis_id) { setDragId(null); return }

                        // Re-tier the dropped player from whichever same-position
                        // neighbours land beside it: if both sides agree, the drop
                        // is squarely inside that tier; if they differ (a tier
                        // boundary), `place` — the same half-of-the-row signal that
                        // already drives the insertion line — breaks the tie, so
                        // dragging a little higher or lower changes which tier wins.
                        const dragged = sorted.find((x) => x.gsis_id === dragId)
                        if (dragged) {
                          const pos = primaryPos(dragged)
                          const aboveStart = place === 'before' ? i - 1 : i
                          const belowStart = place === 'before' ? i : i + 1
                          let above: number | null = null
                          for (let j = aboveStart; j >= 0; j--) {
                            if (sorted[j].gsis_id === dragId) continue
                            const t = effectiveTier(sorted[j], prep!.entry)
                            if (primaryPos(sorted[j]) === pos && t > 0) { above = t; break }
                          }
                          let below: number | null = null
                          for (let j = belowStart; j < sorted.length; j++) {
                            if (sorted[j].gsis_id === dragId) continue
                            const t = effectiveTier(sorted[j], prep!.entry)
                            if (primaryPos(sorted[j]) === pos && t > 0) { below = t; break }
                          }
                          const autoTier =
                            above != null && below != null
                              ? (above === below ? above : (place === 'before' ? above : below))
                              : above ?? below
                          if (autoTier != null) {
                            const next = autoTier === dragged.tier ? null : autoTier
                            if (next !== (prep!.entry(dragId).custom_tier ?? null)) prep!.setCustomTier(dragId, next)
                          }
                        }

                        prep!.onMove(dragId, p.gsis_id, place)
                        setDragId(null)
                      }
                    : undefined
                }
              >
                {prep && (
                  <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-center gap-1">
                      {canMove && (
                        <span
                          draggable
                          onDragStart={(e) => { e.dataTransfer.effectAllowed = 'move'; setDragId(p.gsis_id) }}
                          onDragEnd={() => {
                            setDragId(null)
                            setDragOverId(null)
                            setDragOverPlace(null)
                            setDragOverDividerId(null)
                            setDragOverDividerHalf(null)
                          }}
                          aria-label={`Drag to reorder ${p.name}`}
                          title="Drag to reorder"
                          className="cursor-grab text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
                        >
                          <GripVertical className="h-3.5 w-3.5" />
                        </span>
                      )}
                      <span className="font-mono tabular-nums text-xs text-foreground w-5 text-right">
                        {mine?.custom_rank ?? i + 1}
                      </span>
                      {canMove && (
                        <span className="flex flex-col leading-none">
                          <button
                            onClick={() => prep.onMove(p.gsis_id, sorted[i - 1].gsis_id, 'before')}
                            disabled={i === 0}
                            aria-label={`Move ${p.name} up`}
                            className="font-mono text-[9px] text-muted-foreground hover:text-foreground disabled:opacity-30"
                          >
                            ▲
                          </button>
                          <button
                            onClick={() => prep.onMove(p.gsis_id, sorted[i + 1].gsis_id, 'after')}
                            disabled={i === sorted.length - 1}
                            aria-label={`Move ${p.name} down`}
                            className="font-mono text-[9px] text-muted-foreground hover:text-foreground disabled:opacity-30"
                          >
                            ▼
                          </button>
                        </span>
                      )}
                    </div>
                  </TableCell>
                )}
                <TableCell className="text-center text-muted-foreground font-mono tabular-nums">
                  {p.overall_rank}
                </TableCell>
                <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} linked />
                {prep && (
                  <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
                    {/* Target or avoid — one thumb each way, re-click to clear. */}
                    <div className="flex items-center justify-center gap-1.5">
                      {INTEREST_LEVELS.map(({ level, label }) => {
                        const on = mine?.interest === level
                        const Icon = level > 0 ? ThumbsUp : ThumbsDown
                        return (
                          <button
                            key={level}
                            onClick={() => prep.setInterest(p.gsis_id, level)}
                            title={label}
                            aria-label={`${label} — ${p.name}`}
                            aria-pressed={on}
                            className="flex h-5 w-5 items-center justify-center rounded hover:bg-muted"
                          >
                            <Icon className={`h-3.5 w-3.5 ${interestIconClass(level, on)}`} />
                          </button>
                        )
                      })}
                    </div>
                  </TableCell>
                )}
                {prep && (
                  <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
                    {mine?.planned_cost == null ? (
                      <button
                        onClick={() => prep.setPlannedCost(p.gsis_id, p.auction_value)}
                        title={`Add to your team at $${p.auction_value}`}
                        className="h-5 w-5 rounded bg-muted font-display text-[11px] font-semibold text-muted-foreground hover:text-foreground"
                      >
                        +
                      </button>
                    ) : (
                      <span className="inline-flex items-center gap-1">
                        <span className="font-mono text-xs tabular-nums text-primary">
                          ${mine.planned_cost}
                        </span>
                        <button
                          onClick={() => prep.setPlannedCost(p.gsis_id, null)}
                          title="Remove from your team"
                          className="font-mono text-[10px] text-muted-foreground hover:text-foreground"
                        >
                          ×
                        </button>
                      </span>
                    )}
                  </TableCell>
                )}
                {prep && (
                  <TableCell className="w-40" onClick={(e) => e.stopPropagation()}>
                    <NoteField
                      value={mine?.note ?? ''}
                      onCommit={(next) => prep.setNote(p.gsis_id, next)}
                    />
                  </TableCell>
                )}
                <TableCell>
                  <TrendSparkline points={p.trajectory ?? []} />
                </TableCell>
                <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
                <TableCell className="text-center" onClick={(e) => prep && e.stopPropagation()}>
                  {prep ? (
                    <TierField
                      tier={p.tier}
                      position={p.position_group}
                      customTier={mine?.custom_tier ?? null}
                      onCommit={(next) => prep.setCustomTier(p.gsis_id, next)}
                    />
                  ) : (
                    <TierBadge tier={p.tier} position={p.position_group} />
                  )}
                </TableCell>
                <TableCell className="text-center text-muted-foreground font-mono tabular-nums">{p.age || '—'}</TableCell>
                <TableCell className="text-right tabular-nums font-mono">
                  {p.player_grade != null ? (
                    <>
                      <span className={gradeColorClass(p.player_grade)}>{p.player_grade.toFixed(0)}</span>
                      {gradeRankMap.has(p.gsis_id) && (
                        <DeltaBadge gradeRank={gradeRankMap.get(p.gsis_id)!} fantasyRank={p.overall_rank} />
                      )}
                    </>
                  ) : (
                    <span className="text-muted-foreground/40">—</span>
                  )}
                </TableCell>
                <TableCell className="text-right tabular-nums font-mono">
                  {p.vor.toFixed(1)}
                </TableCell>
                <TableCell className="text-right tabular-nums font-mono">
                  {p.proj_league_fpts.toFixed(1)}
                </TableCell>
                <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                  {p.proj_league_ppg.toFixed(1)}
                </TableCell>
                <TableCell className={`text-right tabular-nums font-mono ${valueTier(p.auction_value)}`}>
                  ${p.auction_value}
                </TableCell>
                {showConsensus && (
                  <>
                    <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                      {p.consensus_auction_value == null ? (
                        <span className="text-muted-foreground/40" title="No external source covers this player">—</span>
                      ) : (
                        <span
                          title={
                            `${p.consensus_derived
                              ? `Our price for the ${p.position_group}${p.consensus_position_rank?.toFixed(0)} slot — the market's rank for him`
                              : 'Median imported auction price'} · ${p.consensus_sources} source${p.consensus_sources === 1 ? '' : 's'}`
                          }
                        >
                          ${p.consensus_auction_value}
                          {p.consensus_sources === 1 && <span className="text-muted-foreground/60">*</span>}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-right tabular-nums font-mono">
                      {(() => {
                        const edge = edgeOf(p)
                        if (edge == null) return <span className="text-muted-foreground/40">—</span>
                        // Sign carries the meaning; colour is the secondary cue.
                        const tone =
                          Math.abs(edge) < 5 ? 'text-muted-foreground'
                          : edge > 0 ? 'text-positive'
                          : 'text-secondary'
                        return (
                          <span
                            className={tone}
                            title={
                              edge > 0
                                ? `We value him $${edge} above the market — a bargain if we're right.`
                                : edge < 0
                                  ? `The market pays $${-edge} more than we would.`
                                  : 'We and the market agree on price.'
                            }
                          >
                            {edge > 0 ? '+' : edge < 0 ? '−' : ''}${Math.abs(edge)}
                          </span>
                        )
                      })()}
                    </TableCell>
                  </>
                )}
                <TableCell className="text-center">
                  <ConfidenceBadge value={p.confidence} />
                </TableCell>
                <TableCell>
                  <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} />
                </TableCell>
              </ClickableRow>
              </Fragment>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
