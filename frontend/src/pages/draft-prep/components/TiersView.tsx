import { memo, useCallback, useMemo, useState } from 'react'
import type { DraftPlayer, DraftPrepEntry } from '@/api/client'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import { MobileStatCard } from '@/components/ui/mobile-stat-card'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import type { PrepControls } from './DraftBoardTable'

interface Props {
  players: DraftPlayer[]
  /** Read-only (no tier editing) when omitted — the league Draft tab's usage. */
  prep?: PrepControls
  /** Caps the printed sheet to this many players overall (by `overall_rank`) —
   *  a full 700-deep board is a browsing tool on screen, not something anyone
   *  drafts off paper past the realistic player pool. Screen view is
   *  untouched; players beyond the cap just carry `print:hidden`. No cap
   *  (undefined/0) prints everyone. */
  printPoolSize?: number
  /** Opens the player in-place (a drawer/sheet) rather than navigating to `/players/:gsisId`. */
  onPlayerClick: (gsisId: string) => void
}

/** Draft-relevant position order — the order a roster gets built in. */
const POSITION_ORDER = ['QB', 'RB', 'WR', 'TE', 'K', 'DEF', 'DST']

/** Position group can be a comma-separated eligibility list; tiers are per this first one. */
function primaryPos(p: DraftPlayer): string {
  return (p.position_group || p.position || '').split(',')[0]
}

/** A custom tier overrides the algorithm's own — same convention everywhere it's read. */
function effectiveTier(p: DraftPlayer, entry?: (gsisId: string) => DraftPrepEntry): number {
  const custom = entry?.(p.gsis_id).custom_tier
  return custom ?? p.tier
}

/** A player's own valuation, falling back to the algorithm's price when they
 *  have no personal valuation of their own yet. */
function valueOf(p: DraftPlayer, entry?: (gsisId: string) => DraftPrepEntry): number {
  return entry?.(p.gsis_id).my_value ?? p.auction_value
}

function interpolate(above: number | null, below: number | null): number | null {
  if (above != null && below != null) return Math.round((above + below) / 2)
  return above ?? below ?? null
}

/**
 * Where "the players above and below" land for a tier move: the mover's own
 * projected points slot it among the target tier's other members (already
 * sorted by points), and its immediate neighbours there are the answer. At
 * the very top/bottom of the tier — including a brand-new tier with no
 * members yet — this falls through to the boundary player of the nearest
 * real tier on that side.
 */
function neighborValuesForTierMove(
  player: DraftPlayer,
  targetTier: number,
  groups: PositionGroup[],
  entry?: (gsisId: string) => DraftPrepEntry,
): { above: number | null; below: number | null } {
  const group = groups.find((g) => g.position === primaryPos(player))
  if (!group) return { above: null, below: null }

  // Excluded from every tier, not just the target one — otherwise a tier the
  // player is the sole occupant of would reference the player's own pre-move
  // value as its own boundary neighbour once it's (about to be) vacated. Every
  // tier is re-sorted by points here, regardless of how the caller's tiers are
  // ordered for display (which is by value, not points) — points are what
  // place the mover among tier-mates for this purpose, both within the target
  // tier and at a tier boundary, independent of the passed-in array's own sort.
  const tiers = group.tiers.map((t) => ({
    tier: t.tier,
    members: t.members
      .filter((m) => m.gsis_id !== player.gsis_id)
      .slice()
      .sort((x, y) => y.proj_league_fpts - x.proj_league_fpts),
  }))

  const siblings = tiers.find((t) => t.tier === targetTier)?.members ?? []
  let insertAt = siblings.findIndex((m) => m.proj_league_fpts < player.proj_league_fpts)
  if (insertAt < 0) insertAt = siblings.length
  const aboveInTier = insertAt > 0 ? siblings[insertAt - 1] : undefined
  const belowInTier = insertAt < siblings.length ? siblings[insertAt] : undefined

  const priorTier = [...tiers].reverse().find((t) => t.tier > 0 && t.tier < targetTier && t.members.length > 0)
  const nextTier = tiers.find((t) => t.tier > targetTier && t.members.length > 0)

  const above = aboveInTier ?? priorTier?.members[priorTier.members.length - 1]
  const below = belowInTier ?? nextTier?.members[0]

  return {
    above: above ? valueOf(above, entry) : null,
    below: below ? valueOf(below, entry) : null,
  }
}

/**
 * Weight steps down as the tier does, so the top of a position reads as
 * heavier than its replacement-level tail without needing a different colour
 * per tier — there are more tiers than the palette has meanings.
 */
function tierWeightClass(tier: number): string {
  if (tier <= 0) return 'text-muted-foreground/40'
  if (tier <= 2) return 'bg-primary/20 text-primary font-semibold'
  if (tier <= 4) return 'bg-muted text-foreground'
  return 'bg-muted/60 text-muted-foreground'
}

interface TierGroup {
  tier: number
  members: DraftPlayer[]
}

interface PositionGroup {
  position: string
  tiers: TierGroup[]
}

/**
 * Bucket every player by position, then by tier within it — the read this app's
 * tiering algorithm actually supports (tiers are computed within a position;
 * raw point totals aren't comparable across positions, see docs/stats/tiering.md).
 * This is a dedicated view rather than a column on the main board specifically so
 * a tier number is never seen without the position it belongs to right there
 * next to it.
 */
function TiersViewImpl({ players, prep, printPoolSize, onPlayerClick }: Props) {
  const printable = useCallback(
    (p: DraftPlayer) => !printPoolSize || p.overall_rank <= printPoolSize,
    [printPoolSize],
  )

  const groups = useMemo<PositionGroup[]>(() => {
    const byPosition = new Map<string, Map<number, DraftPlayer[]>>()
    for (const p of players) {
      const pos = primaryPos(p)
      if (!byPosition.has(pos)) byPosition.set(pos, new Map())
      const tiers = byPosition.get(pos)!
      const tier = effectiveTier(p, prep?.entry)
      if (!tiers.has(tier)) tiers.set(tier, [])
      tiers.get(tier)!.push(p)
    }

    return [...byPosition.entries()]
      .sort(([a], [b]) => {
        const ai = POSITION_ORDER.indexOf(a)
        const bi = POSITION_ORDER.indexOf(b)
        return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi)
      })
      .map(([position, tiers]) => ({
        position,
        tiers: [...tiers.entries()]
          // Untiered (0/falsy — outside the draftable range the algorithm tiers)
          // sorts last; everyone else ascending, the same order value descends in.
          .sort(([a], [b]) => (a <= 0 ? 1 : 0) - (b <= 0 ? 1 : 0) || a - b)
          .map(([tier, members]) => ({
            tier,
            // Highest value first — your own value where you've set one,
            // falling back to the system's; points break ties, which is most
            // of them until values start getting set; read-only (no prep) has
            // no "my value" concept at all, so it's just the system's price.
            members: members.sort((x, y) => (
              valueOf(y, prep?.entry) - valueOf(x, prep?.entry) || y.proj_league_fpts - x.proj_league_fpts
            )),
          })),
      }))
  }, [players, prep])

  const [picking, setPicking] = useState<DraftPlayer | null>(null)
  const pickingMax = picking
    ? Math.max(
        ...players.filter((p) => primaryPos(p) === primaryPos(picking)).map((p) => p.tier || 0),
        picking.tier || 1,
      ) + 1
    : 0

  // Tracked as state, not just a dataTransfer payload, because dragover needs
  // to know who's being dragged (to grey out cross-position buckets) and
  // DataTransfer.getData() isn't readable until the actual drop.
  const [dragging, setDragging] = useState<DraftPlayer | null>(null)
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)
  // Stable identity so it can be handed to memoized row components as a prop
  // without defeating their memoization.
  const clearDrag = useCallback(() => { setDragging(null); setDragOverKey(null) }, [])

  /**
   * Sets the tier, and — only when the tier actually changes — auto-fills
   * "my value" to sit between whichever players now flank it, so building a
   * tiers view by hand still produces a real price curve without typing every
   * number in yourself. Both go out in one `setFields` write: two independent
   * patches here would each read the same pre-mutation snapshot (nothing has
   * re-rendered between them yet), so the second one's stale copy of the tier
   * would silently revert the first the moment both requests land.
   */
  const moveToTier = (player: DraftPlayer, target: number) => {
    if (!prep) return
    const previousTier = effectiveTier(player, prep.entry)
    const newCustomTier = target === player.tier ? null : target
    if (target === previousTier) {
      prep.setCustomTier(player.gsis_id, newCustomTier)
      return
    }
    // Never overwrite a value the user typed themselves.
    if (prep.entry(player.gsis_id).my_value_source === 'user') {
      prep.setCustomTier(player.gsis_id, newCustomTier)
      return
    }
    const { above, below } = neighborValuesForTierMove(player, target, groups, prep.entry)
    const interpolated = interpolate(above, below)
    prep.setFields(player.gsis_id, {
      customTier: newCustomTier,
      ...(interpolated != null ? { myValue: interpolated, myValueSource: 'derived' } : {}),
    })
  }

  if (!groups.length) {
    return <p className="text-sm text-muted-foreground">No players to tier yet.</p>
  }

  return (
    <>
    <div className="hidden gap-4 md:grid md:grid-cols-2 xl:grid-cols-3 print:grid print:grid-cols-3 print:gap-2">
      {groups.map(({ position, tiers }) => {
        // A position with nobody inside the print pool (realistically only an
        // ultra-late K/DEF panel) drops out of the printed sheet entirely
        // rather than printing an empty card.
        const groupPrintable = tiers.some((t) => t.members.some(printable))
        return (
        <div
          key={position}
          className={`rounded-lg bg-card print:break-inside-avoid ${groupPrintable ? '' : 'print:hidden'}`}
        >
          <h3 className="border-b border-border px-3 py-1.5 font-display text-[11px] font-semibold uppercase tracking-wide text-foreground">
            {position}{' '}
            <span className="font-mono text-muted-foreground">
              {tiers.reduce((n, t) => n + t.members.length, 0)}
            </span>
          </h3>
          {tiers.map(({ tier, members }) => {
            // Computed by scanning, not by array position — members are
            // ordered by value for display now, not points, so the first/last
            // entries no longer line up with the points max/min.
            const pts = members.map((m) => m.proj_league_fpts)
            const top = Math.max(...pts)
            const bottom = Math.min(...pts)
            const key = `${position}-${tier}`
            // Only a same-position drag can land here — tiers aren't comparable
            // across positions, so cross-position drops are refused outright by
            // never calling preventDefault, same convention as roster-slot
            // eligibility gating elsewhere in the app.
            const canDrop = !!prep && !!dragging && primaryPos(dragging) === position
            // Same idea one level down — a tier that's entirely past the print
            // pool (a deep bench tier, or "Untiered") doesn't print its header
            // for zero printed rows underneath it.
            const tierPrintable = members.some(printable)
            return (
              <div
                key={tier}
                className={`border-b border-border last:border-0 ${
                  canDrop ? 'bg-positive-light/10' : ''
                } ${canDrop && dragOverKey === key ? 'ring-1 ring-inset ring-primary' : ''} ${
                  tierPrintable ? '' : 'print:hidden'
                }`}
                onDragOver={
                  canDrop
                    ? (e) => {
                        e.preventDefault()
                        if (dragOverKey !== key) setDragOverKey(key)
                      }
                    : undefined
                }
                onDragLeave={
                  canDrop ? () => setDragOverKey((cur) => (cur === key ? null : cur)) : undefined
                }
                onDrop={
                  canDrop
                    ? (e) => {
                        e.preventDefault()
                        setDragOverKey(null)
                        if (dragging) moveToTier(dragging, tier)
                        setDragging(null)
                      }
                    : undefined
                }
              >
                <div className="flex items-baseline justify-between gap-2 px-3 pt-1.5">
                  <span className={`inline-block rounded px-1.5 py-0.5 font-display text-[10px] font-semibold uppercase tracking-wide ${tierWeightClass(tier)}`}>
                    {tier > 0 ? `Tier ${tier}` : 'Untiered'}
                  </span>
                  <span className="font-mono text-[10px] text-muted-foreground/60">
                    {members.length} · {top === bottom ? top.toFixed(0) : `${bottom.toFixed(0)}–${top.toFixed(0)}`} pts
                  </span>
                </div>
                <ul className="px-3 pb-1.5">
                  {members.map((p) => (
                    <TierMemberRow
                      key={p.gsis_id}
                      player={p}
                      myValue={prep?.entry(p.gsis_id).my_value ?? null}
                      myValueSource={prep?.entry(p.gsis_id).my_value_source ?? null}
                      editable={!!prep}
                      printable={printable(p)}
                      isDragging={dragging?.gsis_id === p.gsis_id}
                      onDragStart={setDragging}
                      onDragEnd={clearDrag}
                      onCommitValue={prep?.setMyValue}
                      onOpenPicker={setPicking}
                      onPlayerClick={onPlayerClick}
                    />
                  ))}
                </ul>
              </div>
            )
          })}
        </div>
        )
      })}
    </div>

    {/* Touch replacement for the desktop grid above — HTML5 drag-and-drop
        (the desktop tier-move mechanism) never fires on touch, so tier
        reassignment here goes entirely through the picker dialog, and value
        editing through a numeric sheet, rather than a drag gesture. */}
    <div className="space-y-4 md:hidden">
      {groups.map(({ position, tiers }) => (
        <div key={position} className="rounded-lg bg-card">
          <h3 className="border-b border-border px-3 py-1.5 font-display text-[11px] font-semibold uppercase tracking-wide text-foreground">
            {position}{' '}
            <span className="font-mono text-muted-foreground">
              {tiers.reduce((n, t) => n + t.members.length, 0)}
            </span>
          </h3>
          {tiers.map(({ tier, members }) => {
            const pts = members.map((m) => m.proj_league_fpts)
            const top = Math.max(...pts)
            const bottom = Math.min(...pts)
            return (
              <div key={tier} className="border-b border-border px-3 py-1.5 last:border-0">
                <div className="flex items-baseline justify-between gap-2 pb-1.5">
                  <span className={`inline-block rounded px-1.5 py-0.5 font-display text-[10px] font-semibold uppercase tracking-wide ${tierWeightClass(tier)}`}>
                    {tier > 0 ? `Tier ${tier}` : 'Untiered'}
                  </span>
                  <span className="font-mono text-[10px] text-muted-foreground/60">
                    {members.length} · {top === bottom ? top.toFixed(0) : `${bottom.toFixed(0)}–${top.toFixed(0)}`} pts
                  </span>
                </div>
                <div className="space-y-1.5">
                  {members.map((p) => (
                    <MobileTierMemberCard
                      key={p.gsis_id}
                      player={p}
                      myValue={prep?.entry(p.gsis_id).my_value ?? null}
                      myValueSource={prep?.entry(p.gsis_id).my_value_source ?? null}
                      editable={!!prep}
                      onCommitValue={prep?.setMyValue}
                      onOpenPicker={setPicking}
                      onPlayerClick={onPlayerClick}
                    />
                  ))}
                </div>
              </div>
            )
          })}
        </div>
      ))}
    </div>

    {prep && (
      <TierPickerDialog
        player={picking}
        max={pickingMax}
        prep={prep}
        onCommit={(target) => picking && moveToTier(picking, target)}
        onClose={() => setPicking(null)}
      />
    )}
    </>
  )
}

/**
 * Memoized so unrelated re-renders of the page (a filter chip, a query
 * refetch elsewhere, another tab's state) don't force ~300 players' worth of
 * grouping/sorting and DOM back through render again — it only actually runs
 * when the player list or the (now-stable) prep controls object changes.
 */
export const TiersView = memo(TiersViewImpl)

interface TierMemberRowProps {
  player: DraftPlayer
  myValue: number | null
  myValueSource: 'user' | 'derived' | null
  editable: boolean
  printable: boolean
  isDragging: boolean
  onDragStart: (player: DraftPlayer) => void
  onDragEnd: () => void
  onCommitValue?: (gsisId: string, value: number | null) => void
  onOpenPicker: (player: DraftPlayer) => void
  onPlayerClick: (gsisId: string) => void
}

/**
 * One player row. Memoized on primitive/stable props so editing one player's
 * value (or dragging one player) only re-renders that row, not every other
 * member across every tier and position panel.
 */
const TierMemberRow = memo(function TierMemberRow({
  player: p, myValue, myValueSource, editable, printable, isDragging, onDragStart, onDragEnd, onCommitValue, onOpenPicker, onPlayerClick,
}: TierMemberRowProps) {
  // The base a nudge starts from — your own value if you've set one,
  // otherwise the system's, same fallback the auto-fill interpolation uses.
  const base = myValue ?? p.auction_value

  return (
    <li
      draggable={editable}
      onDragStart={editable ? () => onDragStart(p) : undefined}
      onDragEnd={editable ? onDragEnd : undefined}
      className={`flex items-center gap-2 py-0.5 ${
        editable ? 'cursor-grab active:cursor-grabbing' : ''
      } ${isDragging ? 'opacity-40' : ''} ${printable ? '' : 'print:hidden'}`}
    >
      <PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />
      <button
        type="button"
        onClick={() => onPlayerClick(p.gsis_id)}
        className="min-w-0 flex-1 truncate text-left font-display text-xs font-semibold text-foreground hover:underline"
        title={`${p.name} · ${p.team} · ${p.proj_league_fpts.toFixed(0)} proj`}
      >
        {p.name}
      </button>
      <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{p.team}</span>
      <span
        className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground"
        title="System value"
      >
        ${p.auction_value}
      </span>
      {editable && onCommitValue && (
        <MyValueField
          value={myValue}
          derived={myValueSource === 'derived'}
          onCommit={(v) => onCommitValue(p.gsis_id, v)}
        />
      )}
      {editable && onCommitValue && (
        // Editor-only chrome — meaningless on a printed sheet, so it drops out there.
        <span className="flex shrink-0 items-center gap-1 print:hidden">
          {/* Nudges your value by $1 — tier changes are drag or the
              picker below, not these; a spot is a dollar, not a tier. */}
          <span className="flex flex-col leading-none">
            <button
              onClick={() => onCommitValue(p.gsis_id, Math.min(10000, base + 1))}
              aria-label={`Increase ${p.name}'s value by $1`}
              title="+$1"
              className="font-mono text-[9px] text-muted-foreground hover:text-foreground"
            >
              ▲
            </button>
            <button
              onClick={() => onCommitValue(p.gsis_id, Math.max(0, base - 1))}
              disabled={base <= 0}
              aria-label={`Decrease ${p.name}'s value by $1`}
              title="−$1"
              className="font-mono text-[9px] text-muted-foreground hover:text-foreground disabled:opacity-30"
            >
              ▼
            </button>
          </span>
          <button
            onClick={() => onOpenPicker(p)}
            title="Move to a different tier"
            aria-label={`Set tier for ${p.name}`}
            className="font-mono text-[10px] text-muted-foreground hover:text-foreground"
          >
            ⋯
          </button>
        </span>
      )}
    </li>
  )
})

/**
 * Touch equivalent of `TierMemberRow`: no drag gesture, so tier reassignment
 * routes through the picker dialog and value editing through a numeric sheet
 * instead. Both funnel through the same `prep.setMyValue`/`moveToTier` the
 * desktop row uses, so there's one behavior underneath either input method.
 */
function MobileTierMemberCard({
  player: p, myValue, myValueSource, editable, onCommitValue, onOpenPicker, onPlayerClick,
}: {
  player: DraftPlayer
  myValue: number | null
  myValueSource: 'user' | 'derived' | null
  editable: boolean
  onCommitValue?: (gsisId: string, value: number | null) => void
  onOpenPicker: (player: DraftPlayer) => void
  onPlayerClick: (gsisId: string) => void
}) {
  const [valueOpen, setValueOpen] = useState(false)
  const derived = myValueSource === 'derived'

  return (
    <>
      <MobileStatCard
        onClick={() => onPlayerClick(p.gsis_id)}
        leading={<PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />}
        title={p.name}
        subtitle={`${p.team} · sys $${p.auction_value}`}
        face={
          editable && onCommitValue ? (
            <button
              onClick={() => setValueOpen(true)}
              title={derived ? 'Interpolated from neighbours — edit to keep it fixed' : 'My value'}
              className="flex h-11 min-w-[3rem] flex-col items-center justify-center rounded font-mono text-xs tabular-nums hover:bg-muted"
            >
              <span className="text-[9px] font-display uppercase text-muted-foreground">Mine</span>
              <span className={derived ? 'border-b border-dotted border-primary text-primary opacity-60' : 'text-primary'}>
                {myValue != null ? `$${myValue}` : '—'}
              </span>
            </button>
          ) : undefined
        }
        expandedExtra={
          editable ? (
            <div className="flex gap-2">
              <button
                onClick={() => onOpenPicker(p)}
                className="flex h-11 flex-1 items-center justify-center rounded-md border border-input font-display text-xs font-semibold text-foreground"
              >
                Set tier ▾
              </button>
              {onCommitValue && (
                <button
                  onClick={() => setValueOpen(true)}
                  className="flex h-11 flex-1 items-center justify-center rounded-md border border-input font-display text-xs font-semibold text-foreground"
                >
                  Edit value
                </button>
              )}
            </div>
          ) : undefined
        }
      />
      {editable && onCommitValue && (
        <MobileSheet open={valueOpen} onClose={() => setValueOpen(false)} title={`My value — ${p.name}`}>
          <MobileValueSheetBody
            value={myValue}
            onSave={(v) => {
              onCommitValue(p.gsis_id, v)
              setValueOpen(false)
            }}
          />
        </MobileSheet>
      )}
    </>
  )
}

function MobileValueSheetBody({ value, onSave }: { value: number | null; onSave: (v: number | null) => void }) {
  const [draft, setDraft] = useState(value != null ? String(value) : '')
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
          if (draft.trim() === '') { onSave(null); return }
          const n = parseInt(draft, 10)
          if (Number.isFinite(n) && n >= 0) onSave(Math.min(n, 10000))
        }}
        className="flex h-11 items-center justify-center rounded-md bg-primary font-display text-sm font-semibold text-primary-foreground hover:bg-primary-hover"
      >
        Save
      </button>
    </div>
  )
}

/**
 * Editable "my value" figure. Blank means no opinion yet — distinct from a
 * literal $0 — so clearing the field removes the override rather than
 * storing a zero. A derived (auto-interpolated) value renders dimmed with a
 * dotted underline — the same affordance HeaderTip uses — so it reads as a
 * suggestion rather than something you typed, until you edit it yourself.
 */
function MyValueField({ value, derived, onCommit }: { value: number | null; derived: boolean; onCommit: (v: number | null) => void }) {
  const [draft, setDraft] = useState(value != null ? String(value) : '')
  // Adopt server/optimistic changes (e.g. the auto-fill from a move) that
  // didn't come from this field.
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(value != null ? String(value) : '')
  }

  const commit = () => {
    if (draft.trim() === '') {
      if (value != null) onCommit(null)
      return
    }
    const next = parseInt(draft, 10)
    if (!Number.isFinite(next) || next < 0) {
      setDraft(value != null ? String(value) : '')
      return
    }
    if (next !== value) onCommit(Math.min(next, 10000))
  }

  return (
    <span
      className="inline-flex shrink-0 items-baseline"
      title={derived ? 'Interpolated from neighbours — edit to keep it fixed' : 'My value'}
    >
      <span className="font-mono text-[10px] text-primary/70">$</span>
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value.replace(/[^0-9]/g, '').slice(0, 5))}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') setDraft(value != null ? String(value) : '')
        }}
        placeholder="—"
        aria-label="My value"
        className={`w-8 bg-transparent text-right font-mono text-[11px] tabular-nums focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary placeholder:text-muted-foreground/40 ${
          derived ? 'border-b border-dotted border-primary text-primary opacity-60' : 'text-primary'
        }`}
      />
    </span>
  )
}

/**
 * Reassign a player's tier bucket. Lists every tier the algorithm has actually
 * produced for this position (plus one, to open a new bottom tier) rather than
 * the full 1–20 schema range — the same set of destinations a drag between
 * buckets could ever reach.
 */
function TierPickerDialog({
  player, max, prep, onCommit, onClose,
}: {
  player: DraftPlayer | null
  max: number
  prep: PrepControls
  onCommit: (target: number) => void
  onClose: () => void
}) {
  if (!player) return <ResponsiveDialog open={false} onClose={onClose}>{null}</ResponsiveDialog>

  const mine = prep.entry(player.gsis_id)
  const current = effectiveTier(player, prep.entry)

  return (
    <ResponsiveDialog open={!!player} onClose={onClose} title={`Set tier — ${player.name}`}>
      <ul>
        <li className="border-b border-border">
          <button
            onClick={() => {
              onCommit(player.tier)
              onClose()
            }}
            className={`flex min-h-[2.5rem] w-full items-center justify-between py-2 font-display text-sm ${
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
                onCommit(t)
                onClose()
              }}
              className={`flex min-h-[2.5rem] w-full items-center py-2 font-display text-sm ${
                current === t ? 'font-semibold text-primary' : 'text-foreground'
              }`}
            >
              Tier {t}
            </button>
          </li>
        ))}
      </ul>
    </ResponsiveDialog>
  )
}
