import { useState } from 'react'
import { HistoricalSeason, ProjComp } from '@/api/client'

interface TrajectoryChartProps {
  historical: HistoricalSeason[]
  projectedSeason: number
  projectedFptsPPRPG: number
  projectedAge?: number
  comps?: ProjComp[]
}

export default function TrajectoryChart({
  historical,
  projectedSeason,
  projectedFptsPPRPG,
  projectedAge,
  comps,
}: TrajectoryChartProps) {
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null)

  const recent = historical.slice(-6)
  const lastAge = recent.length > 0 ? recent[recent.length - 1].age : 0
  const pAge = projectedAge ?? (lastAge ? lastAge + 1 : 0)

  const allPoints = [
    ...recent.map(h => ({ season: h.season, value: h.fpts_ppr_pg, age: h.age, projected: false })),
    { season: projectedSeason, value: projectedFptsPPRPG, age: pAge, projected: true },
  ]

  if (allPoints.length < 2) return null

  // Comp band: min/max of comp trajectories aligned to our x-axis
  const compBand: { idx: number; min: number; max: number }[] = []
  if (comps && comps.length >= 2) {
    const baseSeason = projectedSeason - 1
    const compValuesBySeason = new Map<number, number[]>()
    for (const comp of comps) {
      if (comp.match_profile.fpts_ppr_pg != null) {
        const arr = compValuesBySeason.get(baseSeason) ?? []
        arr.push(comp.match_profile.fpts_ppr_pg)
        compValuesBySeason.set(baseSeason, arr)
      }
      for (const pt of comp.trajectory ?? []) {
        const mappedSeason = baseSeason + (pt.season - comp.match_season)
        const arr = compValuesBySeason.get(mappedSeason) ?? []
        arr.push(pt.fpts_ppr_pg)
        compValuesBySeason.set(mappedSeason, arr)
      }
    }
    for (let i = 0; i < allPoints.length; i++) {
      const vals = compValuesBySeason.get(allPoints[i].season)
      if (vals && vals.length >= 2) {
        compBand.push({ idx: i, min: Math.min(...vals), max: Math.max(...vals) })
      }
    }
  }

  const values = allPoints.map(p => p.value)
  const bandMins = compBand.map(b => b.min)
  const bandMaxes = compBand.map(b => b.max)
  const allValues = [...values, ...bandMins, ...bandMaxes]
  const minV = Math.min(...allValues) * 0.85
  const maxV = Math.max(...allValues) * 1.1

  const W = 480
  const H = 160
  const PAD = { top: 14, right: 24, bottom: 38, left: 40 }

  const xScale = (i: number) =>
    PAD.left + (i / (allPoints.length - 1)) * (W - PAD.left - PAD.right)
  const yScale = (v: number) =>
    PAD.top + (1 - (v - minV) / (maxV - minV)) * (H - PAD.top - PAD.bottom)

  const histPoints = allPoints.slice(0, allPoints.length - 1)

  const areaPath = histPoints.length >= 2
    ? `M${xScale(0)},${yScale(histPoints[0].value)} ` +
      histPoints.map((p, i) => `L${xScale(i)},${yScale(p.value)}`).join(' ') +
      ` L${xScale(histPoints.length - 1)},${H - PAD.bottom} L${xScale(0)},${H - PAD.bottom} Z`
    : null

  const bandPath = compBand.length >= 2
    ? 'M' + compBand.map(b => `${xScale(b.idx)},${yScale(b.max)}`).join(' L') +
      ' L' + [...compBand].reverse().map(b => `${xScale(b.idx)},${yScale(b.min)}`).join(' L') + ' Z'
    : null

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className="w-full"
      style={{ maxHeight: 200 }}
      onMouseLeave={() => setHoveredIdx(null)}
    >
      {/* Grid lines */}
      {[0.25, 0.5, 0.75, 1].map(f => {
        const y = PAD.top + f * (H - PAD.top - PAD.bottom)
        const v = maxV - f * (maxV - minV)
        return (
          <g key={f}>
            <line x1={PAD.left} y1={y} x2={W - PAD.right} y2={y}
              stroke="currentColor" strokeWidth="0.5" className="text-border" strokeDasharray="3,3" />
            <text x={PAD.left - 4} y={y + 3} textAnchor="end"
              fontSize="8" className="fill-muted-foreground">{v.toFixed(1)}</text>
          </g>
        )
      })}

      {/* Comp trajectory band */}
      {bandPath && <path d={bandPath} fill="hsl(var(--chart-line))" opacity="0.08" />}

      {/* Area fill under historical */}
      {areaPath && <path d={areaPath} fill="hsl(var(--chart-line))" opacity="0.12" />}

      {/* Dashed line: last historical → projection */}
      {(() => {
        const lastHist = allPoints.length - 2
        const projIdx = allPoints.length - 1
        return (
          <line
            x1={xScale(lastHist)} y1={yScale(allPoints[lastHist].value)}
            x2={xScale(projIdx)} y2={yScale(allPoints[projIdx].value)}
            stroke="hsl(var(--chart-line))" strokeWidth="2" strokeDasharray="5,3"
          />
        )
      })()}

      {/* Solid historical line */}
      <polyline
        points={histPoints.map((p, i) => `${xScale(i)},${yScale(p.value)}`).join(' ')}
        fill="none" stroke="hsl(var(--chart-line))" strokeWidth="2"
      />

      {/* Points */}
      {allPoints.map((p, i) => {
        const cx = xScale(i)
        const cy = yScale(p.value)
        const isHovered = hoveredIdx === i
        return (
          <g
            key={i}
            onMouseEnter={() => setHoveredIdx(i)}
            style={{ cursor: 'default' }}
          >
            {/* Invisible large hit area */}
            <rect
              x={cx - 16} y={PAD.top} width={32} height={H - PAD.top - PAD.bottom}
              fill="transparent"
            />

            {/* Point */}
            <circle
              cx={cx} cy={cy}
              r={isHovered ? (p.projected ? 6 : 5) : (p.projected ? 5 : 3.5)}
              fill="hsl(var(--chart-line))"
              stroke="hsl(var(--background))"
              strokeWidth={p.projected ? 2 : 1}
              opacity={p.projected ? 1 : 0.8}
              style={{ transition: 'r 100ms' }}
            />

            {/* Hover value tooltip */}
            {isHovered && (
              <g>
                {/* Bubble background */}
                <rect
                  x={cx - 18} y={cy - 22} width={36} height={14}
                  rx="3" fill="hsl(var(--chart-line))" opacity="0.9"
                />
                <text x={cx} y={cy - 12} textAnchor="middle"
                  fontSize="8.5" fontWeight="bold" fill="white">
                  {p.value.toFixed(1)}
                </text>
              </g>
            )}

            {/* Season label */}
            <text x={cx} y={H - 18} textAnchor="middle"
              fontSize="9"
              className={p.projected ? 'fill-highlight font-bold' : 'fill-muted-foreground'}>
              {p.season}{p.projected ? '*' : ''}
            </text>
            {/* Age label */}
            {p.age > 0 && (
              <text x={cx} y={H - 7} textAnchor="middle"
                fontSize="7.5" className="fill-muted-foreground/60">
                age {p.age}
              </text>
            )}
          </g>
        )
      })}

      {/* Band legend */}
      {bandPath && (
        <text x={W - PAD.right} y={PAD.top - 2} textAnchor="end"
          fontSize="7" className="fill-muted-foreground/50">
          shaded = comp range
        </text>
      )}
    </svg>
  )
}
