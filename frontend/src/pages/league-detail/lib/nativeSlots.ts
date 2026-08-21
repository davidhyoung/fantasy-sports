// Matches the backend's validSlots (league_rosters.go) minus DEF — nflverse
// has no team-defense gsis_ids, so DEF can't be assigned in a native league.
export const ROSTER_SLOTS = ['BN', 'QB', 'RB', 'WR', 'TE', 'K', 'FLEX', 'SFLEX', 'TAXI', 'IR'] as const

// Slots that do NOT count as a starter for weekly scoring — mirrors the
// backend's `slot NOT IN ('BN','TAXI','IR')` filter in ScoreLeagueWeek.
export const BENCH_SLOTS = new Set(['BN', 'TAXI', 'IR'])

// Starters first, then bench/taxi/IR — how a full lineup actually reads,
// as opposed to ROSTER_SLOTS above (dropdown option order, alphabetical-ish
// and not meant to imply a display order).
export const SLOT_DISPLAY_ORDER = ['QB', 'RB', 'WR', 'TE', 'FLEX', 'SFLEX', 'K', 'BN', 'TAXI', 'IR'] as const

// Which real NFL positions can fill each slot — mirrors the backend's
// slotEligiblePositions (league_rosters.go). A slot with no entry (BN, TAXI,
// IR, SFLEX) accepts anyone: BN/TAXI/IR because those slots don't count as a
// starter, SFLEX because superflex is open to every position by definition.
// FLEX takes anyone but QB.
const SLOT_ELIGIBILITY: Record<string, string[] | undefined> = {
  QB: ['QB'],
  RB: ['RB'],
  WR: ['WR'],
  TE: ['TE'],
  K: ['K'],
  FLEX: ['RB', 'WR', 'TE', 'K'],
}

// Slots to omit when *displaying* a player's eligible slots (e.g. the Roster
// table's Eligible column) — not from actual eligibility, which still
// allows placement in either. Since every position is now eligible for
// SFLEX and every non-QB position is eligible for FLEX, listing them next
// to every player's real position would be noise, not information.
export const DISPLAY_HIDDEN_SLOTS = new Set(['FLEX', 'SFLEX'])

export function isSlotEligible(slot: string, position: string): boolean {
  const eligible = SLOT_ELIGIBILITY[slot]
  return eligible == null || eligible.includes(position)
}
