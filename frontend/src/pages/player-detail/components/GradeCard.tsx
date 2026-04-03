import { useState } from 'react'
import { NFLPlayerGradeSeason } from '@/api/client'

interface Props {
  grades: NFLPlayerGradeSeason[]
  positionGroup: string
}

function gradeColor(grade: number): string {
  if (grade >= 90) return 'text-positive-foreground'
  if (grade >= 70) return 'text-highlight-foreground'
  if (grade >= 50) return 'text-foreground'
  return 'text-muted-foreground'
}

function gradeBg(grade: number): string {
  if (grade >= 90) return 'bg-positive-light border-positive-border'
  if (grade >= 70) return 'bg-highlight-light border-highlight-border'
  if (grade >= 50) return 'bg-muted/30 border-border'
  return 'bg-muted/20 border-border/50'
}

function gradeLabel(grade: number): string {
  if (grade >= 90) return 'Elite'
  if (grade >= 80) return 'Great'
  if (grade >= 70) return 'Good'
  if (grade >= 55) return 'Average'
  if (grade >= 40) return 'Below Avg'
  return 'Poor'
}

function barColor(grade: number): string {
  if (grade >= 90) return 'bg-positive'
  if (grade >= 70) return 'bg-highlight'
  if (grade >= 50) return 'bg-muted-foreground/40'
  return 'bg-muted-foreground/25'
}

function trendArrow(trend: number | null): string {
  if (trend == null) return ''
  if (trend > 0.05) return '↑'
  if (trend < -0.05) return '↓'
  return '→'
}

function trendColor(trend: number | null): string {
  if (trend == null) return 'text-muted-foreground'
  if (trend > 0.05) return 'text-positive-foreground'
  if (trend < -0.05) return 'text-negative-foreground'
  return 'text-muted-foreground'
}

function phaseLabel(phase: string): string {
  switch (phase) {
    case 'developing': return 'Developing'
    case 'entering-prime': return 'Entering Prime'
    case 'prime': return 'Prime'
    case 'post-prime': return 'Post-Prime'
    case 'late-career': return 'Late Career'
    default: return phase
  }
}

const SUB_SCORES: { key: keyof Pick<NFLPlayerGradeSeason, 'production' | 'efficiency' | 'usage' | 'durability'>; label: string }[] = [
  { key: 'production', label: 'Production' },
  { key: 'efficiency', label: 'Efficiency' },
  { key: 'usage', label: 'Usage' },
  { key: 'durability', label: 'Durability' },
]

export default function GradeCard({ grades }: Props) {
  if (grades.length === 0) return null

  // grades arrive sorted DESC (most recent first)
  const [selectedSeason, setSelectedSeason] = useState(grades[0].season)
  const selected = grades.find(g => g.season === selectedSeason) ?? grades[0]
  const trend = selected.yoy_trend

  return (
    <div className="rounded-lg bg-card p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-foreground">Player Grade</h2>
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          {grades.length > 1 ? (
            <select
              value={selectedSeason}
              onChange={e => setSelectedSeason(Number(e.target.value))}
              className="rounded border border-border bg-card px-1.5 py-0.5 text-xs text-foreground cursor-pointer focus:outline-none focus:ring-1 focus:ring-ring"
            >
              {grades.map(g => (
                <option key={g.season} value={g.season}>{g.season}</option>
              ))}
            </select>
          ) : (
            <span>{selected.season} season</span>
          )}
          <span className="text-border">·</span>
          <span>{phaseLabel(selected.career_phase)}</span>
        </div>
      </div>

      <div className="flex items-start gap-5">
        {/* Overall grade circle */}
        <div className={`flex flex-col items-center justify-center rounded-xl border p-4 min-w-[100px] shrink-0 ${gradeBg(selected.overall)}`}>
          <div className={`text-3xl font-bold tabular-nums font-mono ${gradeColor(selected.overall)}`}>
            {selected.overall.toFixed(0)}
          </div>
          <div className={`text-xs font-medium mt-0.5 ${gradeColor(selected.overall)}`}>
            {gradeLabel(selected.overall)}
          </div>
          {trend != null && (
            <div className={`text-xs mt-1 font-medium tabular-nums ${trendColor(trend)}`}>
              {trendArrow(trend)} {trend > 0 ? '+' : ''}{(trend * 100).toFixed(0)} YoY
            </div>
          )}
        </div>

        {/* Sub-score bars */}
        <div className="flex-1 space-y-2.5 min-w-0">
          {SUB_SCORES.map(({ key, label }) => {
            const val = selected[key] as number
            return (
              <div key={key} className="space-y-0.5">
                <div className="flex items-center justify-between text-xs">
                  <span className="text-muted-foreground">{label}</span>
                  <span className={`font-mono tabular-nums font-medium ${gradeColor(val)}`}>{val.toFixed(0)}</span>
                </div>
                <div className="h-1.5 bg-muted/30 rounded-full overflow-hidden">
                  <div
                    className={`h-full rounded-full transition-all ${barColor(val)}`}
                    style={{ width: `${Math.min(val, 100)}%` }}
                  />
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {/* Collapsible line graph — grade history */}
      {grades.length > 1 && (() => {
        const sorted = [...grades].reverse() // oldest → newest
        const padX = 28
        const padTop = 16
        const padBot = 22
        const w = 300
        const h = 130
        const minG = Math.min(...sorted.map(g => g.overall))
        const maxG = Math.max(...sorted.map(g => g.overall))
        const range = maxG - minG || 1
        const yMin = Math.max(0, minG - range * 0.15)
        const yMax = Math.min(100, maxG + range * 0.15)
        const yRange = yMax - yMin || 1
        const pts = sorted.map((g, i) => ({
          x: padX + (sorted.length === 1 ? (w - 2 * padX) / 2 : i * ((w - 2 * padX) / (sorted.length - 1))),
          y: padTop + (1 - (g.overall - yMin) / yRange) * (h - padTop - padBot),
          g,
        }))
        const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x},${p.y}`).join(' ')

        return (
          <details className="border-t border-border pt-2 group">
            <summary className="text-xs text-muted-foreground cursor-pointer select-none list-none flex items-center gap-1 hover:text-foreground transition-colors">
              <svg className="w-3 h-3 transition-transform group-open:rotate-90" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round"><path d="M4 2l4 4-4 4" /></svg>
              Grade History
            </summary>
            <svg viewBox={`0 0 ${w} ${h}`} className="w-full mt-2" style={{ maxHeight: '140px' }}>
              {/* grid lines */}
              {[yMin, yMin + yRange / 2, yMax].map(v => {
                const y = padTop + (1 - (v - yMin) / yRange) * (h - padTop - padBot)
                return (
                  <g key={v}>
                    <line x1={padX} x2={w - padX} y1={y} y2={y} stroke="currentColor" className="text-border" strokeWidth={0.5} strokeDasharray="3,3" />
                    <text x={padX - 4} y={y + 3} textAnchor="end" className="fill-muted-foreground" fontSize={9} fontFamily="monospace">{v.toFixed(0)}</text>
                  </g>
                )
              })}
              {/* line */}
              <path d={line} fill="none" stroke="currentColor" className="text-highlight" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
              {/* dots + labels */}
              {pts.map((p, i) => {
                const isSelected = p.g.season === selectedSeason
                return (
                  <g key={p.g.season} className="cursor-pointer" onClick={() => setSelectedSeason(p.g.season)}>
                    <circle cx={p.x} cy={p.y} r={isSelected ? 5 : 3.5} className={isSelected ? 'fill-highlight' : 'fill-card stroke-highlight'} strokeWidth={isSelected ? 0 : 1.5} />
                    {isSelected && (
                      <text x={p.x} y={p.y - 8} textAnchor="middle" className="fill-foreground" fontSize={10} fontWeight={600} fontFamily="monospace">{p.g.overall.toFixed(0)}</text>
                    )}
                    <text x={p.x} y={h - 4} textAnchor="middle" className={isSelected ? 'fill-foreground' : 'fill-muted-foreground'} fontSize={9} fontWeight={isSelected ? 600 : 400} fontFamily="monospace">
                      {i === 0 || i === pts.length - 1 || isSelected ? `'${String(p.g.season).slice(2)}` : ''}
                    </text>
                  </g>
                )
              })}
            </svg>
          </details>
        )
      })()}
    </div>
  )
}
