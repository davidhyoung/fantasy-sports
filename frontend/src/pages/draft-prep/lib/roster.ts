import type { DraftPlayer } from '@/api/client'
import type { DraftSettings, SlotPosition } from '@/pages/league-detail/hooks/useDraftSettings'

/** A player you plan to roster, at the price you plan to pay. */
export interface PlannedPlayer {
  player: DraftPlayer
  cost: number
}

export interface FilledSlot {
  /** QB, RB, …, FLEX, SFLEX, BN */
  slot: SlotPosition
  player?: PlannedPlayer
}

/** Which positions each slot can start. Mirrors the backend's flex eligibility. */
const SLOT_ELIGIBILITY: Record<SlotPosition, string[] | null> = {
  QB: ['QB'],
  RB: ['RB'],
  WR: ['WR'],
  TE: ['TE'],
  K: ['K'],
  DEF: ['DEF', 'DST'],
  FLEX: ['RB', 'WR', 'TE'],
  SFLEX: ['QB', 'RB', 'WR', 'TE'],
  BN: null, // bench takes anyone
}

/** Order slots are filled in: the most restrictive first, so a flex isn't
 *  consumed by a player who was the only one eligible for a dedicated slot. */
const FILL_ORDER: SlotPosition[] = ['QB', 'RB', 'WR', 'TE', 'K', 'DEF', 'FLEX', 'SFLEX', 'BN']

function positionOf(p: DraftPlayer): string {
  const group = p.position_group || p.position || ''
  const comma = group.indexOf(',')
  return comma >= 0 ? group.slice(0, comma) : group
}

export interface RosterSummary {
  slots: FilledSlot[]
  /** Planned players who had no slot left — the roster is over capacity. */
  overflow: PlannedPlayer[]
  spent: number
  budget: number
  remaining: number
  /** Roster spots with nobody in them yet. */
  emptySlots: number
  emptyStarters: number
  /** Most you could bid on one player and still fill every remaining spot at $1. */
  maxBid: number
  /** Projected league points from the starting lineup only. */
  starterPoints: number
  /** Points above replacement for the starters — how much the lineup actually wins you. */
  starterVOR: number
  warnings: string[]
}

/**
 * Assembles planned players into the league's roster and reports what the team
 * looks like.
 *
 * Assignment is greedy by projected points within each slot type, filling the
 * most restrictive slots first. That ordering matters: filling FLEX first could
 * put your only kicker on the bench and leave K empty. It is not a global
 * optimum — a true assignment problem — but for a roster this size the greedy
 * result matches the optimum in every ordinary case, and it stays predictable,
 * which is worth more here than the last fraction of a point.
 */
export function buildRoster(
  planned: PlannedPlayer[],
  settings: DraftSettings,
  replacementByPosition: Map<string, number>,
): RosterSummary {
  const slots: FilledSlot[] = []
  for (const slot of FILL_ORDER) {
    const count = settings.slots[slot] ?? 0
    for (let i = 0; i < count; i++) slots.push({ slot })
  }

  // Best players first, so the strongest available fills each slot.
  const pool = [...planned].sort(
    (a, b) => b.player.proj_league_fpts - a.player.proj_league_fpts,
  )
  const used = new Set<string>()

  for (const filled of slots) {
    const eligible = SLOT_ELIGIBILITY[filled.slot]
    const pick = pool.find(
      (p) => !used.has(p.player.gsis_id) && (eligible === null || eligible.includes(positionOf(p.player))),
    )
    if (pick) {
      filled.player = pick
      used.add(pick.player.gsis_id)
    }
  }

  const overflow = pool.filter((p) => !used.has(p.player.gsis_id))
  const spent = planned.reduce((sum, p) => sum + p.cost, 0)
  const budget = settings.budget
  const remaining = budget - spent
  const emptySlots = slots.filter((s) => !s.player).length
  const starterSlots = slots.filter((s) => s.slot !== 'BN')
  const emptyStarters = starterSlots.filter((s) => !s.player).length

  // Every other empty spot still needs at least $1, so that's what you can't spend.
  const maxBid = Math.max(0, remaining - Math.max(0, emptySlots - 1))

  let starterPoints = 0
  let starterVOR = 0
  for (const s of starterSlots) {
    if (!s.player) continue
    const pts = s.player.player.proj_league_fpts
    starterPoints += pts
    starterVOR += Math.max(0, pts - (replacementByPosition.get(positionOf(s.player.player)) ?? 0))
  }

  const warnings: string[] = []
  if (spent > budget) {
    warnings.push(`Over budget by $${spent - budget}.`)
  } else if (remaining < emptySlots) {
    warnings.push(
      `Only $${remaining} left for ${emptySlots} empty spot${emptySlots === 1 ? '' : 's'} — every roster spot costs at least $1.`,
    )
  }
  if (overflow.length) {
    warnings.push(
      `${overflow.length} planned player${overflow.length === 1 ? '' : 's'} won't fit on the roster.`,
    )
  }
  if (emptyStarters > 0 && planned.length > 0) {
    warnings.push(`${emptyStarters} starting spot${emptyStarters === 1 ? '' : 's'} still empty.`)
  }

  return {
    slots, overflow, spent, budget, remaining,
    emptySlots, emptyStarters, maxBid,
    starterPoints, starterVOR, warnings,
  }
}
