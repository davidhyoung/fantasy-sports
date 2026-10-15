import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { SidePanel } from '@/components/ui/side-panel'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { PlayerAvatar, HeaderRow, ClickableRow } from '@/components/ui/table-helpers'
import { SelectControl } from '@/components/ui/filter-chip'
import {
  getNBAPlayer, NBA_CATEGORY_LABELS, NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS,
  type NBAStatPeriod, type NBACategory, type NBADarkoDetail, type NBADarkoTrendPoint,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

const SEASON_CATEGORIES = Object.keys(NBA_CATEGORY_LABELS) as NBACategory[]

interface Props {
  /** The player to show, or null when closed — same "no open-with-no-player
   *  state" convention as the NFL PlayerDetailPanel. */
  espnId: string | null
  leagueId?: number
  period: NBAStatPeriod
  onPeriodChange: (p: NBAStatPeriod) => void
  onClose: () => void
  /** Jumps the panel to a different player — wired to each comp row's name
   *  so the comparison chain stays explorable in place. */
  onSelectPlayer: (espnId: string) => void
}

function fmtHeight(inches: number) {
  if (!inches) return null
  return `${Math.floor(inches / 12)}'${inches % 12}"`
}

/**
 * Minimal in-place NBA player detail — a right-docked SidePanel on desktop, a
 * MobileSheet on mobile, opened from a name click on the Players/Roster tabs
 * (same onClick-not-href convention PlayerCell establishes app-wide). No
 * contracts/trade/drop actions this phase — native NBA leagues have no
 * cap/contracts yet (see CLAUDE.md), so unlike the NFL panel there's nothing
 * for a footer action bar to do. The period picker here is independent of
 * whichever table opened it, so switching it doesn't disturb the caller.
 */
export function NBAPlayerDetailPanel({ espnId, leagueId, period, onPeriodChange, onClose, onSelectPlayer }: Props) {
  const open = espnId != null

  const { data } = useQuery({
    queryKey: keys.nbaPlayer(espnId ?? '', period, leagueId),
    queryFn: () => getNBAPlayer(espnId ?? '', period, leagueId),
    enabled: !!espnId,
    staleTime: 5 * 60 * 1000,
  })

  const bio = data ? [fmtHeight(data.height), data.weight ? `${data.weight} lbs` : null, data.college, data.years_exp ? `${data.years_exp} yrs exp` : null].filter(Boolean).join(' · ') : ''

  const body = !espnId ? null : !data ? (
    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  ) : (
    <div className="flex flex-col gap-5">
      <div className="flex items-center gap-3">
        <PlayerAvatar src={data.headshot_url} alt={data.name} size={84} />
        <div>
          <h1 className="font-display text-xl font-bold text-foreground">{data.name}</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            {data.position} · {data.team || 'Free Agent'}
          </p>
          {bio && <p className="text-xs text-muted-foreground mt-1">{bio}</p>}
        </div>
      </div>

      <div>
        <div className="mb-3">
          <SelectControl value={period} onChange={(e) => onPeriodChange(e.target.value as NBAStatPeriod)}>
            {NBA_STAT_PERIODS.map((p) => <option key={p} value={p}>{NBA_STAT_PERIOD_LABELS[p]}</option>)}
          </SelectControl>
        </div>
        <div className="grid grid-cols-3 gap-2">
          {data.stats.map((s) => {
            const isPct = s.category === 'fg_pct' || s.category === 'ft_pct'
            const isAvg = period === 'season_avg' || period === 'last_season_avg'
            return (
              <div key={s.category} className="rounded-lg bg-card p-3 text-center">
                <div className="font-mono text-lg tabular-nums text-foreground">
                  {s.value.toFixed(isPct ? 3 : isAvg ? 1 : 0)}
                </div>
                <div className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground mt-0.5">
                  {NBA_CATEGORY_LABELS[s.category]}
                </div>
              </div>
            )
          })}
        </div>
      </div>

      {data.projection && data.projection.comp_count > 0 && (
        <div>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            {data.projection.target_season} Projection
          </span>
          <p className="text-xs text-muted-foreground mt-1 mb-2">
            Comp-based, grown from {data.name}'s {data.projection.base_season} per-game rate using {data.projection.comp_count} comp{data.projection.comp_count === 1 ? '' : 's'}.
          </p>
          <div className="grid grid-cols-3 gap-2">
            {data.projection.stats.map((s) => {
              const isPct = s.category === 'fg_pct' || s.category === 'ft_pct'
              return (
                <div key={s.category} className="rounded-lg bg-highlight-light border border-highlight-border p-3 text-center">
                  <div className="font-mono text-lg tabular-nums text-highlight-foreground">
                    {s.value.toFixed(isPct ? 3 : 1)}
                  </div>
                  <div className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground mt-0.5">
                    {NBA_CATEGORY_LABELS[s.category]}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {data.seasons.length > 0 && (
        <div>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Season by Season
          </span>
          <div className="mt-2 rounded-lg bg-card overflow-x-auto">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead>Season</TableHead>
                  <TableHead>Team</TableHead>
                  <TableHead className="text-right">GP</TableHead>
                  {SEASON_CATEGORIES.map((cat) => (
                    <TableHead key={cat} className="text-right">{NBA_CATEGORY_LABELS[cat]}</TableHead>
                  ))}
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {data.seasons.map((s) => (
                  <TableRow key={s.season}>
                    <TableCell className="font-mono tabular-nums text-foreground">{s.season}</TableCell>
                    <TableCell className="text-muted-foreground">{s.team || '—'}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{s.games}</TableCell>
                    {SEASON_CATEGORIES.map((cat) => {
                      const v = s.stats.find((e) => e.category === cat)?.value
                      const isPct = cat === 'fg_pct' || cat === 'ft_pct'
                      return (
                        <TableCell key={cat} className="text-right font-mono tabular-nums">
                          {v != null ? v.toFixed(isPct ? 3 : 1) : '—'}
                        </TableCell>
                      )
                    })}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      )}

      {data.darko && (data.darko.trend.length > 0 || data.darko.comps.length > 0) && (
        <DarkoSection darko={data.darko} />
      )}

      {data.comps.length > 0 && (
        <div>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Historical Comparisons
          </span>
          <p className="text-xs text-muted-foreground mt-1 mb-2">
            Closest matches ({data.position}s) by statline, physical profile (height/weight), and career stage
            (young/prime/post-prime) — from {data.name}'s best-sample season.
          </p>
          <div className="rounded-lg bg-card overflow-x-auto">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead>Player</TableHead>
                  <TableHead>Season</TableHead>
                  <TableHead>Team</TableHead>
                  <TableHead className="text-right">Age</TableHead>
                  <TableHead className="text-right">GP</TableHead>
                  {SEASON_CATEGORIES.map((cat) => (
                    <TableHead key={cat} className="text-right">{NBA_CATEGORY_LABELS[cat]}</TableHead>
                  ))}
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {data.comps.map((c) => (
                  <ClickableRow key={`${c.espn_id}-${c.season}`} className="cursor-pointer" onClick={() => onSelectPlayer(c.espn_id)}>
                    <TableCell className="font-medium text-foreground">{c.name}</TableCell>
                    <TableCell className="font-mono tabular-nums text-muted-foreground">{c.season}</TableCell>
                    <TableCell className="text-muted-foreground">{c.team || '—'}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{c.age || '—'}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{c.games}</TableCell>
                    {SEASON_CATEGORIES.map((cat) => {
                      const v = c.stats.find((e) => e.category === cat)?.value
                      const isPct = cat === 'fg_pct' || cat === 'ft_pct'
                      return (
                        <TableCell key={cat} className="text-right font-mono tabular-nums">
                          {v != null ? v.toFixed(isPct ? 3 : 1) : '—'}
                        </TableCell>
                      )
                    })}
                  </ClickableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      )}
    </div>
  )

  return (
    <>
      <div className="hidden md:contents">
        <SidePanel open={open} onClose={onClose}>{body}</SidePanel>
      </div>
      <div className="contents md:hidden">
        <MobileSheet open={open} onClose={onClose}>{body}</MobileSheet>
      </div>
    </>
  )
}

const seasonLabel = (endYear: number) => `${endYear - 1}-${String(endYear).slice(2)}`
const fmtDpm = (v: number) => `${v > 0 ? '+' : ''}${v.toFixed(1)}`

/** Season-end DPM as a small line chart; the zero line is the league-average player. */
function DpmSparkline({ trend }: { trend: NBADarkoTrendPoint[] }) {
  if (trend.length < 2) return null
  const W = 240, H = 56, P = 4
  const vals = trend.map((t) => t.dpm)
  const lo = Math.min(0, ...vals), hi = Math.max(0, ...vals)
  const span = hi - lo || 1
  const x = (i: number) => P + (i * (W - 2 * P)) / (trend.length - 1)
  const y = (v: number) => H - P - ((v - lo) / span) * (H - 2 * P)
  const pts = trend.map((t, i) => `${x(i).toFixed(1)},${y(t.dpm).toFixed(1)}`).join(' ')
  return (
    <svg viewBox={`0 0 ${W} ${H}`} className="h-14 w-60 text-foreground" role="img" aria-label="DPM at the end of each season">
      <line x1={P} x2={W - P} y1={y(0)} y2={y(0)} className="stroke-border" strokeWidth={1} strokeDasharray="2 3" />
      <polyline points={pts} fill="none" stroke="currentColor" strokeWidth={1.5} />
      <circle cx={x(trend.length - 1)} cy={y(trend[trend.length - 1].dpm)} r={2.5} fill="currentColor" />
    </svg>
  )
}

/** DARKO's DPM trajectory and nearest historical comps for one player. */
function DarkoSection({ darko }: { darko: NBADarkoDetail }) {
  const trend = darko.trend
  const last = trend[trend.length - 1]
  const prev = trend.length > 1 ? trend[trend.length - 2] : null
  const dpms = trend.map((t) => t.dpm)
  return (
    <div>
      <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        DARKO
      </span>
      {trend.length > 0 && (
        <div className="mt-2 mb-4">
          <p className="text-xs text-muted-foreground mb-2">
            Daily Plus-Minus (impact per 100 possessions vs. an average player) at the end of each season.
            {prev && (
              <> Last season{' '}
                <span className={`font-mono ${last.dpm - prev.dpm >= 0 ? 'text-positive' : 'text-secondary'}`}>
                  {last.dpm - prev.dpm >= 0 ? '▲' : '▼'} {Math.abs(last.dpm - prev.dpm).toFixed(1)}
                </span>
                {' '}({fmtDpm(prev.dpm)} → {fmtDpm(last.dpm)}).
              </>
            )}
            {trend.length > 2 && <> Range: <span className="font-mono">{fmtDpm(Math.min(...dpms))}</span> to <span className="font-mono">{fmtDpm(Math.max(...dpms))}</span>.</>}
          </p>
          <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
            <DpmSparkline trend={trend} />
            <div className="rounded-lg bg-card overflow-x-auto">
              <Table>
                <TableHeader style={{ top: 0 }}>
                  <HeaderRow>
                    <TableHead>Season</TableHead>
                    <TableHead className="text-right">DPM</TableHead>
                    <TableHead className="text-right">Change</TableHead>
                  </HeaderRow>
                </TableHeader>
                <TableBody>
                  {[...trend].reverse().slice(0, 6).map((t, i, arr) => {
                    const older = arr[i + 1]
                    const d = older ? t.dpm - older.dpm : null
                    return (
                      <TableRow key={t.season}>
                        <TableCell className="font-mono tabular-nums text-foreground">{seasonLabel(t.season)}</TableCell>
                        <TableCell className="text-right font-mono tabular-nums">{fmtDpm(t.dpm)}</TableCell>
                        <TableCell className="text-right font-mono tabular-nums">
                          {d == null ? (
                            <span className="text-muted-foreground/40">—</span>
                          ) : (
                            <span className={Math.abs(d) < 0.5 ? 'text-muted-foreground' : d > 0 ? 'text-positive' : 'text-secondary'}>
                              {d > 0 ? '+' : ''}{d.toFixed(1)}
                            </span>
                          )}
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          </div>
        </div>
      )}
      {darko.comps.length > 0 && (
        <div>
          <p className="text-xs text-muted-foreground mb-2">
            DARKO's nearest historical player-seasons at this player's age (similarity out of 100) — a different
            method from the statline comps below.
          </p>
          <div className="rounded-lg bg-card overflow-x-auto">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead>Player</TableHead>
                  <TableHead>Season</TableHead>
                  <TableHead className="text-right">Age</TableHead>
                  <TableHead className="text-right">Similarity</TableHead>
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {darko.comps.map((c) => (
                  <TableRow key={`${c.name}-${c.season}`}>
                    <TableCell className="font-medium text-foreground">{c.name}</TableCell>
                    <TableCell className="font-mono tabular-nums text-muted-foreground">{seasonLabel(c.season)}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{c.age.toFixed(1)}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums">{c.similarity.toFixed(0)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>
      )}
    </div>
  )
}
