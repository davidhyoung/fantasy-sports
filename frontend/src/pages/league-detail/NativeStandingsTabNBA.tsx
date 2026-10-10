import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { HeaderRow, TeamAvatar } from '@/components/ui/table-helpers'
import { getLeagueStandingsNBA } from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  leagueId: number
  active: boolean
  myTeamId?: number
}

const RESULT_CLASS: Record<string, string> = {
  W: 'bg-primary',
  L: 'bg-negative',
  T: 'bg-muted-foreground',
}

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
 * Standings for a native NBA league — match record (majority of categories
 * won each week) first, cumulative category record as the tiebreaker (the
 * standard roto-league convention — see GetLeagueStandingsNBA). A pure read
 * over scored league_matchups_nba, same "no separate standings table"
 * approach as the NFL side.
 */
export function NativeStandingsTabNBA({ leagueId, active, myTeamId }: Props) {
  const { data: standings, isFetching } = useQuery({
    queryKey: keys.standingsNBA(leagueId),
    queryFn: () => getLeagueStandingsNBA(leagueId),
    enabled: active,
  })

  if (!active) return null
  if (isFetching) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  if (!standings || standings.length === 0) {
    return <p className="text-muted-foreground">No standings yet — generate a schedule and score a week from the Scoreboard tab.</p>
  }

  return (
    <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead>#</TableHead>
            <TableHead>Team</TableHead>
            <TableHead>W-L-T</TableHead>
            <TableHead className="text-center">L5</TableHead>
            <TableHead className="text-right">Cat record</TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {standings.map((s) => {
            const isMe = myTeamId != null && s.team_id === myTeamId
            return (
              <TableRow key={s.team_id} className={isMe ? 'bg-muted' : ''}>
                <TableCell className="font-mono tabular-nums">{s.rank}</TableCell>
                <TableCell className="font-medium">
                  <div className="flex items-center gap-2">
                    <TeamAvatar name={s.name} />
                    <RouterLink to={`/leagues/${leagueId}?tab=roster&team=${s.team_id}`} className="hover:text-primary">
                      {s.name}
                    </RouterLink>
                    {isMe && <span className="font-mono text-[11px] text-muted-foreground">you</span>}
                  </div>
                </TableCell>
                <TableCell className="font-mono tabular-nums">{s.wins}-{s.losses}-{s.ties}</TableCell>
                <TableCell><Last5Strip results={s.last5} /></TableCell>
                <TableCell className="text-right font-mono tabular-nums">
                  {s.category_wins}-{s.category_losses}
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
