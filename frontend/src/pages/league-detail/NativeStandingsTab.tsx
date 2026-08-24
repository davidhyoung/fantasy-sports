import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { HeaderRow, TeamAvatar } from '@/components/ui/table-helpers'
import { MobileStatCard } from '@/components/ui/mobile-stat-card'
import { getLeagueStandings, type Standing } from '@/api/client'
import { keys } from '@/api/queryKeys'

/** Games back of the leader — derivable from wins/losses alone, so no
 *  backend field for it; only the mobile card's expand panel shows it
 *  (desktop's table never grew a GB column). "—" for the leader itself. */
function gamesBack(s: Standing, leader: Standing): string {
  const gb = ((leader.wins - s.wins) + (s.losses - leader.losses)) / 2
  if (gb <= 0) return '—'
  return Number.isInteger(gb) ? String(gb) : gb.toFixed(1)
}

interface Props {
  leagueId: number
  active: boolean
  /** The signed-in user's own team, if they've claimed one — tints that row
   *  instead of using an accent color, which would misread as "winning". */
  myTeamId?: number
}

const RESULT_CLASS: Record<string, string> = {
  W: 'bg-primary',
  L: 'bg-negative',
  T: 'bg-muted-foreground',
}

/** Five small squares, win/loss(/tie) only — no sparkline of point totals,
 *  since the system has no chart precedent and a W/L strip answers "hot or
 *  cold" at a glance (design review, 2026-08-21). */
function Last5Strip({ results }: { results?: string[] }) {
  if (!results || results.length === 0) return <span className="text-muted-foreground">—</span>
  return (
    <div className="flex items-center justify-center gap-[3px]">
      {results.map((r, i) => (
        <span key={i} className={`h-[6px] w-[6px] rounded-[1px] ${RESULT_CLASS[r] ?? 'bg-muted-foreground'}`} />
      ))}
    </div>
  )
}

/**
 * Standings for a native league — a pure read over scored league_matchups
 * (see nativeStandings in scoring.go), so this only has data once at least
 * one week has been scored. No playoff seeding — regular season only, see
 * .claude/plans/native-leagues.md's weekly-play scope notes. The dashed
 * playoff-line border marks a size-based cutoff heuristic (no real
 * playoff-teams setting exists), not a real bracket.
 */
export function NativeStandingsTab({ leagueId, active, myTeamId }: Props) {
  const { data: standings, isFetching } = useQuery({
    queryKey: keys.standings(leagueId),
    queryFn: () => getLeagueStandings(leagueId),
    enabled: active,
  })

  if (!active) return null

  if (isFetching) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  if (!standings || standings.length === 0) {
    return <p className="text-muted-foreground">No standings yet — generate a schedule and score a week from the Scoreboard tab.</p>
  }

  const leader = standings[0]

  return (
    <>
      {/* Card-per-row below md — face is Team + Record + L5 (the same three
       *  things the desktop table leads with), GB/PF/PA behind the chevron.
       *  The playoff line becomes a plain divider between cards rather than
       *  a row border, since cards don't share edges the way table rows do. */}
      <div className="space-y-2 md:hidden">
        {standings.flatMap((s, i) => {
          const isCutoffRow = !!s.makes_playoffs && (i === standings.length - 1 || !standings[i + 1]?.makes_playoffs)
          const isMe = myTeamId != null && s.team_id === myTeamId
          const card = (
            <MobileStatCard
              key={s.team_id ?? s.team_key}
              href={s.team_id ? `/leagues/${leagueId}?tab=roster&team=${s.team_id}` : undefined}
              leading={
                <div className="flex items-center gap-2">
                  <span className="w-4 font-mono text-xs text-muted-foreground">{s.rank}</span>
                  <TeamAvatar name={s.name} />
                </div>
              }
              title={s.name}
              subtitle={isMe ? 'you' : undefined}
              face={
                <div className="flex items-center gap-3">
                  <Last5Strip results={s.last5} />
                  <span className="font-mono text-sm tabular-nums text-foreground">{s.wins}-{s.losses}-{s.ties}</span>
                </div>
              }
              expanded={[
                { label: 'GB', value: gamesBack(s, leader) },
                { label: 'PF', value: s.points_for },
                { label: 'PA', value: s.points_against },
              ]}
              className={isMe ? 'bg-muted' : ''}
            />
          )
          return isCutoffRow ? [card, <div key={`${s.team_id}-cutoff`} className="border-b border-dashed border-primary/35" />] : [card]
        })}
      </div>

      <div className="hidden md:block rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
        <Table>
          <TableHeader style={{ top: 0 }}>
            <HeaderRow>
              <TableHead>#</TableHead>
              <TableHead>Team</TableHead>
              <TableHead>W-L-T</TableHead>
              <TableHead className="text-center">L5</TableHead>
              <TableHead className="text-right">PF</TableHead>
              <TableHead className="text-right">PA</TableHead>
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {standings.map((s, i) => {
              const isCutoffRow = !!s.makes_playoffs && (i === standings.length - 1 || !standings[i + 1]?.makes_playoffs)
              const isMe = myTeamId != null && s.team_id === myTeamId
              return (
                <TableRow
                  key={s.team_id ?? s.team_key}
                  className={`${isMe ? 'bg-muted' : ''} ${isCutoffRow ? 'border-b border-dashed border-primary/35' : ''}`}
                >
                  <TableCell className="font-mono tabular-nums">{s.rank}</TableCell>
                  <TableCell className="font-medium">
                    <div className="flex items-center gap-2">
                      <TeamAvatar name={s.name} />
                      {s.team_id ? (
                        <RouterLink to={`/leagues/${leagueId}?tab=roster&team=${s.team_id}`} className="hover:text-primary">{s.name}</RouterLink>
                      ) : (
                        s.name
                      )}
                      {isMe && <span className="font-mono text-[11px] text-muted-foreground">you</span>}
                    </div>
                  </TableCell>
                  <TableCell className="font-mono tabular-nums">{s.wins}-{s.losses}-{s.ties}</TableCell>
                  <TableCell>
                    <Last5Strip results={s.last5} />
                  </TableCell>
                  <TableCell className="text-right font-mono tabular-nums">{s.points_for}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums">{s.points_against}</TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </>
  )
}
