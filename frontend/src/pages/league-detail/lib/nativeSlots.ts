// Matches the backend's validSlots (league_rosters.go) minus DEF — nflverse
// has no team-defense gsis_ids, so DEF can't be assigned in a native league.
export const ROSTER_SLOTS = ['BN', 'QB', 'RB', 'WR', 'TE', 'K', 'FLEX', 'SFLEX', 'TAXI', 'IR'] as const

// Slots that do NOT count as a starter for weekly scoring — mirrors the
// backend's `slot NOT IN ('BN','TAXI','IR')` filter in ScoreLeagueWeek.
export const BENCH_SLOTS = new Set(['BN', 'TAXI', 'IR'])
