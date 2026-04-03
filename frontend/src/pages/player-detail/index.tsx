import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getNFLPlayer, NFLSeasonStats, ProjStats, ProjDetailResponse } from '@/api/client'
import { keys } from '@/api/queryKeys'
import GradeCard from './components/GradeCard'
import CompCard from '@/pages/projection-detail/components/CompCard'
import TrajectoryChart from '@/pages/projection-detail/components/TrajectoryChart'
import ConfidenceBadge from '@/pages/projections/components/ConfidenceBadge'
import UniquenessBadge from '@/pages/projections/components/UniquenessBadge'
import { Loader2 } from 'lucide-react'

// ── helpers ────────────────────────────────────────────────────────────────

function fmtHeight(inches: number) {
  if (!inches) return ''
  return `${Math.floor(inches / 12)}'${inches % 12}"`
}

function fmtDraft(club: string, num: number) {
  if (!club && !num) return ''
  if (!num) return club
  return `${club} #${num}`
}

// Determine which stat columns are relevant for a position group.
function visibleCols(posGroup: string, seasons: NFLSeasonStats[]) {
  const any = (key: keyof NFLSeasonStats) =>
    seasons.some(s => (s[key] as number) > 0)

  const cols: { key: keyof NFLSeasonStats; label: string; fmt: (v: number) => string }[] = []

  if (posGroup === 'QB') {
    cols.push(
      { key: 'pass_yards',   label: 'Pass Yds', fmt: v => Math.round(v).toString() },
      { key: 'pass_tds',     label: 'Pass TD',  fmt: v => v.toString() },
      { key: 'interceptions',label: 'INT',       fmt: v => v.toString() },
      { key: 'sacks',        label: 'Sacks',     fmt: v => v.toString() },
      { key: 'rush_yards',   label: 'Rush Yds',  fmt: v => Math.round(v).toString() },
      { key: 'rush_tds',     label: 'Rush TD',   fmt: v => v.toString() },
    )
  } else if (posGroup === 'RB') {
    cols.push(
      { key: 'carries',   label: 'Car',      fmt: v => v.toString() },
      { key: 'rush_yards',label: 'Rush Yds', fmt: v => Math.round(v).toString() },
      { key: 'rush_tds',  label: 'Rush TD',  fmt: v => v.toString() },
      { key: 'targets',   label: 'Tgt',      fmt: v => v.toString() },
      { key: 'receptions',label: 'Rec',      fmt: v => v.toString() },
      { key: 'rec_yards', label: 'Rec Yds',  fmt: v => Math.round(v).toString() },
      { key: 'rec_tds',   label: 'Rec TD',   fmt: v => v.toString() },
    )
  } else if (posGroup === 'WR' || posGroup === 'TE') {
    cols.push(
      { key: 'targets',   label: 'Tgt',     fmt: v => v.toString() },
      { key: 'receptions',label: 'Rec',     fmt: v => v.toString() },
      { key: 'rec_yards', label: 'Rec Yds', fmt: v => Math.round(v).toString() },
      { key: 'rec_tds',   label: 'Rec TD',  fmt: v => v.toString() },
    )
    if (any('rush_yards')) {
      cols.push({ key: 'rush_yards', label: 'Rush Yds', fmt: v => Math.round(v).toString() })
    }
  } else if (posGroup === 'K') {
    cols.push(
      { key: 'fg_made', label: 'FGM',  fmt: v => v.toString() },
      { key: 'fg_att',  label: 'FGA',  fmt: v => v.toString() },
      { key: 'fg_long', label: 'Long', fmt: v => v.toString() },
      { key: 'pat_made',label: 'PAT',  fmt: v => v.toString() },
    )
  }

  return cols
}

// Build a synthetic NFLSeasonStats row from projection per-game rates.
function projectedSeasonRow(proj: ProjStats, targetSeason: number, age: number): NFLSeasonStats {
  const g = proj.games
  return {
    season: targetSeason,
    age,
    team: '',
    games: g,
    completions: 0,
    pass_attempts: 0,
    pass_yards: Math.round(proj.pass_yds_pg * g),
    pass_tds: Math.round(proj.pass_td_pg * g),
    interceptions: 0,
    sacks: 0,
    carries: 0,
    rush_yards: Math.round(proj.rush_yds_pg * g),
    rush_tds: Math.round(proj.rush_td_pg * g),
    fumbles: 0,
    receptions: Math.round(proj.rec_pg * g),
    targets: 0,
    rec_yards: Math.round(proj.rec_yds_pg * g),
    rec_tds: Math.round(proj.rec_td_pg * g),
    fg_made: Math.round(proj.fg_made_pg * g),
    fg_att: 0,
    fg_long: 0,
    pat_made: Math.round(proj.pat_made_pg * g),
    fpts_ppr: proj.fpts_ppr,
    fpts: proj.fpts,
    fpts_ppr_pg: proj.fpts_ppr_pg,
    fpts_pg: proj.fpts_pg,
    tags: [],
  } as NFLSeasonStats
}

// ── main page ──────────────────────────────────────────────────────────────

const FORMATS = [
  { label: 'PPR',      value: 'ppr'      as const },
  { label: 'Half PPR', value: 'half'     as const },
  { label: 'Standard', value: 'standard' as const },
] as const
type ScoringFormat = typeof FORMATS[number]['value']

export default function PlayerDetail() {
  const { gsisId } = useParams<{ gsisId: string }>()
  const navigate = useNavigate()
  const [scoringFormat, setScoringFormat] = useState<ScoringFormat>('ppr')

  const { data, isLoading, isError } = useQuery({
    queryKey: keys.nflPlayer(gsisId ?? ''),
    queryFn: () => getNFLPlayer(gsisId ?? ''),
    enabled: !!gsisId,
    staleTime: 5 * 60 * 1000,
  })

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-muted-foreground text-sm p-6">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading player…
      </div>
    )
  }
  if (isError || !data) {
    return (
      <div className="p-6 space-y-2">
        <p className="text-negative-foreground text-sm">Player not found.</p>
        <button
          onClick={() => navigate(-1)}
          className="text-sm text-primary hover:underline"
        >
          ← Go back
        </button>
      </div>
    )
  }

  const { player, seasons, projection, grades } = data
  const tags = seasons.length > 0 ? (seasons[0].tags ?? []) : []
  const cols = visibleCols(player.position_group, seasons)

  return (
    <div className="max-w-4xl mx-auto space-y-6">
      {/* Back */}
      <button
        onClick={() => navigate(-1)}
        className="text-sm text-muted-foreground hover:text-primary transition-colors"
      >
        ← Back
      </button>

      {/* ── Player header ── */}
      <div className="flex items-start gap-5">
        {player.headshot_url ? (
          <img
            src={player.headshot_url}
            alt={player.name}
            width={96}
            height={96}
            className="w-24 h-24 rounded-full object-cover bg-muted shrink-0"
            onError={e => { (e.target as HTMLImageElement).style.display = 'none' }}
          />
        ) : (
          <div className="w-24 h-24 rounded-full bg-muted shrink-0" />
        )}
        <div className="flex-1 min-w-0">
          <h1 className="text-2xl font-bold text-foreground">{player.name}</h1>

          <div className="flex items-center gap-2 mt-1 flex-wrap text-muted-foreground text-sm">
            <span className="font-medium text-foreground">{player.position_group}</span>
            {player.team && <><span className="text-border">·</span><span>{player.team}</span></>}
            {player.jersey_number > 0 && <span className="text-xs">#{player.jersey_number}</span>}
          </div>

          {/* Physical / background */}
          <div className="flex flex-wrap gap-x-4 gap-y-1 mt-2 text-xs text-muted-foreground">
            {player.height > 0 && <span>{fmtHeight(player.height)}, {player.weight} lbs</span>}
            {player.college && <span>{player.college}</span>}
            {player.rookie_year > 0 && <span>Rookie {player.rookie_year}</span>}
            {fmtDraft(player.draft_club, player.draft_number) && (
              <span>Draft: {fmtDraft(player.draft_club, player.draft_number)}</span>
            )}
          </div>

          {/* Tags */}
          {tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5 mt-3">
              {tags.map(tag => (
                <span
                  key={tag}
                  className="px-2 py-0.5 rounded-full text-xs font-medium bg-highlight-light text-highlight-foreground border border-highlight-border"
                >
                  {tag}
                </span>
              ))}
            </div>
          )}

          {/* Projection badges */}
          {projection && (
            <div className="flex items-center gap-2 mt-3 flex-wrap">
              <ConfidenceBadge value={projection.confidence.overall} />
              <UniquenessBadge value={projection.uniqueness} compCount={projection.comp_count} />
            </div>
          )}
        </div>
      </div>

      {/* ── Player Grade ── */}
      <GradeCard grades={grades} positionGroup={player.position_group} />

      {/* ── Year-over-year stats ── */}
      {seasons.length > 0 && (
        <div className="rounded-lg bg-card p-4 space-y-3">
          <h2 className="text-base font-semibold text-foreground">Season Stats</h2>
          <div className="overflow-x-auto">
            <table className="text-xs w-full">
              <thead>
                <tr className="text-muted-foreground border-b border-border">
                  <th className="py-1.5 pr-3 text-left font-normal">Season</th>
                  <th className="py-1.5 pr-3 text-left font-normal">Team</th>
                  <th className="py-1.5 pr-3 text-right font-normal">Age</th>
                  <th className="py-1.5 pr-3 text-right font-normal">G</th>
                  {cols.map(c => (
                    <th key={c.key} className="py-1.5 pr-3 text-right font-normal whitespace-nowrap">
                      {c.label}
                    </th>
                  ))}
                  <th className="py-1.5 pr-3 text-right font-normal whitespace-nowrap">PPR Pts</th>
                  <th className="py-1.5 text-right font-normal whitespace-nowrap">PPR/G</th>
                </tr>
              </thead>
              <tbody>
                {/* Projected season row — first since seasons are desc */}
                {projection && (() => {
                  const projAge = projection.age + (projection.target_season - projection.base_season)
                  const ps = projectedSeasonRow(projection.projection, projection.target_season, projAge)
                  return (
                    <tr className="border-b border-highlight-border bg-highlight-light text-highlight-foreground">
                      <td className="py-1.5 pr-3 tabular-nums font-medium">{ps.season}*</td>
                      <td className="py-1.5 pr-3 text-highlight-foreground/60 italic text-[10px]">projected</td>
                      <td className="py-1.5 pr-3 text-right tabular-nums text-highlight-foreground/60">{projAge || '—'}</td>
                      <td className="py-1.5 pr-3 text-right tabular-nums text-highlight-foreground/60">{ps.games}</td>
                      {cols.map(c => (
                        <td key={c.key} className="py-1.5 pr-3 text-right tabular-nums font-mono">
                          {(ps[c.key] as number) > 0 ? c.fmt(ps[c.key] as number) : <span className="text-highlight-foreground/20">—</span>}
                        </td>
                      ))}
                      <td className="py-1.5 pr-3 text-right tabular-nums font-mono">
                        {ps.fpts_ppr > 0 ? ps.fpts_ppr.toFixed(1) : <span className="text-highlight-foreground/20">—</span>}
                      </td>
                      <td className="py-1.5 text-right tabular-nums font-mono">
                        {ps.fpts_ppr_pg > 0 ? ps.fpts_ppr_pg.toFixed(1) : <span className="text-highlight-foreground/20">—</span>}
                      </td>
                    </tr>
                  )
                })()}
                {seasons.map(s => (
                  <tr key={s.season} className="border-b border-border/30 hover:bg-muted/10">
                    <td className="py-1.5 pr-3 tabular-nums font-medium">{s.season}</td>
                    <td className="py-1.5 pr-3 text-muted-foreground">{s.team || '—'}</td>
                    <td className="py-1.5 pr-3 text-right tabular-nums text-muted-foreground">{s.age || '—'}</td>
                    <td className="py-1.5 pr-3 text-right tabular-nums text-muted-foreground">{s.games}</td>
                    {cols.map(c => (
                      <td key={c.key} className="py-1.5 pr-3 text-right tabular-nums font-mono">
                        {(s[c.key] as number) > 0 ? c.fmt(s[c.key] as number) : <span className="text-muted-foreground/40">—</span>}
                      </td>
                    ))}
                    <td className="py-1.5 pr-3 text-right tabular-nums font-mono">
                      {s.fpts_ppr > 0 ? s.fpts_ppr.toFixed(1) : <span className="text-muted-foreground/40">—</span>}
                    </td>
                    <td className="py-1.5 text-right tabular-nums font-mono">
                      {s.fpts_ppr_pg > 0 ? s.fpts_ppr_pg.toFixed(1) : <span className="text-muted-foreground/40">—</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ── Projection ── */}
      {projection && (
        <>
          <div className="rounded-lg bg-card p-4 space-y-4">
            <div className="flex items-center justify-between flex-wrap gap-2">
              <h2 className="text-lg font-semibold text-foreground">
                {projection.target_season} Projection
              </h2>
              <div className="flex gap-1 bg-muted rounded-lg p-1">
                {FORMATS.map(f => (
                  <button
                    key={f.value}
                    onClick={() => setScoringFormat(f.value)}
                    className={`px-3 py-1 rounded text-sm font-medium transition-colors ${
                      scoringFormat === f.value
                        ? 'bg-primary text-primary-foreground'
                        : 'text-muted-foreground hover:text-foreground'
                    }`}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>

            {/* Season total */}
            {(() => {
              const proj = projection.projection
              const pts = scoringFormat === 'ppr' ? proj.fpts_ppr : scoringFormat === 'half' ? proj.fpts_half : proj.fpts
              const ptsPg = scoringFormat === 'ppr' ? proj.fpts_ppr_pg : scoringFormat === 'half' ? (proj.games > 0 ? proj.fpts_half / proj.games : 0) : proj.fpts_pg
              const formatLabel = scoringFormat === 'ppr' ? 'PPR' : scoringFormat === 'half' ? 'Half PPR' : 'Standard'
              return (
                <div className="flex items-baseline gap-4">
                  <div className="rounded-lg bg-highlight-light border border-highlight-border px-4 py-3 text-center">
                    <div className="text-xs text-muted-foreground">{formatLabel} Pts</div>
                    <div className="text-2xl font-bold tabular-nums font-mono text-highlight-foreground mt-0.5">{pts.toFixed(1)}</div>
                  </div>
                  <div className="text-sm text-muted-foreground tabular-nums font-mono">
                    {ptsPg.toFixed(1)} <span className="text-xs">/G</span>
                  </div>
                </div>
              )
            })()}

            {/* Per-game rates */}
            <div>
              <div className="text-xs text-muted-foreground mb-2">
                Per-game rates ({projection.projection.games} games projected)
              </div>
              <div className="grid grid-cols-4 sm:grid-cols-6 gap-2 text-xs">
                {projection.projection.pass_yds_pg > 5 && <MiniStat label="Pass Yds" value={projection.projection.pass_yds_pg.toFixed(0)} />}
                {projection.projection.pass_td_pg > 0.05 && <MiniStat label="Pass TD" value={projection.projection.pass_td_pg.toFixed(2)} />}
                {projection.projection.rush_yds_pg > 2 && <MiniStat label="Rush Yds" value={projection.projection.rush_yds_pg.toFixed(0)} />}
                {projection.projection.rush_td_pg > 0.02 && <MiniStat label="Rush TD" value={projection.projection.rush_td_pg.toFixed(2)} />}
                {projection.projection.rec_pg > 0.1 && <MiniStat label="Rec" value={projection.projection.rec_pg.toFixed(1)} />}
                {projection.projection.rec_yds_pg > 2 && <MiniStat label="Rec Yds" value={projection.projection.rec_yds_pg.toFixed(0)} />}
                {projection.projection.rec_td_pg > 0.02 && <MiniStat label="Rec TD" value={projection.projection.rec_td_pg.toFixed(2)} />}
                {projection.projection.fg_made_pg > 0.1 && <MiniStat label="FG/G" value={projection.projection.fg_made_pg.toFixed(2)} />}
                {projection.projection.pat_made_pg > 0.1 && <MiniStat label="PAT/G" value={projection.projection.pat_made_pg.toFixed(2)} />}
              </div>
            </div>

            {/* Confidence breakdown */}
            <div>
              <div className="text-xs text-muted-foreground mb-2">Confidence breakdown</div>
              <div className="space-y-1.5">
                <ConfBar label="Similarity quality" value={projection.confidence.similarity} weight={25} />
                <ConfBar label="Comp count" value={projection.confidence.comp_count} weight={20} />
                <ConfBar label="Comp agreement" value={projection.confidence.agreement} weight={25} />
                <ConfBar label="Sample depth" value={projection.confidence.sample_depth} weight={15} />
                <ConfBar label="Data quality" value={projection.confidence.data_quality} weight={15} />
              </div>
            </div>
          </div>

          {/* Trajectory chart + projection narrative */}
          {projection.historical.length > 0 && (
            <div className="rounded-lg bg-card p-4 space-y-3">
              <h2 className="text-base font-semibold text-foreground">Career Trajectory</h2>
              <div className="text-xs text-muted-foreground">
                PPR fantasy points per game · {projection.target_season}* = projection
              </div>
              <TrajectoryChart
                historical={projection.historical}
                projectedSeason={projection.target_season}
                projectedFptsPPRPG={projection.projection.fpts_ppr_pg}
                projectedAge={projection.age + (projection.target_season - projection.base_season)}
                comps={projection.comps}
              />
              {projection.comps.length > 0 && (
                <ProjectionNarrative projection={projection} />
              )}
            </div>
          )}

          {/* Comps */}
          <div className="space-y-3">
            <div>
              <h2 className="text-base font-semibold text-foreground">Historical Comparisons</h2>
              <p className="text-xs text-muted-foreground mt-0.5">
                {projection.comp_count === 0
                  ? 'No strong historical matches found. Projection uses position-group baseline.'
                  : `${projection.comp_count} players found with ≥60% similarity. Each comp's post-match trajectory informs the projection.`}
              </p>
            </div>
            {projection.comps.length === 0 ? (
              <div className="rounded-lg bg-card p-4 text-sm text-muted-foreground italic">
                No comps above similarity threshold.
              </div>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {projection.comps.map((comp, i) => (
                  <CompCard key={comp.gsis_id + comp.match_season} comp={comp} rank={i + 1} />
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  )
}

// ── sub-components ─────────────────────────────────────────────────────────

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
    value >= 0.70 ? 'bg-highlight' :
    value >= 0.45 ? 'bg-highlight/60' :
                    'bg-highlight/30'
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

// ── Projection narrative ────────────────────────────────────────────────────

// Position-specific career phase thresholds.
// Keep in sync with backend/internal/aging/phases.go (authoritative source).
// prime = peak production window; post-prime = age-related decline becomes a real factor.
const POSITION_PHASES: Record<string, { developing: number; prime: [number, number]; postPrime: number }> = {
  QB:  { developing: 25, prime: [25, 34], postPrime: 35 },
  RB:  { developing: 22, prime: [22, 26], postPrime: 27 },
  WR:  { developing: 24, prime: [24, 30], postPrime: 31 },
  TE:  { developing: 25, prime: [25, 30], postPrime: 31 },
  K:   { developing: 25, prime: [25, 38], postPrime: 39 },
}
const DEFAULT_PHASES = { developing: 24, prime: [24, 30] as [number, number], postPrime: 31 }

function getCareerPhase(posGroup: string, age: number): 'developing' | 'entering-prime' | 'prime' | 'post-prime' | 'late-career' {
  const p = POSITION_PHASES[posGroup] ?? DEFAULT_PHASES
  if (age < p.developing) return 'developing'
  if (age === p.developing || age === p.prime[0]) return 'entering-prime'
  if (age <= p.prime[1]) return 'prime'
  if (age <= p.prime[1] + 3) return 'post-prime'
  return 'late-career'
}

function phaseLabel(phase: string): string {
  switch (phase) {
    case 'developing': return 'still developing'
    case 'entering-prime': return 'entering prime'
    case 'prime': return 'in their prime'
    case 'post-prime': return 'past prime'
    case 'late-career': return 'in late career'
    default: return ''
  }
}

function ProjectionNarrative({ projection }: { projection: ProjDetailResponse }) {
  const { historical, comps, projection: proj, target_season, age, base_season, position_group } = projection

  // YoY direction vs last historical season
  const lastHist = historical[historical.length - 1]
  const prevFptsPg = lastHist?.fpts_ppr_pg ?? 0
  const projFptsPg = proj.fpts_ppr_pg
  const changePct = prevFptsPg > 0 ? ((projFptsPg - prevFptsPg) / prevFptsPg) * 100 : 0
  const isUp = changePct > 4
  const isDown = changePct < -4

  // Projected age + position-specific career phase
  const projAge = age + (target_season - base_season)
  const phase = getCareerPhase(position_group, projAge)
  const ageLabel = phaseLabel(phase)

  // Weighted comp direction: what did comps do in year 1 after their match?
  const compsWithTraj = comps.filter(c => (c.trajectory ?? []).length > 0)
  let weightedGrowthSum = 0
  let weightSum = 0
  for (const comp of compsWithTraj) {
    weightedGrowthSum += comp.weight * comp.trajectory[0].growth
    weightSum += comp.weight
  }
  const avgCompGrowth = weightSum > 0 ? weightedGrowthSum / weightSum : 1
  const compsImproved = compsWithTraj.filter(c => c.trajectory[0].growth > 1.03).length
  const compsDeclined = compsWithTraj.filter(c => c.trajectory[0].growth < 0.97).length

  // Headline
  const directionLabel = isUp
    ? `↑ ${changePct.toFixed(0)}% improvement projected`
    : isDown
    ? `↓ ${Math.abs(changePct).toFixed(0)}% decline projected`
    : '→ Similar output projected'
  const directionColor = isUp ? 'text-positive-foreground' : isDown ? 'text-negative-foreground' : 'text-muted-foreground'

  // Comp consensus sentence
  let compSentence = ''
  if (compsWithTraj.length > 0) {
    if (compsImproved > compsDeclined) {
      compSentence = `${compsImproved} of ${compsWithTraj.length} comparable players improved the following season`
      if (avgCompGrowth > 1) compSentence += ` (avg +${((avgCompGrowth - 1) * 100).toFixed(0)}%)`
    } else if (compsDeclined > compsImproved) {
      compSentence = `${compsDeclined} of ${compsWithTraj.length} comparable players declined the following season`
      if (avgCompGrowth < 1) compSentence += ` (avg ${((avgCompGrowth - 1) * 100).toFixed(0)}%)`
    } else {
      compSentence = `Comparable players were split — ${compsImproved} improved, ${compsDeclined} declined`
    }
  }

  // Age narrative — concise, position-aware
  let ageSentence = ''
  if (prevFptsPg > 0) {
    const agePhrase = `Age ${projAge} (${ageLabel} for ${position_group}s)`
    if (phase === 'developing' || phase === 'entering-prime') {
      ageSentence = isDown ? `${agePhrase} — decline is comp-driven, not age.` : `${agePhrase} — typical development window.`
    } else if (phase === 'prime') {
      ageSentence = isDown ? `${agePhrase} — decline is comp-driven.` : isUp ? `${agePhrase} — comps suggest continued upside.` : `${agePhrase} — steady output expected.`
    } else if (phase === 'post-prime') {
      ageSentence = isDown ? `${agePhrase} — age-related decline is a factor.` : isUp ? `${agePhrase} — bucking the typical aging curve.` : `${agePhrase} — holding steady is a positive sign.`
    } else {
      ageSentence = isDown ? `${agePhrase} — significant age-related decline expected.` : `${agePhrase} — sustaining production here is rare.`
    }
  }

  // Top comp detail for the paragraph
  const top = comps[0]
  const topSim = top ? Math.round(top.similarity * 100) : 0
  const topDims = top?.matching_dims?.slice(0, 3) ?? []
  let topTrajDesc = ''
  if (top && (top.trajectory ?? []).length > 0) {
    const pts = top.trajectory.slice(0, 3)
    const descs = pts.map((t, i) => {
      const pct = Math.round((t.growth - 1) * 100)
      const sign = pct >= 0 ? '+' : ''
      return `${t.fpts_ppr_pg.toFixed(1)} PPR/G (${sign}${pct}%) in year ${i + 1}`
    })
    topTrajDesc = descs.join(', ')
  }

  return (
    <div className="border-t border-border pt-3 space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="text-xs font-semibold text-foreground">Why this projection?</div>
        <span className={`text-xs font-semibold tabular-nums ${directionColor}`}>{directionLabel}</span>
      </div>

      <p className="text-xs text-muted-foreground leading-relaxed">
        {compSentence && <>{compSentence}. </>}
        {ageSentence}
        {top && (
          <> The closest comp is <span className="text-foreground font-medium">{top.name}</span> ({top.match_season}, age {top.match_age}, {topSim}% similarity)
          {topDims.length > 0 && <>, matched on similar {topDims.join(', ')}</>}
          {topTrajDesc && <>. After the match season, {top.name.split(' ').pop()} went {topTrajDesc}</>}
          . </>
        )}
        Based on {comps.length} comp{comps.length !== 1 ? 's' : ''}, {proj.games} games projected.
      </p>
    </div>
  )
}
