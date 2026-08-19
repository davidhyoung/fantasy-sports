import { Loader2 } from 'lucide-react'
import { Link as RouterLink } from 'react-router-dom'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { HeaderRow, HeaderTip } from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { useStandings } from './hooks/useStandings'

interface Props {
  leagueId: number
  active: boolean
  yahooKeyToId: Record<string, number>
}

/** Renders the league standings table with W-L-T, GB, PF, PA, and streak. */
export function StandingsTab({ leagueId, active, yahooKeyToId }: Props) {
  const { data: standings, error } = useStandings(leagueId, active)

  if (error) return <p className="text-destructive text-sm">{(error as Error).message}</p>
  if (standings === undefined) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  if (standings.length === 0) return <p className="text-muted-foreground">No standings data available.</p>

  const leader = standings[0]
  const gamesBack = (s: typeof standings[0]) => {
    const raw = ((leader.wins - s.wins) + (s.losses - leader.losses)) / 2
    if (raw === 0) return '—'
    return raw % 1 === 0 ? String(raw) : raw.toFixed(1)
  }

  // Yahoo doesn't return PF/PA/streak for H2H category or roto leagues — hide those columns when no data exists.
  const hasPF     = standings.some((s) => !!s.points_for)
  const hasPA     = standings.some((s) => !!s.points_against)
  const hasStreak = standings.some((s) => s.streak_value > 0)

  return (
    <>
      {/* Card list below md — face: Rank, Team, W-L-T; expansion: GB, PF, PA,
          Streak (only the columns that already conditionally render). */}
      <div className="space-y-2 md:hidden">
        {standings.map((s) => {
          const expanded: MobileStatField[] = [{ label: 'GB', value: gamesBack(s) }]
          if (hasPF) expanded.push({ label: 'PF', value: s.points_for })
          if (hasPA) expanded.push({ label: 'PA', value: s.points_against })
          if (hasStreak) {
            expanded.push({
              label: 'Streak',
              value: s.streak_value > 0 && s.streak_type
                ? `${s.streak_value}${s.streak_type === 'win' ? 'W' : 'L'}`
                : '—',
            })
          }
          const teamId = yahooKeyToId[s.team_key]
          return (
            <MobileStatCard
              key={s.team_key}
              href={teamId ? `/teams/${teamId}` : undefined}
              leading={
                s.logo_url ? (
                  <img src={s.logo_url} alt={s.name} width={28} height={28} className="h-7 w-7 rounded object-contain" />
                ) : (
                  <span className="flex h-7 w-7 items-center justify-center rounded bg-muted font-mono text-xs text-muted-foreground">
                    {s.rank}
                  </span>
                )
              }
              title={s.name}
              subtitle={`Rank #${s.rank}`}
              face={
                <span className="font-mono text-xs tabular-nums text-foreground">
                  {s.wins}-{s.losses}{s.ties > 0 ? `-${s.ties}` : ''}
                </span>
              }
              expanded={expanded}
            />
          )
        })}
      </div>

      <div className="hidden md:block rounded-lg bg-card">
      <Table>
        <TableHeader>
          <HeaderRow>
            <TableHead className="w-8">#</TableHead>
            <TableHead>Team</TableHead>
            <TableHead className="text-center"><HeaderTip description="Wins–Losses–Ties">W-L-T</HeaderTip></TableHead>
            <TableHead className="text-right"><HeaderTip description="Games Back — how far behind the leader in the standings">GB</HeaderTip></TableHead>
            {hasPF     && <TableHead className="text-right"><HeaderTip description="Points For — total points scored this season">PF</HeaderTip></TableHead>}
            {hasPA     && <TableHead className="text-right"><HeaderTip description="Points Against — total points allowed this season">PA</HeaderTip></TableHead>}
            {hasStreak && <TableHead className="text-right"><HeaderTip description="Current winning or losing streak">Streak</HeaderTip></TableHead>}
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {standings.map((s) => (
            <TableRow key={s.team_key}>
              <TableCell className="text-muted-foreground">{s.rank}</TableCell>
              <TableCell className="font-medium">
                <div className="flex items-center gap-2">
                  {s.logo_url && (
                    <img src={s.logo_url} alt={s.name} width={24} height={24} className="h-6 w-6 rounded object-contain shrink-0" />
                  )}
                  {yahooKeyToId[s.team_key] ? (
                    <RouterLink to={`/teams/${yahooKeyToId[s.team_key]}`} className="hover:underline">
                      {s.name}
                    </RouterLink>
                  ) : (
                    s.name
                  )}
                </div>
              </TableCell>
              <TableCell className="text-center font-mono tabular-nums">
                {s.wins}-{s.losses}{s.ties > 0 ? `-${s.ties}` : ''}
              </TableCell>
              <TableCell className="text-right font-mono tabular-nums text-muted-foreground">
                {gamesBack(s)}
              </TableCell>
              {hasPF     && <TableCell className="text-right font-mono tabular-nums">{s.points_for}</TableCell>}
              {hasPA     && <TableCell className="text-right font-mono tabular-nums">{s.points_against}</TableCell>}
              {hasStreak && (
                <TableCell className="text-right text-xs text-muted-foreground font-mono tabular-nums">
                  {s.streak_value > 0 && s.streak_type
                    ? `${s.streak_value}${s.streak_type === 'win' ? 'W' : 'L'}`
                    : '—'}
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>
      </div>
    </>
  )
}
