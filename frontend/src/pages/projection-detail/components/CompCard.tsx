import { ProjComp, ProjTrajPoint } from '@/api/client'

interface CompCardProps {
  comp: ProjComp
  rank: number
}

export default function CompCard({ comp, rank }: CompCardProps) {
  const simPct = Math.round(comp.similarity * 100)
  const weightPct = Math.round(comp.weight * 100)

  // Collect relevant non-zero stats from match profile into a compact inline display
  const stats: { label: string; value: string }[] = []
  const mp = comp.match_profile
  if (mp.fpts_ppr_pg != null && mp.fpts_ppr_pg > 0.5)
    stats.push({ label: 'PPR/G', value: mp.fpts_ppr_pg.toFixed(1) })
  if (mp.pass_yds_pg != null && mp.pass_yds_pg > 1)
    stats.push({ label: 'Pass', value: mp.pass_yds_pg.toFixed(0) })
  if (mp.rush_yds_pg != null && mp.rush_yds_pg > 1)
    stats.push({ label: 'Rush', value: mp.rush_yds_pg.toFixed(0) })
  if (mp.rec_yds_pg != null && mp.rec_yds_pg > 1)
    stats.push({ label: 'Rec', value: mp.rec_yds_pg.toFixed(0) })
  if (mp.rec_pg != null && mp.rec_pg > 0.1)
    stats.push({ label: 'Rec/G', value: mp.rec_pg.toFixed(1) })

  const preMatch = comp.pre_match ?? []
  const traj = comp.trajectory ?? []
  const hasCareer = preMatch.length > 0 || traj.length > 0

  return (
    <div className="rounded-lg bg-card p-3 space-y-2">
      {/* Header row: rank + player info + similarity */}
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2.5 min-w-0">
          <span className="text-muted-foreground text-xs tabular-nums w-4 shrink-0">#{rank}</span>
          {comp.headshot_url ? (
            <img
              src={comp.headshot_url}
              alt={comp.name}
              className="w-8 h-8 rounded-full object-cover bg-muted shrink-0"
              onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
            />
          ) : (
            <div className="w-8 h-8 rounded-full bg-muted shrink-0" />
          )}
          <div className="min-w-0">
            <div className="font-semibold text-sm text-foreground truncate">{comp.name}</div>
            <div className="text-[11px] text-muted-foreground">
              {comp.match_season} · Age {comp.match_age}
            </div>
          </div>
        </div>
        <div className="text-right shrink-0">
          <div className="text-sm font-medium text-primary">{simPct}%</div>
          <div className="text-[10px] text-muted-foreground">{weightPct}% wt</div>
        </div>
      </div>

      {/* Dimension tags + inline stats */}
      <div className="flex items-center gap-1.5 flex-wrap">
        {(comp.matching_dims ?? []).map(dim => (
          <span key={dim}
            className="inline-flex items-center rounded-full bg-positive-light px-1.5 py-0.5 text-[10px] font-medium text-positive-foreground">
            {dim}
          </span>
        ))}
        {(comp.divergent_dims ?? []).map(dim => (
          <span key={dim}
            className="inline-flex items-center rounded-full bg-warning-light px-1.5 py-0.5 text-[10px] font-medium text-warning-foreground">
            {dim}
          </span>
        ))}
        {stats.length > 0 && (
          <>
            <span className="text-border mx-0.5">|</span>
            {stats.map(s => (
              <span key={s.label} className="text-[10px] text-muted-foreground">
                <span className="text-muted-foreground/60">{s.label}</span>{' '}
                <span className="font-mono tabular-nums text-foreground/80">{s.value}</span>
              </span>
            ))}
          </>
        )}
      </div>

      {/* Full career sparkline with highlighted match section */}
      {hasCareer ? (
        <CompSparkline
          preMatch={preMatch}
          matchFptsPPRPG={mp.fpts_ppr_pg ?? 0}
          matchAge={comp.match_age}
          trajectory={traj}
        />
      ) : (
        <div className="text-[10px] text-muted-foreground/60 italic">No career data on record</div>
      )}
    </div>
  )
}

function CompSparkline({
  preMatch,
  matchFptsPPRPG,
  matchAge,
  trajectory,
}: {
  preMatch: ProjTrajPoint[]
  matchFptsPPRPG: number
  matchAge: number
  trajectory: ProjTrajPoint[]
}) {
  // Build full career: pre-match + match year + post-match trajectory
  type Point = { age: number; value: number; phase: 'pre' | 'match' | 'post' }

  const points: Point[] = [
    ...preMatch.map(t => ({ age: t.age, value: t.fpts_ppr_pg, phase: 'pre' as const })),
    { age: matchAge, value: matchFptsPPRPG, phase: 'match' as const },
    ...trajectory.map(t => ({ age: t.age, value: t.fpts_ppr_pg, phase: 'post' as const })),
  ]

  if (points.length < 2) return null

  // Index of the match year point
  const matchIdx = preMatch.length

  const values = points.map(p => p.value)
  const minV = Math.min(...values) * 0.8
  const maxV = Math.max(...values) * 1.15

  const W = 200
  const H = 52
  const PAD = { top: 12, right: 8, bottom: 14, left: 8 }

  const xScale = (i: number) =>
    PAD.left + (i / (points.length - 1)) * (W - PAD.left - PAD.right)
  const yScale = (v: number) => {
    if (maxV === minV) return PAD.top + (H - PAD.top - PAD.bottom) / 2
    return PAD.top + (1 - (v - minV) / (maxV - minV)) * (H - PAD.top - PAD.bottom)
  }

  // Highlighted section: match year through end of trajectory
  const highlightStart = xScale(matchIdx)
  const highlightEnd = xScale(points.length - 1)

  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="w-full" style={{ maxHeight: 56 }}>
      {/* Highlighted region behind match + post-match */}
      {trajectory.length > 0 && (
        <rect
          x={highlightStart - 4}
          y={PAD.top - 4}
          width={highlightEnd - highlightStart + 8}
          height={H - PAD.top - PAD.bottom + 8}
          rx={3}
          fill="hsl(var(--chart-line))" opacity="0.08"
        />
      )}

      {/* Pre-match line (dimmed) */}
      {preMatch.length > 0 && (
        <polyline
          points={points.slice(0, matchIdx + 1).map((p, i) => `${xScale(i)},${yScale(p.value)}`).join(' ')}
          fill="none" stroke="hsl(var(--chart-line))" strokeWidth="1.2" opacity="0.25"
        />
      )}

      {/* Match + post-match line (prominent) */}
      <polyline
        points={points.slice(matchIdx).map((_, i) => `${xScale(matchIdx + i)},${yScale(points[matchIdx + i].value)}`).join(' ')}
        fill="none" stroke="hsl(var(--chart-line))" strokeWidth="1.8" opacity="0.85"
      />

      {/* Points + labels */}
      {points.map((p, i) => {
        const isPre = p.phase === 'pre'
        const isMatch = p.phase === 'match'
        const pointOpacity = isPre ? 0.3 : 0.85
        const radius = isMatch ? 2.5 : 2
        const labelColor = isPre ? 'fill-muted-foreground/30' : isMatch ? 'fill-primary' : p.value >= matchFptsPPRPG ? 'fill-positive' : 'fill-negative'

        return (
          <g key={i}>
            <circle
              cx={xScale(i)} cy={yScale(p.value)} r={radius}
              fill="hsl(var(--chart-line))" opacity={pointOpacity}
            />
            {/* Value above — only show for match year and post-match, or first/last pre-match */}
            {(!isPre || i === 0) && (
              <text x={xScale(i)} y={yScale(p.value) - 4} textAnchor="middle"
                fontSize="7" className={labelColor}>
                {p.value.toFixed(1)}
              </text>
            )}
            {/* Age below */}
            <text x={xScale(i)} y={H - 2} textAnchor="middle"
              fontSize="6.5" className={isPre ? 'fill-muted-foreground/25' : 'fill-muted-foreground/60'}>
              {p.age}
            </text>
          </g>
        )
      })}
    </svg>
  )
}
