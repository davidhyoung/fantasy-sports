import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { SidePanel } from '@/components/ui/side-panel'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { PlayerAvatar, HeaderRow, ClickableRow } from '@/components/ui/table-helpers'
import {
  getNBAPlayer, NBA_CATEGORY_LABELS, NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS,
  type NBAStatPeriod, type NBACategory,
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
        <div className="flex w-fit flex-none rounded-lg bg-muted overflow-hidden mb-3">
          {NBA_STAT_PERIODS.map((p) => (
            <button
              key={p}
              onClick={() => onPeriodChange(p)}
              className={`px-2.5 py-1 font-display text-[11px] font-semibold whitespace-nowrap ${
                period === p ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {NBA_STAT_PERIOD_LABELS[p]}
            </button>
          ))}
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
