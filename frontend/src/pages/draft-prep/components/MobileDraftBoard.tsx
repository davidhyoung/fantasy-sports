import { useEffect, useState } from 'react'
import { ThumbsUp, ThumbsDown } from 'lucide-react'
import type { DraftPlayer } from '@/api/client'
import { PlayerAvatar, type SortDir } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { MobileSortSheet, type MobileSortOption } from '@/components/ui/mobile-sort-sheet'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { gradeColorClass } from '@/lib/grades'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { TrendSparkline } from '@/pages/league-detail/components/TrendSparkline'
import { INTEREST_LEVELS, interestIconClass, interestRowClass } from '../lib/interest'
import { primaryPos, effectiveTier, tierWeightClass, edgeOf, type PrepControls } from './DraftBoardTable'

const SORT_OPTIONS: MobileSortOption[] = [
  { col: 'board', label: 'Board' },
  { col: 'rank', label: '#' },
  { col: 'name', label: 'Player' },
  { col: 'interest', label: 'Interest' },
  { col: 'pos', label: 'Pos' },
  { col: 'tier', label: 'Tier' },
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

interface Props {
  /** Already sorted by the parent. */
  players: DraftPlayer[]
  gradeRankMap: Map<string, number>
  prep?: PrepControls
  showConsensus?: boolean
  sortCol: string
  sortDir: SortDir
  onSort: (col: string) => void
  tierBoundaries: Map<string, { aboveTier: number; displayTier: number }>
  /** Reordering (and its touch equivalent) only makes sense while board order is showing. */
  canMove: boolean
}

/**
 * Card-list replacement for `DraftBoardTable`'s `<Table>` below `md`. Native
 * HTML5 drag-and-drop (the desktop reorder/tier mechanism) never fires on
 * touch, so this isn't a shrunk copy of the desktop board — reordering and
 * tier reassignment get their own tap-driven equivalents (a "move to
 * position" sheet and a "set tier" sheet) that call the exact same
 * `prep.onMove`/`prep.setCustomTier` handlers the drag interactions use.
 */
export function MobileDraftBoard({
  players: sorted, gradeRankMap, prep, showConsensus, sortCol, sortDir, onSort, tierBoundaries, canMove,
}: Props) {
  const options = SORT_OPTIONS.filter((o) => {
    if (o.col === 'board' && !prep) return false
    if ((o.col === 'cons' || o.col === 'edge') && !showConsensus) return false
    return true
  })

  return (
    <div className="space-y-2 md:hidden">
      <div className="flex justify-end">
        <MobileSortSheet options={options} current={sortCol} dir={sortDir} onSort={onSort} />
      </div>
      {sorted.map((p, i) => {
        const divider = tierBoundaries.get(p.gsis_id)
        return (
          <div key={p.gsis_id}>
            {divider && (
              <div className="mb-2 mt-4 px-1 font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground first:mt-0">
                Tier {divider.displayTier}
              </div>
            )}
            <PlayerCard
              player={p}
              index={i}
              players={sorted}
              gradeRank={gradeRankMap.get(p.gsis_id)}
              prep={prep}
              showConsensus={showConsensus}
              canMove={canMove}
            />
          </div>
        )
      })}
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

function PlayerCard({
  player: p, index: i, players: sorted, gradeRank, prep, showConsensus, canMove,
}: {
  player: DraftPlayer
  index: number
  players: DraftPlayer[]
  gradeRank?: number
  prep?: PrepControls
  showConsensus?: boolean
  canMove: boolean
}) {
  const [moveOpen, setMoveOpen] = useState(false)
  const [tierOpen, setTierOpen] = useState(false)
  const [noteOpen, setNoteOpen] = useState(false)
  const [costOpen, setCostOpen] = useState(false)

  const mine = prep?.entry(p.gsis_id)
  const rankDisplay = mine?.custom_rank ?? i + 1
  const tier = effectiveTier(p, prep?.entry)
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
          {rankDisplay}
        </button>
      ) : (
        <span className="font-mono text-xs tabular-nums text-muted-foreground">{p.overall_rank}</span>
      )}
      {prep ? (
        <button
          onClick={() => setTierOpen(true)}
          title="Set tier"
          className={`flex h-11 min-w-9 items-center justify-center rounded px-1.5 font-mono text-[11px] tabular-nums hover:opacity-80 ${tierWeightClass(tier)}`}
        >
          {tier || '—'}
        </button>
      ) : (
        <span className={`inline-block rounded px-1.5 py-0.5 font-mono text-[11px] tabular-nums ${tierWeightClass(p.tier)}`}>
          {p.tier || '—'}
        </span>
      )}
    </>
  )

  const faceExtra = prep && (
    <div className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-1.5">
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
              className="flex h-7 w-7 items-center justify-center rounded hover:bg-muted"
            >
              <Icon className={`h-4 w-4 ${interestIconClass(level, on)}`} />
            </button>
          )
        })}
      </div>
      {mine?.planned_cost == null ? (
        <button
          onClick={() => prep.setPlannedCost(p.gsis_id, p.auction_value)}
          title={`Add to your team at $${p.auction_value}`}
          className="flex h-7 items-center rounded bg-muted px-2 font-display text-[11px] font-semibold text-muted-foreground hover:text-foreground"
        >
          + Plan ${p.auction_value}
        </button>
      ) : (
        <span className="flex items-center gap-1.5">
          <button
            onClick={() => setCostOpen(true)}
            title="Edit planned cost"
            className="font-mono text-xs tabular-nums text-primary"
          >
            ${mine.planned_cost}
          </button>
          <button
            onClick={() => prep.setPlannedCost(p.gsis_id, null)}
            title="Remove from your team"
            className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
          >
            ×
          </button>
        </span>
      )}
    </div>
  )

  const expanded: MobileStatField[] = [
    {
      label: 'Note',
      value: prep ? (
        <button onClick={() => setNoteOpen(true)} className="text-left text-primary underline-offset-2 hover:underline">
          {mine?.note ? mine.note : 'Add note…'}
        </button>
      ) : '—',
    },
    { label: 'Trend', value: <TrendSparkline points={p.trajectory ?? []} /> },
    { label: 'Age', value: p.age || '—' },
    {
      label: 'Grade',
      value: p.player_grade != null ? (
        <>
          <span className={gradeColorClass(p.player_grade)}>{p.player_grade.toFixed(0)}</span>
          {gradeRank != null && <DeltaBadge gradeRank={gradeRank} fantasyRank={p.overall_rank} />}
        </>
      ) : '—',
    },
    { label: 'VOR', value: p.vor.toFixed(1) },
    { label: 'Proj Pts', value: p.proj_league_fpts.toFixed(1) },
    { label: 'Pts/G', value: p.proj_league_ppg.toFixed(1) },
    ...(showConsensus
      ? [
          {
            label: 'Cons $',
            value: p.consensus_auction_value == null
              ? '—'
              : `$${p.consensus_auction_value}${p.consensus_sources === 1 ? '*' : ''}`,
          },
          {
            label: 'Edge',
            value: edge == null ? '—' : `${edge > 0 ? '+' : edge < 0 ? '−' : ''}$${Math.abs(edge)}`,
          },
        ]
      : []),
    { label: 'Confidence', value: <ConfidenceBadge value={p.confidence} /> },
    { label: 'Profile', value: <UniquenessBadge value={p.uniqueness} compCount={p.comp_count} /> },
  ]

  return (
    <>
      <MobileStatCard
        href={`/players/${p.gsis_id}`}
        leading={<PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />}
        title={p.name}
        subtitle={`${p.team ?? ''} · ${p.position_group}`}
        face={face}
        faceExtra={faceExtra}
        expanded={expanded}
        className={interestRowClass(mine?.interest ?? null)}
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
        <TierSheet open={tierOpen} onClose={() => setTierOpen(false)} player={p} players={sorted} prep={prep} />
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

function maxTierForPosition(players: DraftPlayer[], pos: string): number {
  let max = 0
  for (const p of players) {
    if (primaryPos(p) === pos) max = Math.max(max, p.tier || 0)
  }
  return max
}

/**
 * Touch replacement for dropping a player onto a tier divider. Lists every
 * tier the algorithm has actually produced for this position (plus one, to
 * open a new bottom tier) rather than the full 1–20 schema range — the same
 * set of destinations dragging onto a divider could ever reach.
 */
function TierSheet({
  open, onClose, player, players, prep,
}: {
  open: boolean
  onClose: () => void
  player: DraftPlayer
  players: DraftPlayer[]
  prep: PrepControls
}) {
  const pos = primaryPos(player)
  const max = Math.min(20, Math.max(maxTierForPosition(players, pos), player.tier || 1) + 1)
  const mine = prep.entry(player.gsis_id)
  const current = effectiveTier(player, prep.entry)

  return (
    <MobileSheet open={open} onClose={onClose} title={`Set tier — ${player.name}`}>
      <ul>
        <li className="border-b border-border">
          <button
            onClick={() => {
              prep.setCustomTier(player.gsis_id, null)
              onClose()
            }}
            className={`flex min-h-[var(--tap-target-min)] w-full items-center justify-between py-2 font-display text-sm ${
              mine.custom_tier == null ? 'font-semibold text-primary' : 'text-foreground'
            }`}
          >
            Match algorithm
            <span className="font-mono text-xs text-muted-foreground">({player.tier || '—'})</span>
          </button>
        </li>
        {Array.from({ length: max }, (_, idx) => idx + 1).map((t) => (
          <li key={t} className="border-b border-border last:border-0">
            <button
              onClick={() => {
                prep.setCustomTier(player.gsis_id, t === player.tier ? null : t)
                onClose()
              }}
              className={`flex min-h-[var(--tap-target-min)] w-full items-center py-2 font-display text-sm ${
                current === t ? 'font-semibold text-primary' : 'text-foreground'
              }`}
            >
              Tier {t}
            </button>
          </li>
        ))}
      </ul>
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
