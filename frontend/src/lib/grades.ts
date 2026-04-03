// Shared grade display utilities — used by rankings, player detail, draft, projections, etc.

export function gradeColorClass(grade: number): string {
  if (grade >= 90) return 'text-positive-foreground font-semibold'
  if (grade >= 70) return 'text-highlight-foreground'
  if (grade >= 50) return ''
  return 'text-muted-foreground'
}

export function trendIndicator(trend: number | null | undefined): { text: string; color: string } {
  if (trend == null) return { text: '', color: '' }
  const pct = Math.round(trend * 100)
  if (trend > 0.05) return { text: `+${pct}`, color: 'text-positive-foreground' }
  if (trend < -0.05) return { text: `${pct}`, color: 'text-negative-foreground' }
  return { text: `${pct >= 0 ? '+' : ''}${pct}`, color: 'text-muted-foreground' }
}

export function phaseLabel(phase: string): string {
  switch (phase) {
    case 'developing': return 'Dev'
    case 'entering-prime': return 'Enter'
    case 'prime': return 'Prime'
    case 'post-prime': return 'Post'
    case 'late-career': return 'Late'
    default: return phase
  }
}

export function phaseColor(phase: string): string {
  switch (phase) {
    case 'developing': return 'text-highlight-foreground'
    case 'entering-prime': return 'text-positive-foreground'
    case 'prime': return 'text-positive font-medium'
    case 'post-prime': return 'text-warning-foreground'
    case 'late-career': return 'text-negative-foreground'
    default: return 'text-muted-foreground'
  }
}
