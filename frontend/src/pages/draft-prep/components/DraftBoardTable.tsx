import { useMemo } from 'react'
import type { DraftPlayer, DraftPrepEntry, InterestLevel } from '@/api/client'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { SortableHead, useTableSort, PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from '@/pages/league-detail/components/TrendSparkline'
import { ThumbsUp, ThumbsDown } from 'lucide-react'
import { INTEREST_LEVELS, SCALE_ORDER, isFilled, interestIconClass } from '../lib/interest'

const STRING_COLS = ['name', 'pos']

/** Prep controls turn the read-only board into your editable one. */
export interface PrepControls {
  entry: (gsisId: string) => DraftPrepEntry
  /** Re-picking the current level clears it. */
  setInterest: (gsisId: string, level: InterestLevel) => void
  /** Sets the price you plan to pay; null takes the player out of the plan. */
  setPlannedCost: (gsisId: string, cost: number | null) => void
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
  const { sortCol, sortDir, handleSort } = useTableSort(prep ? 'board' : 'rank', 'asc', STRING_COLS)

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
        case 'ppr':       aVal = a.proj_fpts_ppr_pg; bVal = b.proj_fpts_ppr_pg; break
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

  return (
    // The consensus columns push this past a narrow viewport; scroll the table
    // rather than the page.
    <div className="rounded-lg bg-card overflow-x-auto">
      <Table>
        <TableHeader>
          <HeaderRow>
            {prep && (
              <SortableHead col="board" current={sortCol} dir={sortDir} onSort={handleSort} className="w-20 text-center">
                Board
              </SortableHead>
            )}
            <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort} className="w-10 text-center">#</SortableHead>
            <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
            {prep && (
              <SortableHead col="interest" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">
                Interest
              </SortableHead>
            )}
            {prep && <TableHead className="text-center">Plan</TableHead>}
            <TableHead>Trend</TableHead>
            <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Pos</SortableHead>
            <SortableHead col="age" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Age</SortableHead>
            <SortableHead col="grade" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Grade</SortableHead>
            <SortableHead col="pts" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Proj Pts</SortableHead>
            <SortableHead col="ppr" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">PPR/G</SortableHead>
            <SortableHead col="vor" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">VOR</SortableHead>
            <SortableHead col="auction" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Auction $</SortableHead>
            {showConsensus && (
              <>
                <SortableHead col="cons" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Cons $</SortableHead>
                <SortableHead col="edge" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Edge</SortableHead>
              </>
            )}
            <SortableHead col="confidence" current={sortCol} dir={sortDir} onSort={handleSort} className="text-center">Confidence</SortableHead>
            <TableHead>Profile</TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {sorted.map((p: DraftPlayer, i) => {
            const mine = prep?.entry(p.gsis_id)
            return (
              <ClickableRow key={p.gsis_id} href={`/players/${p.gsis_id}`}>
                {prep && (
                  <TableCell className="text-center" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-center gap-1">
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
                    {/*
                      Three thumbs each way, filling outward from the centre —
                      one thumb up is "like", three is "must draft". The strongest
                      notch sits furthest from the middle on both sides, so the
                      row reads as a scale rather than six separate buttons.
                    */}
                    <div className="flex items-center justify-center">
                      {SCALE_ORDER.map((level) => {
                        const meta = INTEREST_LEVELS.find((l) => l.level === level)!
                        const filled = isFilled(level, mine?.interest ?? null)
                        const Icon = level > 0 ? ThumbsUp : ThumbsDown
                        return (
                          <button
                            key={level}
                            onClick={() => prep.setInterest(p.gsis_id, level)}
                            title={`${meta.label} (${meta.short})`}
                            aria-label={`${meta.label} — ${p.name}`}
                            aria-pressed={mine?.interest === level}
                            className={`flex h-5 w-[17px] items-center justify-center hover:opacity-100 ${
                              filled ? '' : 'opacity-70'
                            } ${level === 1 ? 'ml-1.5' : ''}`}
                          >
                            <Icon className={`h-3 w-3 ${interestIconClass(level, filled)}`} />
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
                <TableCell>
                  <TrendSparkline points={p.trajectory ?? []} />
                </TableCell>
                <TableCell className="text-center text-muted-foreground">{p.position_group}</TableCell>
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
                  {p.proj_league_fpts.toFixed(1)}
                </TableCell>
                <TableCell className="text-right tabular-nums font-mono text-muted-foreground">
                  {p.proj_fpts_ppr_pg.toFixed(1)}
                </TableCell>
                <TableCell className="text-right tabular-nums font-mono">
                  {p.vor.toFixed(1)}
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
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
