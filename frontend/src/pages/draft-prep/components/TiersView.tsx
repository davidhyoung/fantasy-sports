import { useMemo, useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer, DraftPrepEntry } from '@/api/client'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import type { PrepControls } from './DraftBoardTable'

interface Props {
  players: DraftPlayer[]
  /** Read-only (no tier editing) when omitted — the league Draft tab's usage. */
  prep?: PrepControls
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
  // value as its own boundary neighbour once it's (about to be) vacated.
  const tiers = group.tiers.map((t) => ({
    tier: t.tier,
    members: t.members.filter((m) => m.gsis_id !== player.gsis_id),
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
export function TiersView({ players, prep }: Props) {
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
            members: members.sort((x, y) => y.proj_league_fpts - x.proj_league_fpts),
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
    const { above, below } = neighborValuesForTierMove(player, target, groups, prep.entry)
    const interpolated = interpolate(above, below)
    prep.setFields(player.gsis_id, {
      customTier: newCustomTier,
      ...(interpolated != null ? { myValue: interpolated } : {}),
    })
  }

  if (!groups.length) {
    return <p className="text-sm text-muted-foreground">No players to tier yet.</p>
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      {groups.map(({ position, tiers }) => {
        // The "+1" destination past the last real tier — same "open a new
        // bottom tier" slot the picker dialog and the down arrow both reach.
        const posMaxTier = Math.max(0, ...tiers.map((t) => (t.tier > 0 ? t.tier : 0)))

        return (
        <div key={position} className="rounded-lg bg-card">
          <h3 className="border-b border-border px-3 py-1.5 font-display text-[11px] font-semibold uppercase tracking-wide text-foreground">
            {position}{' '}
            <span className="font-mono text-muted-foreground">
              {tiers.reduce((n, t) => n + t.members.length, 0)}
            </span>
          </h3>
          {tiers.map(({ tier, members }) => {
            const top = members[0].proj_league_fpts
            const bottom = members[members.length - 1].proj_league_fpts
            const key = `${position}-${tier}`
            // Only a same-position drag can land here — tiers aren't comparable
            // across positions, so cross-position drops are refused outright by
            // never calling preventDefault, same convention as roster-slot
            // eligibility gating elsewhere in the app.
            const canDrop = !!prep && !!dragging && primaryPos(dragging) === position
            return (
              <div
                key={tier}
                className={`border-b border-border last:border-0 ${
                  canDrop ? 'bg-positive-light/10' : ''
                } ${canDrop && dragOverKey === key ? 'ring-1 ring-inset ring-primary' : ''}`}
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
                  {members.map((p) => {
                    // Untiered reads as sitting just past the last real tier for
                    // arrow purposes, so ▲ from there promotes into the bottom
                    // real tier instead of jumping straight to Tier 1.
                    const cur = tier > 0 ? tier : posMaxTier + 1
                    return (
                    <li
                      key={p.gsis_id}
                      draggable={!!prep}
                      onDragStart={prep ? () => setDragging(p) : undefined}
                      onDragEnd={prep ? () => { setDragging(null); setDragOverKey(null) } : undefined}
                      className={`flex items-center gap-2 py-0.5 ${
                        prep ? 'cursor-grab active:cursor-grabbing' : ''
                      } ${dragging?.gsis_id === p.gsis_id ? 'opacity-40' : ''}`}
                    >
                      <PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />
                      <RouterLink
                        to={`/players/${p.gsis_id}`}
                        className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                        title={`${p.name} · ${p.team} · ${p.proj_league_fpts.toFixed(0)} proj`}
                      >
                        {p.name}
                      </RouterLink>
                      <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{p.team}</span>
                      <span
                        className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground"
                        title="System value"
                      >
                        ${p.auction_value}
                      </span>
                      {prep && (
                        <MyValueField
                          value={prep.entry(p.gsis_id).my_value}
                          onCommit={(v) => prep.setMyValue(p.gsis_id, v)}
                        />
                      )}
                      {prep && (
                        <>
                          <span className="flex shrink-0 flex-col leading-none">
                            <button
                              onClick={() => moveToTier(p, Math.max(1, cur - 1))}
                              disabled={cur <= 1}
                              aria-label={`Move ${p.name} up a tier`}
                              title="Move up a tier"
                              className="font-mono text-[9px] text-muted-foreground hover:text-foreground disabled:opacity-30"
                            >
                              ▲
                            </button>
                            <button
                              onClick={() => moveToTier(p, Math.min(posMaxTier + 1, cur + 1))}
                              disabled={cur >= posMaxTier + 1}
                              aria-label={`Move ${p.name} down a tier`}
                              title="Move down a tier"
                              className="font-mono text-[9px] text-muted-foreground hover:text-foreground disabled:opacity-30"
                            >
                              ▼
                            </button>
                          </span>
                          <button
                            onClick={() => setPicking(p)}
                            title="Move to a different tier"
                            aria-label={`Set tier for ${p.name}`}
                            className="shrink-0 font-mono text-[10px] text-muted-foreground hover:text-foreground"
                          >
                            ⋯
                          </button>
                        </>
                      )}
                    </li>
                    )
                  })}
                </ul>
              </div>
            )
          })}
        </div>
        )
      })}

      {prep && (
        <TierPickerDialog
          player={picking}
          max={pickingMax}
          prep={prep}
          onCommit={(target) => picking && moveToTier(picking, target)}
          onClose={() => setPicking(null)}
        />
      )}
    </div>
  )
}

/**
 * Editable "my value" figure. Blank means no opinion yet — distinct from a
 * literal $0 — so clearing the field removes the override rather than
 * storing a zero.
 */
function MyValueField({ value, onCommit }: { value: number | null; onCommit: (v: number | null) => void }) {
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
    <span className="inline-flex shrink-0 items-baseline" title="My value">
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
        className="w-8 bg-transparent text-right font-mono text-[11px] tabular-nums text-primary focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary placeholder:text-muted-foreground/40"
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
