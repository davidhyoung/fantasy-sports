import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { SidePanel } from '@/components/ui/side-panel'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { getNBAPlayer, NBA_CATEGORY_LABELS, NBA_STAT_PERIODS, NBA_STAT_PERIOD_LABELS, type NBAStatPeriod } from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  /** The player to show, or null when closed — same "no open-with-no-player
   *  state" convention as the NFL PlayerDetailPanel. */
  espnId: string | null
  leagueId?: number
  period: NBAStatPeriod
  onPeriodChange: (p: NBAStatPeriod) => void
  onClose: () => void
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
export function NBAPlayerDetailPanel({ espnId, leagueId, period, onPeriodChange, onClose }: Props) {
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
