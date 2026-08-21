import { RankingTrajectoryPoint } from '@/api/client'

export function TrendSparkline({ points }: { points: RankingTrajectoryPoint[] }) {
  if (points.length < 2) return <span className="text-muted-foreground/40 text-xs">—</span>

  const values = points.map(p => p.fpts_ppr_pg)
  const minV = Math.min(...values)
  const maxV = Math.max(...values)
  const range = maxV - minV || 1

  const W = 64
  const H = 22
  const PAD = { top: 3, right: 2, bottom: 3, left: 2 }

  const xScale = (i: number) =>
    PAD.left + (i / (points.length - 1)) * (W - PAD.left - PAD.right)
  const yScale = (v: number) =>
    PAD.top + (1 - (v - minV) / range) * (H - PAD.top - PAD.bottom)

  const polyPoints = points.map((p, i) => `${xScale(i)},${yScale(p.fpts_ppr_pg)}`).join(' ')

  // Pink/positive for trending up, blue/negative for down — the design
  // system's semantic colors, not raw hues (matches the Standings L5 strip
  // and Players tab L4 sparkline).
  const trending = values[values.length - 1] >= values[0]
  const stroke = trending ? 'hsl(var(--primary))' : 'hsl(var(--negative))'
  const fill = trending ? 'hsl(var(--positive-light))' : 'hsl(var(--negative-light))'

  const firstX = xScale(0)
  const lastX = xScale(points.length - 1)
  const areaPath =
    `M${firstX},${yScale(values[0])} ` +
    points.map((p, i) => `L${xScale(i)},${yScale(p.fpts_ppr_pg)}`).join(' ') +
    ` L${lastX},${H - PAD.bottom} L${firstX},${H - PAD.bottom} Z`

  return (
    <svg viewBox={`0 0 ${W} ${H}`} width={W} height={H} style={{ display: 'block' }}>
      <path d={areaPath} fill={fill} />
      <polyline points={polyPoints} fill="none" stroke={stroke} strokeWidth="1.5" />
      <circle cx={lastX} cy={yScale(values[values.length - 1])} r="2" fill={stroke} />
    </svg>
  )
}
