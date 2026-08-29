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

  if (!groups.length) {
    return <p className="text-sm text-muted-foreground">No players to tier yet.</p>
  }

  return (
    <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
      {groups.map(({ position, tiers }) => (
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
            return (
              <div key={tier} className="border-b border-border last:border-0">
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
                    <li key={p.gsis_id} className="flex items-center gap-2 py-0.5">
                      <PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />
                      <RouterLink
                        to={`/players/${p.gsis_id}`}
                        className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                        title={`${p.name} · ${p.team} · ${p.proj_league_fpts.toFixed(0)} proj`}
                      >
                        {p.name}
                      </RouterLink>
                      <span className="shrink-0 font-mono text-[10px] text-muted-foreground">{p.team}</span>
                      <span className="shrink-0 font-mono text-[11px] tabular-nums text-muted-foreground">
                        ${p.auction_value}
                      </span>
                      {prep && (
                        <button
                          onClick={() => setPicking(p)}
                          title="Move to a different tier"
                          aria-label={`Set tier for ${p.name}`}
                          className="shrink-0 font-mono text-[10px] text-muted-foreground hover:text-foreground"
                        >
                          ⋯
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            )
          })}
        </div>
      ))}

      {prep && (
        <TierPickerDialog
          player={picking}
          max={pickingMax}
          prep={prep}
          onClose={() => setPicking(null)}
        />
      )}
    </div>
  )
}

/**
 * Reassign a player's tier bucket. Lists every tier the algorithm has actually
 * produced for this position (plus one, to open a new bottom tier) rather than
 * the full 1–20 schema range — the same set of destinations a drag between
 * buckets could ever reach.
 */
function TierPickerDialog({
  player, max, prep, onClose,
}: {
  player: DraftPlayer | null
  max: number
  prep: PrepControls
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
              prep.setCustomTier(player.gsis_id, null)
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
                prep.setCustomTier(player.gsis_id, t === player.tier ? null : t)
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
