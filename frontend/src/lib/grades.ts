// Shared grade display utilities — used by rankings, player detail, draft, projections, etc.

export function gradeColorClass(grade: number): string {
  if (grade >= 90) return 'text-emerald-600 dark:text-emerald-400 font-semibold'
  if (grade >= 70) return 'text-purple-600 dark:text-purple-400'
  if (grade >= 50) return ''
  return 'text-muted-foreground'
}

export function trendIndicator(trend: number | null | undefined): { text: string; color: string } {
  if (trend == null) return { text: '', color: '' }
  const pct = Math.round(trend * 100)
  if (trend > 0.05) return { text: `+${pct}`, color: 'text-emerald-600 dark:text-emerald-400' }
  if (trend < -0.05) return { text: `${pct}`, color: 'text-red-600 dark:text-red-400' }
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
    case 'developing': return 'text-sky-600 dark:text-sky-400'
    case 'entering-prime': return 'text-emerald-600 dark:text-emerald-400'
    case 'prime': return 'text-emerald-700 dark:text-emerald-300'
    case 'post-prime': return 'text-amber-600 dark:text-amber-400'
    case 'late-career': return 'text-red-600 dark:text-red-400'
    default: return 'text-muted-foreground'
  }
}
