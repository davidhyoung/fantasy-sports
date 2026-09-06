import { useEffect, useState } from 'react'
import type { DraftPlayer, InterestLevel } from '@/api/client'
import { PlayerAvatar, type SortDir } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { MobileSortSheet, type MobileSortOption } from '@/components/ui/mobile-sort-sheet'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from '@/pages/league-detail/components/TrendSparkline'
import { INTEREST_LEVELS, interestIconClass, interestRowClass } from '../lib/interest'
import { edgeOf, type AdvancedCol, type PrepControls } from './DraftBoardTable'

const SORT_OPTIONS: MobileSortOption[] = [
  { col: 'board', label: 'Board' },
  { col: 'rank', label: '#' },
  { col: 'name', label: 'Player' },
  { col: 'interest', label: 'Interest' },
  { col: 'pos', label: 'Pos' },
  { col: 'age', label: 'Age' },
  { col: 'grade', label: 'Grade' },
  { col: 'vor', label: 'VOR' },
  { col: 'pts', label: 'Proj Pts' },
  { col: 'ppg', label: 'Pts/G' },
  { col: 'auction', label: 'Auction $' },
  { col: 'cons', label: 'Cons $' },
  { col: 'edge', label: 'Edge' },
  { col: 'confidence', label: 'Confidence' },
]

// Columns that live behind the desktop "Columns ▾" toggle carry the same
// visibility here — the mobile expansion and desktop table should never
// disagree about what's considered "advanced".
const ADVANCED_SORT_COLS = new Set<string>(['rank', 'age', 'vor', 'pts', 'ppg', 'cons', 'edge', 'confidence'])

// A few hundred cards, all mounted, with no cap made the list itself the
// performance problem — this caps what's rendered below `md` to the active
// sort's top N, with a "Show all" row to lift it for the rest of the session.
const CARD_CAP = 150

interface Props {
  /** Already sorted by the parent. */
  players: DraftPlayer[]
  gradeRankMap: Map<string, number>
  prep?: PrepControls
  showConsensus?: boolean
  sortCol: string
  sortDir: SortDir
  onSort: (col: string) => void
  /** Reordering (and its touch equivalent) only makes sense while board order is showing. */
  canMove: boolean
  /** Short "Sorted by X" line shown above the list when board order isn't active. */
  sortNotice?: string | null
  visibleAdvanced: Partial<Record<AdvancedCol, boolean>>
  isVisible: (col: AdvancedCol) => boolean
  recentlyCleared?: Map<string, InterestLevel>
  onUndoInterest?: (gsisId: string) => void
}

/**
 * Card-list replacement for `DraftBoardTable`'s `<Table>` below `md`. Native
 * HTML5 drag-and-drop (the desktop reorder mechanism) never fires on touch,
 * so this isn't a shrunk copy of the desktop board — reordering gets its own
 * tap-driven equivalent (a "move to position" sheet) that calls the exact
 * same `prep.onMove` handler the drag interaction uses.
 */
export function MobileDraftBoard({
  players: sorted, gradeRankMap, prep, showConsensus, sortCol, sortDir, onSort, canMove, sortNotice,
  isVisible, recentlyCleared, onUndoInterest,
}: Props) {
  const options = SORT_OPTIONS.filter((o) => {
    if (o.col === 'board' && !prep) return false
    if ((o.col === 'cons' || o.col === 'edge') && !showConsensus) return false
    if (ADVANCED_SORT_COLS.has(o.col) && !isVisible(o.col as AdvancedCol)) return false
    return true
  })

  const [showAll, setShowAll] = useState(false)
  const visible = showAll ? sorted : sorted.slice(0, CARD_CAP)

  return (
    <div className="space-y-2 md:hidden">
      {sortNotice && (
        <p className="text-xs text-muted-foreground">
          {sortNotice} —{' '}
          <button onClick={() => onSort('board')} className="text-primary underline underline-offset-2">
            back to board order to reorder
          </button>
        </p>
      )}
      <div className="flex justify-end">
        <MobileSortSheet options={options} current={sortCol} dir={sortDir} onSort={onSort} />
      </div>
      {visible.map((p, i) => {
        const mine = prep?.entry(p.gsis_id)
        const clearedLevel = mine?.interest == null ? recentlyCleared?.get(p.gsis_id) : undefined
        return (
          <div key={p.gsis_id}>
            <PlayerCard
              player={p}
              index={i}
              players={sorted}
              gradeRank={gradeRankMap.get(p.gsis_id)}
              prep={prep}
              showConsensus={showConsensus}
              canMove={canMove}
              isVisible={isVisible}
              clearedLevel={clearedLevel}
              onUndoInterest={onUndoInterest}
            />
          </div>
        )
      })}
      {!showAll && sorted.length > CARD_CAP && (
        <button
          onClick={() => setShowAll(true)}
          className="flex h-11 w-full items-center justify-center rounded-lg border border-border bg-card font-display text-sm font-semibold text-muted-foreground hover:text-foreground"
        >
          Show all {sorted.length}
        </button>
      )}
    </div>
  )
}

function DeltaBadge({ gradeRank, fantasyRank }: { gradeRank: number; fantasyRank: number }) {
  const diff = fantasyRank - gradeRank
  if (Math.abs(diff) < 10) return null
  return diff > 0
    ? <span className="ml-1 rounded bg-positive-light px-1 py-0.5 text-[10px] text-positive-foreground">UV</span>
    : <span className="ml-1 rounded bg-negative-light px-1 py-0.5 text-[10px] text-negative-foreground">OV</span>
}

/** Rank + delta vs. the projection's own rank, same composite as the desktop board. */
function RankFace({ customRank, overallRank }: { customRank: number; overallRank: number }) {
  const diff = overallRank - customRank
  if (Math.abs(diff) < 1) return null
  const n = Math.abs(diff)
  return (
    <span className="text-[9px] text-muted-foreground" title={`${n} spot${n === 1 ? '' : 's'} ${diff > 0 ? 'above' : 'below'} the projection's rank`}>
      {diff > 0 ? '▲' : '▼'}{n}
    </span>
  )
}

function PlayerCard({
  player: p, index: i, players: sorted, gradeRank, prep, showConsensus, canMove, isVisible, clearedLevel, onUndoInterest,
}: {
  player: DraftPlayer
  index: number
  players: DraftPlayer[]
  gradeRank?: number
  prep?: PrepControls
  showConsensus?: boolean
  canMove: boolean
  isVisible: (col: AdvancedCol) => boolean
  clearedLevel?: InterestLevel
  onUndoInterest?: (gsisId: string) => void
}) {
  const [moveOpen, setMoveOpen] = useState(false)
  const [noteOpen, setNoteOpen] = useState(false)
  const [costOpen, setCostOpen] = useState(false)

  const mine = prep?.entry(p.gsis_id)
  const rankDisplay = mine?.custom_rank ?? i + 1
  const edge = edgeOf(p)

  const face = (
    <>
      {prep && canMove ? (
        <button
          onClick={() => setMoveOpen(true)}
          title="Move to position"
          className="flex h-11 min-w-[2.75rem] flex-col items-center justify-center rounded font-mono text-xs tabular-nums text-foreground hover:bg-muted"
        >
          <span className="text-[9px] font-display uppercase text-muted-foreground">#</span>
          <span className="flex items-baseline gap-0.5">
            {rankDisplay}
            {mine?.custom_rank != null && <RankFace customRank={mine.custom_rank} overallRank={p.overall_rank} />}
          </span>
        </button>
      ) : (
        <span className="font-mono text-xs tabular-nums text-muted-foreground">{p.overall_rank}</span>
      )}
    </>
  )

  const faceExtra = prep && (
    clearedLevel != null ? (
      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-muted-foreground">Interest cleared</span>
        <button
          onClick={() => onUndoInterest?.(p.gsis_id)}
          className="font-mono text-xs text-primary underline underline-offset-2"
        >
          Undo
        </button>
      </div>
    ) : (
    <div className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-1.5">
        {INTEREST_LEVELS.map(({ level, label }) => {
          const on = mine?.interest === level
          return (
            <button
              key={level}
              onClick={() => prep.setInterest(p.gsis_id, level)}
              title={label}
              aria-label={`${label} — ${p.name}`}
              aria-pressed={on}
              className="flex h-11 w-11 items-center justify-center rounded hover:bg-muted"
            >
              <span className={`text-base ${interestIconClass(level, on)}`}>{level > 0 ? '△' : '▽'}</span>
            </button>
          )
        })}
      </div>
      {mine?.planned_cost == null ? (
        <button
          onClick={() => prep.setPlannedCost(p.gsis_id, p.auction_value)}
          title={`Add to your team at $${p.auction_value}`}
          className="flex h-11 items-center rounded bg-muted px-2 font-display text-[11px] font-semibold text-muted-foreground hover:text-foreground"
        >
          + Plan ${p.auction_value}
        </button>
      ) : (
        <span className="flex items-center gap-1">
          <button
            onClick={() => setCostOpen(true)}
            title="Edit planned cost"
            className="flex h-11 items-center font-mono text-xs tabular-nums text-primary"
          >
            ${mine.planned_cost}
          </button>
          <button
            onClick={() => prep.setPlannedCost(p.gsis_id, null)}
            title="Remove from your team"
            className="flex h-11 w-11 items-center justify-center font-mono text-xs text-muted-foreground hover:text-foreground"
          >
            ×
          </button>
        </span>
      )}
    </div>
    )
  )

  const expanded: MobileStatField[] = [
    ...(isVisible('note')
      ? [{
          label: 'Note',
          value: prep ? (
            <button onClick={() => setNoteOpen(true)} className="text-left text-primary underline-offset-2 hover:underline">
              {mine?.note ? mine.note : 'Add note…'}
            </button>
          ) : '—',
        }]
      : []),
    { label: 'Trend', value: <TrendSparkline points={p.trajectory ?? []} /> },
    ...(isVisible('age') ? [{ label: 'Age', value: p.age || '—' }] : []),
    {
      label: 'Grade',
      value: p.player_grade != null ? (
        <>
          <span className={gradeColorClass(p.player_grade)}>{p.player_grade.toFixed(0)}</span>
          {gradeRank != null && <DeltaBadge gradeRank={gradeRank} fantasyRank={p.overall_rank} />}
        </>
      ) : '—',
    },
    ...(isVisible('vor') ? [{ label: 'VOR', value: p.vor.toFixed(1) }] : []),
    ...(isVisible('pts') ? [{ label: 'Proj Pts', value: p.proj_league_fpts.toFixed(1) }] : []),
    ...(isVisible('ppg') ? [{ label: 'Pts/G', value: p.proj_league_ppg.toFixed(1) }] : []),
    ...(showConsensus && isVisible('cons')
      ? [{
          label: 'Cons $',
          value: p.consensus_auction_value == null
            ? '—'
            : `$${p.consensus_auction_value}${p.consensus_sources === 1 ? '*' : ''}`,
        }]
      : []),
    ...(showConsensus && isVisible('edge')
      ? [{
          label: 'Edge',
          value: edge == null ? '—' : `${edge > 0 ? '+' : edge < 0 ? '−' : ''}$${Math.abs(edge)}`,
        }]
      : []),
    ...(isVisible('confidence') ? [{ label: 'Confidence', value: <ConfidenceBadge value={p.confidence} /> }] : []),
    ...(isVisible('profile') ? [{ label: 'Profile', value: <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} /> }] : []),
  ]

  return (
    <>
      <MobileStatCard
        href={`/players/${p.gsis_id}`}
        leading={<PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />}
        title={<span className={clearedLevel != null ? 'line-through text-muted-foreground' : ''}>{p.name}</span>}
        subtitle={`${p.team ?? ''} · ${p.position_group}`}
        face={face}
        faceExtra={faceExtra}
        expanded={expanded}
        className={clearedLevel != null ? 'opacity-50' : interestRowClass(mine?.interest ?? null)}
      />
      {prep && canMove && (
        <MoveToPositionSheet
          open={moveOpen}
          onClose={() => setMoveOpen(false)}
          player={p}
          index={i}
          players={sorted}
          onMove={prep.onMove}
        />
      )}
      {prep && (
        <MobileSheet open={noteOpen} onClose={() => setNoteOpen(false)} title={`Note — ${p.name}`}>
          <NoteSheetBody
            value={mine?.note ?? ''}
            onSave={(v) => {
              prep.setNote(p.gsis_id, v)
              setNoteOpen(false)
            }}
          />
        </MobileSheet>
      )}
      {prep && mine?.planned_cost != null && (
        <MobileSheet open={costOpen} onClose={() => setCostOpen(false)} title={`Planned cost — ${p.name}`}>
          <CostSheetBody
            value={mine.planned_cost}
            onSave={(v) => {
              prep.setPlannedCost(p.gsis_id, v)
              setCostOpen(false)
            }}
          />
        </MobileSheet>
      )}
    </>
  )
}

/**
 * Touch replacement for the desktop board's drag-and-drop reorder: a stepper +
 * numeric "move to position" entry that resolves to the same
 * `onMove(movingId, neighbourId, place)` call the drag/▲▼ nudge already make,
 * so there's exactly one reordering code path underneath either input method.
 */
function MoveToPositionSheet({
  open, onClose, player, index, players, onMove,
}: {
  open: boolean
  onClose: () => void
  player: DraftPlayer
  index: number
  players: DraftPlayer[]
  onMove: PrepControls['onMove']
}) {
  const [pos, setPos] = useState(index + 1)
  useEffect(() => {
    if (open) setPos(index + 1)
  }, [open, index])

  const commit = () => {
    const reduced = players.filter((p) => p.gsis_id !== player.gsis_id)
    if (reduced.length === 0) {
      onClose()
      return
    }
    const target = Math.max(1, Math.min(reduced.length + 1, pos)) - 1
    let neighbourId: string
    let place: 'before' | 'after'
    if (target <= 0) {
      neighbourId = reduced[0].gsis_id
      place = 'before'
    } else if (target >= reduced.length) {
      neighbourId = reduced[reduced.length - 1].gsis_id
      place = 'after'
    } else {
      neighbourId = reduced[target].gsis_id
      place = 'before'
    }
    onMove(player.gsis_id, neighbourId, place)
    onClose()
  }

  return (
    <MobileSheet open={open} onClose={onClose} title={`Move ${player.name}`}>
      <div className="flex items-center justify-center gap-4 py-2">
        <button
          onClick={() => setPos((v) => Math.max(1, v - 1))}
          aria-label="Move target position up"
          className="flex h-11 w-11 items-center justify-center rounded-md border border-input font-mono text-lg text-foreground"
        >
          −
        </button>
        <div className="flex flex-col items-center">
          <span className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
            Position
          </span>
          <input
            type="number"
            inputMode="numeric"
            value={pos}
            onChange={(e) => setPos(Math.max(1, Math.min(players.length, parseInt(e.target.value, 10) || 1)))}
            className="w-16 bg-transparent text-center font-mono text-2xl tabular-nums text-foreground focus-visible:outline-none"
          />
        </div>
        <button
          onClick={() => setPos((v) => Math.min(players.length, v + 1))}
          aria-label="Move target position down"
          className="flex h-11 w-11 items-center justify-center rounded-md border border-input font-mono text-lg text-foreground"
        >
          +
        </button>
      </div>
      <button
        onClick={commit}
        className="mt-3 flex h-11 w-full items-center justify-center rounded-md bg-primary font-display text-sm font-semibold text-primary-foreground hover:bg-primary-hover"
      >
        Move here
      </button>
    </MobileSheet>
  )
}

function NoteSheetBody({ value, onSave }: { value: string; onSave: (v: string) => void }) {
  const [draft, setDraft] = useState(value)
  return (
    <div className="flex flex-col gap-3">
      <textarea
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        maxLength={500}
        rows={5}
        placeholder="Note…"
        className="w-full rounded-md border border-input bg-background p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
      <button
        onClick={() => onSave(draft)}
        className="flex h-11 items-center justify-center rounded-md bg-primary font-display text-sm font-semibold text-primary-foreground hover:bg-primary-hover"
      >
        Save
      </button>
    </div>
  )
}

function CostSheetBody({ value, onSave }: { value: number; onSave: (v: number) => void }) {
  const [draft, setDraft] = useState(String(value))
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-center gap-1">
        <span className="font-mono text-2xl text-muted-foreground">$</span>
        <input
          type="number"
          inputMode="numeric"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          className="w-24 bg-transparent text-center font-mono text-2xl tabular-nums text-foreground focus-visible:outline-none"
        />
      </div>
      <button
        onClick={() => {
          const n = parseInt(draft, 10)
          if (Number.isFinite(n) && n >= 0) onSave(n)
        }}
        className="flex h-11 items-center justify-center rounded-md bg-primary font-display text-sm font-semibold text-primary-foreground hover:bg-primary-hover"
      >
        Save
      </button>
    </div>
  )
}
