import { useMemo } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer } from '@/api/client'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import type { useDraftPrep } from '../hooks/useDraftPrep'

interface Props {
  players: DraftPlayer[]
  prep: ReturnType<typeof useDraftPrep>
}

/** Draft order, so the panel reads the way a roster is built. */
const POSITION_ORDER = ['QB', 'RB', 'WR', 'TE', 'K', 'DEF', 'DST']

function primaryPosition(p: DraftPlayer): string {
  const group = p.position_group || p.position || ''
  const comma = group.indexOf(',')
  return comma >= 0 ? group.slice(0, comma) : group
}

/**
 * Your targets, grouped by position and then by tier.
 *
 * Tier is the unit that matters while the draft runs: a tier with three of your
 * targets left in it means you can wait, and one with a single name means this
 * is the pick. Grouping by position first is what makes that readable — tiers
 * are computed within position, so a bare "Tier 2" heading spanning RBs and WRs
 * would be comparing two different things.
 */
export function TargetList({ players, prep }: Props) {
  const groups = useMemo(() => {
    const targets = players.filter((p) => prep.entry(p.gsis_id).interest === 1)

    const byPosition = new Map<string, Map<number, DraftPlayer[]>>()
    for (const p of targets) {
      const pos = primaryPosition(p)
      if (!byPosition.has(pos)) byPosition.set(pos, new Map())
      const tiers = byPosition.get(pos)!
      // A custom tier overrides the algorithm's grouping here too, so this
      // panel and the board never disagree about which tier a player is in.
      const tier = prep.entry(p.gsis_id).custom_tier ?? (p.tier || 99)
      if (!tiers.has(tier)) tiers.set(tier, [])
      tiers.get(tier)!.push(p)
    }

    return [...byPosition.entries()]
      .sort(([a], [b]) => {
        const ai = POSITION_ORDER.indexOf(a)
        const bi = POSITION_ORDER.indexOf(b)
        return (ai < 0 ? 99 : ai) - (bi < 0 ? 99 : bi)
      })
      .map(([pos, tiers]) => ({
        position: pos,
        tiers: [...tiers.entries()]
          .sort(([a], [b]) => a - b)
          .map(([tier, members]) => ({
            tier,
            members: members.sort((x, y) => y.proj_league_fpts - x.proj_league_fpts),
          })),
      }))
  }, [players, prep])

  if (!groups.length) {
    return (
      <p className="rounded-lg bg-card px-3 py-3 text-xs text-muted-foreground">
        Mark players with the thumbs-up on the board and they'll collect here,
        grouped by position and tier.
      </p>
    )
  }

  return (
    <div className="space-y-3">
      {groups.map(({ position, tiers }) => (
        <div key={position} className="rounded-lg bg-card">
          <h3 className="border-b border-border px-3 py-1.5 font-display text-[11px] font-semibold uppercase tracking-wide text-foreground">
            {position}{' '}
            <span className="font-mono text-muted-foreground">
              {tiers.reduce((n, t) => n + t.members.length, 0)}
            </span>
          </h3>
          {tiers.map(({ tier, members }) => (
            <div key={tier} className="border-b border-border last:border-0">
              <div className="flex items-baseline gap-1.5 px-3 pt-1.5">
                <span className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                  {tier === 99 ? 'Untiered' : `Tier ${tier}`}
                </span>
                {/* How many of your targets are left in this tier is the whole
                    point: more than one means you can afford to wait. */}
                <span className="font-mono text-[10px] text-muted-foreground/60">
                  {members.length} left
                </span>
              </div>
              <ul className="px-3 pb-1.5">
                {members.map((p) => {
                  const planned = prep.entry(p.gsis_id).planned_cost
                  return (
                    <li key={p.gsis_id} className="flex items-center gap-2 py-0.5">
                      <PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />
                      <RouterLink
                        to={`/players/${p.gsis_id}`}
                        className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                        title={`${p.name} · ${p.team} · ${p.proj_league_fpts.toFixed(0)} proj`}
                      >
                        {p.name}
                      </RouterLink>
                      <span
                        className={`shrink-0 font-mono text-[11px] tabular-nums ${
                          planned != null ? 'text-primary' : 'text-muted-foreground'
                        }`}
                        title={planned != null ? 'Your planned price' : 'Our auction value'}
                      >
                        ${planned ?? p.auction_value}
                      </span>
                      <button
                        onClick={() => prep.setInterest(p.gsis_id, 1)}
                        title="Remove target"
                        aria-label={`Remove ${p.name} as a target`}
                        className="shrink-0 font-mono text-[10px] text-muted-foreground hover:text-foreground"
                      >
                        ×
                      </button>
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}
