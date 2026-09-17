import { SCORING_LABELS, type ScoringStat } from '../hooks/useDraftSettings'

// Context categories on a full stat line that this app has no scoring-config
// UI for at all — a league's real scoring is always seeded from exactly
// SCORING_STATS' 9 keys (CreateLeague.tsx), nothing ever sets these — so the
// backend always includes them regardless of weight (see league_rosters.go's
// informationalStats) rather than gating them the way every other column is.
export const INFO_STATS = ['pass_int', 'rush_att', 'targets', 'return_td', 'two_pt', 'fumbles_lost'] as const
export type InfoStat = (typeof INFO_STATS)[number]

export const INFO_LABELS: Record<InfoStat, string> = {
  pass_int: 'Int',
  rush_att: 'Rush Att',
  targets: 'Tgt',
  return_td: 'Ret TD',
  two_pt: '2PT',
  fumbles_lost: 'Fum Lost',
}

/** Every stat category the Roster/Players tables can show a column for —
 *  the scored ones (SCORING_STATS) plus the always-shown informational
 *  ones above. Not used by the draft-settings scoring editor, which stays
 *  bounded to SCORING_STATS on purpose (those are the only categories with
 *  a projection to actually price against). */
export type DisplayStat = ScoringStat | InfoStat

export const DISPLAY_LABELS: Record<DisplayStat, string> = { ...SCORING_LABELS, ...INFO_LABELS }

// Box-score order — Passing (Yds, TD, Int) → Rushing (Att, Yds, TD) →
// Receiving (Tgt, Rec, Yds, TD) → Ret (TD) → Misc (2PT) → Fum (Lost) →
// Kicking (FG Made, PAT Made).
export const DISPLAY_STATS: DisplayStat[] = [
  'pass_yds', 'pass_td', 'pass_int',
  'rush_att', 'rush_yds', 'rush_td',
  'targets', 'rec', 'rec_yds', 'rec_td',
  'return_td', 'two_pt', 'fumbles_lost',
  'fg_made', 'pat_made',
]
