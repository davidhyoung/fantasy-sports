/**
 * Best-effort explanations for raw stat abbreviations that come from the API
 * as free-form labels (Yahoo's `display_name`, or our own YoY column labels) —
 * rather than being written by a specific page. Covers the standard NFL/NBA
 * default categories (`backend/internal/yahoo/stats.go`); a league with custom
 * Yahoo categories may show labels not in this list, which is fine —
 * `describeStat` returns undefined and the header just renders with no tooltip.
 */
const STAT_DESCRIPTIONS: Record<string, string> = {
  // NFL — offense
  'pass yds': 'Passing yards',
  'pass td': 'Passing touchdowns',
  'int': 'Interceptions thrown',
  'car': 'Rushing attempts (carries)',
  'rush yds': 'Rushing yards',
  'rush td': 'Rushing touchdowns',
  'tgt': 'Targets — times thrown to',
  'rec': 'Receptions',
  'rec yds': 'Receiving yards',
  'rec td': 'Receiving touchdowns',
  'fum lost': 'Fumbles lost',
  '2-pt': 'Two-point conversions',

  // NFL — kicking
  'fgm': 'Field goals made',
  'fga': 'Field goals attempted',
  'long': 'Longest field goal made',
  'pat': 'Extra points made',
  'xp': 'Extra points made',
  'fg 0-19': 'Field goals made from 0–19 yards',
  'fg 20-29': 'Field goals made from 20–29 yards',
  'fg 30-39': 'Field goals made from 30–39 yards',
  'fg 40-49': 'Field goals made from 40–49 yards',
  'fg 50+': 'Field goals made from 50+ yards',

  // NFL — defense / special teams
  'sacks': 'Quarterback sacks',
  'int (d)': 'Interceptions (defense/special teams)',
  'fr': 'Fumble recoveries',
  'td (d)': 'Defensive or special-teams touchdowns',

  // NBA
  'pts': 'Points scored',
  'ast': 'Assists',
  'reb': 'Rebounds',
  'stl': 'Steals',
  'blk': 'Blocks',
  'to': 'Turnovers',
  'fg%': 'Field goal percentage',
  'ftm': 'Free throws made',
  'fta': 'Free throws attempted',
  'ft%': 'Free throw percentage',
  '3pm': 'Three-pointers made',
  '3pa': 'Three-pointers attempted',
}

/** Looks up a plain-English description for a raw stat label, if we recognize it. */
export function describeStat(label: string): string | undefined {
  return STAT_DESCRIPTIONS[label.trim().toLowerCase()]
}
