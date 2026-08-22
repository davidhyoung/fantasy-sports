/** Small inline sparkline of a player's real fantasy points over the
 *  trailing scored weeks — pink trending up, negative-blue trending down,
 *  muted flat, same semantic colors as the Standings L5 strip. Plain inline
 *  SVG, no charting library, per the app's no-new-chart-dependency
 *  convention. Shared by the Players tab and the Roster table so both
 *  render a player's trend identically. */
export function L4Sparkline({ trend }: { trend?: number[] }) {
  if (!trend || trend.length < 2) return <span className="text-muted-foreground/40 text-xs">—</span>

  const W = 48
  const H = 18
  const PAD = 2
  const minV = Math.min(...trend)
  const maxV = Math.max(...trend)
  const range = maxV - minV || 1
  const xScale = (i: number) => PAD + (i / (trend.length - 1)) * (W - PAD * 2)
  const yScale = (v: number) => PAD + (1 - (v - minV) / range) * (H - PAD * 2)
  const points = trend.map((v, i) => `${xScale(i)},${yScale(v)}`).join(' ')

  const delta = trend[trend.length - 1] - trend[0]
  const stroke = delta > 0.5 ? 'hsl(var(--primary))' : delta < -0.5 ? 'hsl(var(--negative))' : 'hsl(var(--muted-foreground))'

  return (
    <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} style={{ display: 'block', margin: '0 auto' }}>
      <polyline points={points} fill="none" stroke={stroke} strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}
