// Shared display vocabulary for situational notes (injuries, depth-chart
// battles, etc.) — used by the player-detail page's full note list and by
// the table-row notes button/tooltip, so the two surfaces can't drift.

export const NOTE_CATEGORY_LABELS: Record<string, string> = {
  injury: 'Injury',
  depth_chart: 'Depth chart',
  scheme: 'Scheme',
  holdout: 'Holdout',
  suspension: 'Suspension',
  rookie_buzz: 'Rookie buzz',
  trade: 'Trade',
}

export const NOTE_DIRECTION_STYLES: Record<string, string> = {
  positive: 'bg-positive-light text-positive-foreground border-positive-border',
  negative: 'bg-negative-light text-negative-foreground border-negative-border',
  neutral: 'bg-muted text-muted-foreground border-border',
}
