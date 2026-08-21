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
// IR) accepts anyone, matching those slots not counting as a starter.
const SLOT_ELIGIBILITY: Record<string, string[] | undefined> = {
  QB: ['QB'],
  RB: ['RB'],
  WR: ['WR'],
  TE: ['TE'],
  K: ['K'],
  FLEX: ['RB', 'WR', 'TE'],
  SFLEX: ['QB', 'RB', 'WR', 'TE'],
}

export function isSlotEligible(slot: string, position: string): boolean {
  const eligible = SLOT_ELIGIBILITY[slot]
  return eligible == null || eligible.includes(position)
}
