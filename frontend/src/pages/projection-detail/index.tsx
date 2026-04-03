import { useParams, Link } from 'react-router-dom'
import { useProjectionDetail } from './hooks/useProjectionDetail'
import CompCard from './components/CompCard'
import TrajectoryChart from './components/TrajectoryChart'
import ConfidenceBadge from '../projections/components/ConfidenceBadge'
import UniquenessBadge from '../projections/components/UniquenessBadge'

export default function ProjectionDetail() {
  const { gsisId } = useParams<{ gsisId: string }>()
  const { data, isLoading, isError } = useProjectionDetail(gsisId ?? '')

  if (isLoading) {
    return <div className="text-muted-foreground text-sm p-6">Loading projection…</div>
  }
  if (isError || !data) {
    return (
      <div className="p-6 space-y-2">
        <p className="text-red-600 dark:text-red-400 text-sm">Projection not found.</p>
        <Link to="/projections" className="text-sm text-primary hover:underline">
          ← Back to rankings
        </Link>
      </div>
    )
  }

  const p = data.projection

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      {/* Back */}
      <Link to="/projections" className="text-sm text-muted-foreground hover:text-primary transition-colors">
        ← {data.target_season} Draft Rankings
      </Link>

      {/* Player header */}
      <div className="flex items-center gap-4">
        {data.headshot_url ? (
          <img
            src={data.headshot_url}
            alt={data.name}
            className="w-20 h-20 rounded-full object-cover bg-muted shrink-0"
            onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
          />
        ) : (
          <div className="w-20 h-20 rounded-full bg-muted shrink-0" />
        )}
        <div>
          <h1 className="text-2xl font-bold text-foreground">{data.name}</h1>
          <div className="flex items-center gap-2 mt-1 flex-wrap">
            <span className="text-muted-foreground">{data.position_group}</span>
            <span className="text-border">·</span>
            <span className="text-muted-foreground">{data.team || 'FA'}</span>
            {data.age > 0 && (
              <>
                <span className="text-border">·</span>
                <span className="text-muted-foreground">Age {data.age}</span>
              </>
            )}
          </div>
          <div className="flex items-center gap-2 mt-2 flex-wrap">
            <ConfidenceBadge value={data.confidence.overall} />
            <UniquenessBadge value={data.uniqueness} compCount={data.comp_count} />
          </div>
        </div>
      </div>

      {/* Projection summary */}
      <div className="rounded-lg bg-card p-5 space-y-4">
        <h2 className="text-lg font-semibold text-foreground">
          {data.target_season} Projection
        </h2>

        {/* Season totals */}
        <div className="grid grid-cols-3 gap-3">
          <SummaryCard label="PPR Pts" value={p.fpts_ppr.toFixed(1)} highlight />
          <SummaryCard label="Half-PPR Pts" value={p.fpts_half.toFixed(1)} />
          <SummaryCard label="Standard Pts" value={p.fpts.toFixed(1)} />
        </div>

        {/* Per-game rates */}
        <div>
          <div className="text-xs text-muted-foreground mb-2">Per-game rates ({p.games} games projected)</div>
          <div className="grid grid-cols-4 sm:grid-cols-6 gap-2 text-xs">
            {p.fpts_ppr_pg > 0.5 && <MiniStat label="PPR/G" value={p.fpts_ppr_pg.toFixed(1)} />}
            {p.pass_yds_pg > 5 && <MiniStat label="Pass Yds" value={p.pass_yds_pg.toFixed(0)} />}
            {p.pass_td_pg > 0.05 && <MiniStat label="Pass TD" value={p.pass_td_pg.toFixed(2)} />}
            {p.rush_yds_pg > 2 && <MiniStat label="Rush Yds" value={p.rush_yds_pg.toFixed(0)} />}
            {p.rush_td_pg > 0.02 && <MiniStat label="Rush TD" value={p.rush_td_pg.toFixed(2)} />}
            {p.rec_pg > 0.1 && <MiniStat label="Rec" value={p.rec_pg.toFixed(1)} />}
            {p.rec_yds_pg > 2 && <MiniStat label="Rec Yds" value={p.rec_yds_pg.toFixed(0)} />}
            {p.rec_td_pg > 0.02 && <MiniStat label="Rec TD" value={p.rec_td_pg.toFixed(2)} />}
            {p.fg_made_pg > 0.1 && <MiniStat label="FG/G" value={p.fg_made_pg.toFixed(2)} />}
            {p.pat_made_pg > 0.1 && <MiniStat label="PAT/G" value={p.pat_made_pg.toFixed(2)} />}
          </div>
        </div>

        {/* Confidence breakdown */}
        <div>
          <div className="text-xs text-muted-foreground mb-2">Confidence breakdown</div>
          <div className="space-y-1.5">
            <ConfBar label="Similarity quality" value={data.confidence.similarity} weight={25} />
            <ConfBar label="Comp count" value={data.confidence.comp_count} weight={20} />
            <ConfBar label="Comp agreement" value={data.confidence.agreement} weight={25} />
            <ConfBar label="Sample depth" value={data.confidence.sample_depth} weight={15} />
            <ConfBar label="Data quality" value={data.confidence.data_quality} weight={15} />
          </div>
        </div>
      </div>

      {/* Historical trend chart */}
      {data.historical.length > 0 && (
        <div className="rounded-lg bg-card p-5 space-y-2">
          <h2 className="text-base font-semibold text-foreground">Career Trajectory</h2>
          <div className="text-xs text-muted-foreground">
            PPR fantasy points per game · {data.target_season}* = projection
          </div>
          <TrajectoryChart
            historical={data.historical}
            projectedSeason={data.target_season}
            projectedFptsPPRPG={p.fpts_ppr_pg}
            projectedAge={data.age + (data.target_season - data.base_season)}
            comps={data.comps}
          />
          {/* Historical table */}
          <div className="overflow-x-auto mt-2">
            <table className="text-xs w-full">
              <thead>
                <tr className="text-muted-foreground border-b border-border">
                  <th className="py-1 pr-3 text-left font-normal">Season</th>
                  <th className="py-1 pr-3 text-right font-normal">Age</th>
                  <th className="py-1 pr-3 text-right font-normal">Games</th>
                  <th className="py-1 text-right font-normal">PPR/G</th>
                </tr>
              </thead>
              <tbody>
                {data.historical.map(h => (
                  <tr key={h.season} className="border-b border-border/30">
                    <td className="py-1 pr-3 tabular-nums">{h.season}</td>
                    <td className="py-1 pr-3 text-right tabular-nums text-muted-foreground">{h.age}</td>
                    <td className="py-1 pr-3 text-right tabular-nums text-muted-foreground">{h.games}</td>
                    <td className="py-1 text-right tabular-nums font-mono">{h.fpts_ppr_pg.toFixed(1)}</td>
                  </tr>
                ))}
                <tr className="text-primary font-medium">
                  <td className="pt-2 pr-3 tabular-nums">{data.target_season}*</td>
                  <td className="pt-2 pr-3 text-right tabular-nums">{data.age + 1}</td>
                  <td className="pt-2 pr-3 text-right tabular-nums">{p.games}</td>
                  <td className="pt-2 text-right tabular-nums font-mono">{p.fpts_ppr_pg.toFixed(1)}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Comps */}
      <div className="space-y-3">
        <div>
          <h2 className="text-base font-semibold text-foreground">Historical Comparisons</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            {data.comp_count === 0
              ? 'No strong historical matches found. Projection uses position-group baseline.'
              : `${data.comp_count} players found with ≥60% similarity. Each comp's post-match trajectory informs the projection.`}
          </p>
        </div>
        {data.comps.length === 0 ? (
          <div className="rounded-lg bg-card p-4 text-sm text-muted-foreground italic">
            No comps above similarity threshold.
          </div>
        ) : (
          <div className="space-y-3">
            {data.comps.map((comp, i) => (
              <CompCard key={comp.gsis_id + comp.match_season} comp={comp} rank={i + 1} />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ── sub-components ────────────────────────────────────────────────────────────

function SummaryCard({ label, value, highlight }: { label: string; value: string; highlight?: boolean }) {
  return (
    <div className={`rounded-lg p-3 text-center ${highlight ? 'bg-purple-950/40 border border-purple-900/50' : 'bg-muted/30'}`}>
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={`text-2xl font-bold tabular-nums font-mono mt-0.5 ${highlight ? 'text-purple-300' : 'text-foreground'}`}>
        {value}
      </div>
    </div>
  )
}

function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-muted/30 rounded px-2 py-1 text-center">
      <div className="text-muted-foreground">{label}</div>
      <div className="font-mono tabular-nums font-medium">{value}</div>
    </div>
  )
}

function ConfBar({ label, value, weight }: { label: string; value: number; weight: number }) {
  const bar =
    value >= 0.70 ? 'bg-purple-600 dark:bg-purple-400' :
    value >= 0.45 ? 'bg-purple-400 dark:bg-purple-500/70' :
                    'bg-purple-300 dark:bg-purple-600/50'
  return (
    <div className="flex items-center gap-3 text-xs">
      <div className="w-36 text-muted-foreground shrink-0">{label}</div>
      <div className="flex-1 bg-muted/30 rounded-full h-1.5 overflow-hidden">
        <div className={`h-full rounded-full ${bar}`} style={{ width: `${Math.round(value * 100)}%` }} />
      </div>
      <div className="text-muted-foreground w-8 text-right tabular-nums">{Math.round(value * 100)}%</div>
      <div className="text-muted-foreground/50 w-12 text-right">{weight}% wt</div>
    </div>
  )
}
