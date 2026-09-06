import { useEffect, useMemo, useState } from 'react'
import type { DraftPlayer, DraftPrepEntry, InterestLevel } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, ClickableRow, HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from '@/pages/league-detail/components/TrendSparkline'
import { INTEREST_LEVELS, interestIconClass, interestRowClass } from '../lib/interest'
import { NoteField } from './NoteField'
import { MobileDraftBoard } from './MobileDraftBoard'

// Ascending-first columns: names read A→Z, and board/rank are both "1 is best."
// Everything else (points, dollars, grade, confidence…) is a magnitude where
// bigger is more interesting, so it defaults descending.
const ASC_COLS = ['board', 'rank', 'name', 'pos']

/** Every column behind the "Columns ▾" toggle — hidden by default so the
 *  board reads as a decision surface, not a data dump. `#` moved here from
 *  the always-on set; it stays sortable, just not on-screen by default. */
export type AdvancedCol = 'rank' | 'note' | 'age' | 'vor' | 'pts' | 'ppg' | 'cons' | 'edge' | 'confidence' | 'profile'
const ADVANCED_COLS: { col: AdvancedCol; label: string; consensusOnly?: boolean }[] = [
  { col: 'rank', label: '#' },
  { col: 'note', label: 'Note' },
  { col: 'age', label: 'Age' },
  { col: 'vor', label: 'VOR' },
  { col: 'pts', label: 'Proj Pts' },
  { col: 'ppg', label: 'Pts/G' },
  { col: 'cons', label: 'Cons $', consensusOnly: true },
  { col: 'edge', label: 'Edge', consensusOnly: true },
  { col: 'confidence', label: 'Confidence' },
  { col: 'profile', label: 'Profile' },
]
const COLUMNS_KEY = 'fs.draft-prep.columns'

/** Labels for every sortable column, used by the "sorted by X" reordering notice. */
const COLUMN_LABELS: Record<string, string> = {
  board: 'Board', rank: '#', name: 'Player', interest: 'Interest', pos: 'Pos', age: 'Age',
  grade: 'Grade', vor: 'VOR', pts: 'Proj Pts', ppg: 'Pts/G', auction: 'Auction $',
  cons: 'Cons $', edge: 'Edge', confidence: 'Confidence',
}

function readVisibleAdvanced(): Partial<Record<AdvancedCol, boolean>> {
  try {
    const raw = localStorage.getItem(COLUMNS_KEY)
    return raw ? JSON.parse(raw) : {}
  } catch {
    return {}
  }
}

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
  /** Your own valuation for this player, distinct from the algorithm's auction_value. */
  setMyValue: (gsisId: string, value: number | null) => void
  /** Sets several fields in one write — see useDraftPrep's setFields for why
   *  this exists instead of two independent single-field calls. */
  setFields: (gsisId: string, changes: Partial<{ customRank: number | null; customTier: number | null; myValue: number | null; myValueSource: 'user' | 'derived' | null }>) => void
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
  /** Caps the printed sheet to this many players by board order (see BoardPrintSheet). */
  printPoolSize?: number
  /** Players whose interest was just cleared while a Targets/Avoids filter was
   *  active. The caller (index.tsx) keeps them in `players` for this render
   *  pass rather than letting them vanish; this renders them muted with an
   *  Undo control instead. Maps id -> the interest level that was cleared. */
  recentlyCleared?: Map<string, InterestLevel>
  onUndoInterest?: (gsisId: string) => void
  /** Opens the player in-place (a drawer/sheet) rather than navigating to
   *  `/players/:gsisId` — every caller owns its own drawer state and passes
   *  this down, same convention as NativeRosterTable's onPlayerClick. */
  onPlayerClick: (gsisId: string) => void
}

/** Our price minus the market's. Positive = we're higher on him than the market. */
export function edgeOf(p: DraftPlayer): number | null {
  return p.consensus_auction_value == null ? null : p.auction_value - p.consensus_auction_value
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

/** Rank + delta vs. the projection's own rank — the divergence is the actual
 *  output of a draft board, so it rides along with the number rather than
 *  living in a separate `#` column you'd have to cross-reference. */
function RankDelta({ customRank, overallRank }: { customRank: number; overallRank: number }) {
  const diff = overallRank - customRank // positive = you have him higher than the projection
  if (Math.abs(diff) < 1) return null
  const n = Math.abs(diff)
  return (
    <span
      className="font-mono text-[10px] text-muted-foreground"
      title={`${n} spot${n === 1 ? '' : 's'} ${diff > 0 ? 'above' : 'below'} the projection's rank`}
    >
      {diff > 0 ? '▲' : '▼'}{n}
    </span>
  )
}

/** Weight on the neutral ramp, not the accent hue — pink already means "you
 *  did this / act here" (interest, planned cost, active sort), so a dollar
 *  magnitude scale needs a different channel. */
function valueTier(v: number): string {
  if (v >= 40) return 'text-foreground font-semibold'
  if (v >= 20) return 'text-foreground'
  if (v >= 10) return 'text-muted-foreground'
  return 'text-muted-foreground/60'
}

/** Six-field print sheet — rank, player, pos, my value, planned cost, note —
 *  the artefact you actually built, not the working table. Two columns per
 *  page, capped by board order. */
function BoardPrintSheet({ players, entry, cap }: { players: DraftPlayer[]; entry: (id: string) => DraftPrepEntry; cap: number }) {
  const ordered = boardOrder(players, entry).slice(0, cap > 0 ? cap : 200)
  return (
    <div className="hidden print:grid print:grid-cols-2 print:gap-x-6">
      {ordered.map((p, i) => {
        const mine = entry(p.gsis_id)
        return (
          <div key={p.gsis_id} className="print:break-inside-avoid flex items-baseline gap-2 border-b border-border py-0.5 text-[10px]">
            <span className="w-6 shrink-0 font-mono">{mine.custom_rank ?? i + 1}</span>
            <span className="min-w-0 flex-1 truncate font-semibold">{p.name}</span>
            <span className="w-7 shrink-0 text-muted-foreground">{p.position_group}</span>
            <span className="w-10 shrink-0 font-mono">{mine.my_value != null ? `$${mine.my_value}` : '—'}</span>
            <span className="w-10 shrink-0 font-mono">{mine.planned_cost != null ? `$${mine.planned_cost}` : '—'}</span>
            <span className="max-w-[7rem] shrink truncate text-muted-foreground">{mine.note || ''}</span>
          </div>
        )
      })}
    </div>
  )
}

export function DraftBoardTable({ players, gradeRankMap, prep, showConsensus, printPoolSize, recentlyCleared, onUndoInterest, onPlayerClick }: Props) {
  const { sortCol, sortDir, handleSort } = useTableSort(prep ? 'board' : 'rank', 'asc', ASC_COLS)
  // Drag state lives here rather than per-row: only one row can be dragged (or
  // hovered as a drop target) at a time, and lifting it out of the row map
  // avoids re-registering handlers for every row on every hover.
  const [dragId, setDragId] = useState<string | null>(null)
  const [dragOverId, setDragOverId] = useState<string | null>(null)
  // Which half of the drop-target row the pointer is over, so the insertion
  // line can be drawn on the correct edge before the drop actually happens.
  const [dragOverPlace, setDragOverPlace] = useState<'before' | 'after' | null>(null)

  const [visibleAdvanced, setVisibleAdvanced] = useState<Partial<Record<AdvancedCol, boolean>>>(readVisibleAdvanced)
  const [columnsOpen, setColumnsOpen] = useState(false)
  const toggleColumn = (col: AdvancedCol) => {
    setVisibleAdvanced((prev) => {
      const next = { ...prev, [col]: !prev[col] }
      try { localStorage.setItem(COLUMNS_KEY, JSON.stringify(next)) } catch { /* storage unavailable */ }
      return next
    })
  }
  const isVisible = (col: AdvancedCol) => !!visibleAdvanced[col] && (col !== 'cons' && col !== 'edge' ? true : !!showConsensus)

  // A hidden column can't stay the active sort — fall back to board order (or
  // plain rank when there's no board to sort by).
  useEffect(() => {
    const advanced = ADVANCED_COLS.find((c) => c.col === sortCol)
    if (advanced && !isVisible(advanced.col)) {
      handleSort(prep ? 'board' : 'rank')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visibleAdvanced, showConsensus])

  const sorted = useMemo(() => {
    if (prep && sortCol === 'board') return boardOrder(players, prep.entry)
    return [...players].sort((a, b) => {
      let aVal: string | number
      let bVal: string | number

      switch (sortCol) {
        case 'rank':      aVal = a.overall_rank; bVal = b.overall_rank; break
        case 'name':      aVal = a.name; bVal = b.name; break
        case 'pos':       aVal = a.position_group; bVal = b.position_group; break
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

  // Reordering only makes sense while the rows are in board order.
  const canMove = !!prep && sortCol === 'board' && sortDir === 'asc'
  const sortNotice = prep && !canMove ? (
    <p className="text-xs text-muted-foreground">
      Sorted by {COLUMN_LABELS[sortCol] ?? sortCol} —{' '}
      <button
        onClick={() => handleSort('board')}
        className="text-primary underline underline-offset-2 hover:text-foreground"
      >
        back to board order to reorder
      </button>
    </p>
  ) : null

  const columnsControl = (
    <div className="relative hidden md:block">
      <button
        onClick={() => setColumnsOpen((o) => !o)}
        className="rounded-lg border border-border px-3 py-2 font-display text-sm font-semibold text-muted-foreground hover:text-foreground hover:border-muted-foreground"
      >
        Columns ▾
      </button>
      {columnsOpen && (
        <>
          <div className="fixed inset-0 z-40" onClick={() => setColumnsOpen(false)} aria-hidden="true" />
          <div className="absolute right-0 top-full z-50 mt-1 w-44 rounded-md border border-border bg-card p-1.5">
            {ADVANCED_COLS.filter((c) => !c.consensusOnly || showConsensus).map((c) => (
              <label key={c.col} className="flex items-center gap-2 rounded px-1.5 py-1 text-xs text-foreground hover:bg-muted">
                <input
                  type="checkbox"
                  checked={isVisible(c.col)}
                  onChange={() => toggleColumn(c.col)}
                  className="h-3.5 w-3.5"
                />
                {c.label}
              </label>
            ))}
          </div>
        </>
      )}
    </div>
  )

  return (
    <>
      <div className="hidden items-center justify-between gap-2 md:flex print:hidden">
        <div>{sortNotice}</div>
        {columnsControl}
      </div>
      <MobileDraftBoard
        players={sorted}
        gradeRankMap={gradeRankMap}
        prep={prep}
        showConsensus={showConsensus}
        sortCol={sortCol}
        sortDir={sortDir}
        onSort={handleSort}
        canMove={canMove}
        sortNotice={prep && !canMove ? `Sorted by ${COLUMN_LABELS[sortCol] ?? sortCol}` : null}
        visibleAdvanced={visibleAdvanced}
        isVisible={isVisible}
        recentlyCleared={recentlyCleared}
        onUndoInterest={onUndoInterest}
        onPlayerClick={onPlayerClick}
      />
    {prep && <BoardPrintSheet players={players} entry={prep.entry} cap={printPoolSize ?? 0} />}
    {/* The consensus columns push this past a narrow viewport; scroll the table
        rather than the page. Hidden below md — MobileDraftBoard replaces it there. */}
    <div className="hidden md:block rounded-lg bg-card overflow-x-auto print:hidden">
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
            {isVisible('rank') && (
              <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort} className="w-10 text-center">
                <HeaderTip description="Overall projection rank">#</HeaderTip>
              </SortableHead>
            )}
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
            {prep && isVisible('note') && <TableHead className="w-40">Note</TableHead>}
            <TableHead>
              <HeaderTip description="Year-over-year trend in projected points per game, from the player's own recent seasons">Trend</HeaderTip>
            </TableHead>
            <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
              <HeaderTip description="Position group">Pos</HeaderTip>
            </SortableHead>
            {isVisible('age') && (
              <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Age</SortableHead>
            )}
            <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Real-life player grade (0–100 percentile), independent of fantasy scoring">Grade</HeaderTip>
            </SortableHead>
            {isVisible('vor') && (
              <SortableHead col="vor" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                <HeaderTip description="Value Over Replacement — projected points above the last startable player at this position">VOR</HeaderTip>
              </SortableHead>
            )}
            {isVisible('pts') && (
              <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                <HeaderTip description="Projected fantasy points for the full season, in this league's scoring">Proj Pts</HeaderTip>
              </SortableHead>
            )}
            {isVisible('ppg') && (
              <SortableHead col="ppg" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                <HeaderTip description="Projected points per game in this league's scoring">Pts/G</HeaderTip>
              </SortableHead>
            )}
            <SortableHead col="auction" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
              <HeaderTip description="Our auction value — this player's share of the league's budget, based on VOR">Auction $</HeaderTip>
            </SortableHead>
            {showConsensus && isVisible('cons') && (
              <SortableHead col="cons" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                <HeaderTip description="Consensus auction value — what our own price curve pays for the draft-position rank external sources give him">Cons $</HeaderTip>
              </SortableHead>
            )}
            {showConsensus && isVisible('edge') && (
              <SortableHead col="edge" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                <HeaderTip description="Our auction value minus the consensus value. Positive means we'd pay more than the market implies">Edge</HeaderTip>
              </SortableHead>
            )}
            {isVisible('confidence') && (
              <SortableHead col="confidence" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
                <HeaderTip description="How much the projection leans on close historical comps vs. league-average priors">Confidence</HeaderTip>
              </SortableHead>
            )}
            {isVisible('profile') && (
              <TableHead>
                <HeaderTip description="How statistically unusual this player's profile is — fewer close comps means a less certain projection">Profile</HeaderTip>
              </TableHead>
            )}
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {sorted.map((p: DraftPlayer, i) => {
            const mine = prep?.entry(p.gsis_id)
            const clearedLevel = mine?.interest == null ? recentlyCleared?.get(p.gsis_id) : undefined
            return (
              <ClickableRow
                key={p.gsis_id}
                // A rated player is marked on his own row rather than repeated in a
                // panel: an accent edge whose weight tracks how strongly you feel.
                // Dragging gets two static (no-transition) cues layered on top:
                // the lifted row dims, and the row underneath the pointer gets an
                // accent line on whichever edge it'll actually land on.
                className={[
                  interestRowClass(mine?.interest ?? null),
                  clearedLevel != null ? 'opacity-50' : '',
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
                          }}
                          aria-label={`Drag to reorder ${p.name}`}
                          title="Drag to reorder"
                          className="cursor-grab font-mono text-xs text-muted-foreground/50 hover:text-muted-foreground active:cursor-grabbing"
                        >
                          ⠿
                        </span>
                      )}
                      <span className="font-mono tabular-nums text-xs text-foreground w-5 text-right">
                        {mine?.custom_rank ?? i + 1}
                      </span>
                      {mine?.custom_rank != null && (
                        <RankDelta customRank={mine.custom_rank} overallRank={p.overall_rank} />
                      )}
                      {canMove && (
                        // Rendered for keyboard reordering but visually hidden until
                        // focused — the grip handle is the always-on affordance, this
                        // is the focus-visible fallback for it, not a third always-on control.
                        <span className="flex flex-col leading-none opacity-0 focus-within:opacity-100">
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
                {isVisible('rank') && (
                  <TableCell className="text-center text-muted-foreground font-mono tabular-nums">
                    {p.overall_rank}
                  </TableCell>
                )}
                <PlayerCell
                  name={p.name}
                  imageUrl={p.headshot_url}
                  sub={p.team}
                  onClick={() => onPlayerClick(p.gsis_id)}
                  notes={p.notes}
                  strikethrough={clearedLevel != null}
                />
                {prep && (
                  <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
                    {clearedLevel != null ? (
                      <button
                        onClick={() => onUndoInterest?.(p.gsis_id)}
                        className="font-mono text-[10px] text-primary underline underline-offset-2 hover:text-foreground"
                      >
                        Undo
                      </button>
                    ) : (
                      // Target or avoid — one glyph each way, re-click to clear.
                      <div className="flex items-center justify-center gap-1.5">
                        {INTEREST_LEVELS.map(({ level, label }) => {
                          const on = mine?.interest === level
                          return (
                            <button
                              key={level}
                              onClick={() => prep.setInterest(p.gsis_id, level)}
                              title={label}
                              aria-label={`${label} — ${p.name}`}
                              aria-pressed={on}
                              className="flex h-5 w-5 items-center justify-center rounded font-mono text-sm hover:bg-muted"
                            >
                              <span className={interestIconClass(level, on)}>{level > 0 ? '△' : '▽'}</span>
                            </button>
                          )
                        })}
                      </div>
                    )}
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
                {prep && isVisible('note') && (
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
                {isVisible('age') && (
                  <TableCell className="text-center text-muted-foreground font-mono tabular-nums">{p.age || '—'}</TableCell>
                )}
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
                {isVisible('vor') && (
                  <TableCell className="text-right tabular-nums font-mono">
                    {p.vor.toFixed(1)}
                  </TableCell>
                )}
                {isVisible('pts') && (
                  <TableCell className="text-right tabular-nums font-mono">
                    {p.proj_league_fpts.toFixed(1)}
                  </TableCell>
                )}
                {isVisible('ppg') && (
                  <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                    {p.proj_league_ppg.toFixed(1)}
                  </TableCell>
                )}
                <TableCell className={`text-right tabular-nums font-mono ${valueTier(p.auction_value)}`}>
                  ${p.auction_value}
                </TableCell>
                {showConsensus && isVisible('cons') && (
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
                )}
                {showConsensus && isVisible('edge') && (
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
                )}
                {isVisible('confidence') && (
                  <TableCell className="text-center">
                    <ConfidenceBadge value={p.confidence} />
                  </TableCell>
                )}
                {isVisible('profile') && (
                  <TableCell>
                    <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} />
                  </TableCell>
                )}
              </ClickableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
    </>
  )
}
