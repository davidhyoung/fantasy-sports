import { useQuery } from '@tanstack/react-query'
import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { HeaderRow } from '@/components/ui/table-helpers'
import { getLeagueStandings } from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  leagueId: number
  active: boolean
}

/**
 * Standings for a native league — a pure read over scored league_matchups
 * (see nativeStandings in scoring.go), so this only has data once at least
 * one week has been scored. No playoff seeding — regular season only, see
 * .claude/plans/native-leagues.md's weekly-play scope notes.
 */
export function NativeStandingsTab({ leagueId, active }: Props) {
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

  return (
    <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead>#</TableHead>
            <TableHead>Team</TableHead>
            <TableHead>W-L-T</TableHead>
            <TableHead className="text-right">PF</TableHead>
            <TableHead className="text-right">PA</TableHead>
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {standings.map((s) => (
            <TableRow key={s.team_id ?? s.team_key}>
              <TableCell className="font-mono tabular-nums">{s.rank}</TableCell>
              <TableCell className="font-medium">
                {s.team_id ? (
                  <RouterLink to={`/leagues/${leagueId}?tab=roster&team=${s.team_id}`} className="hover:text-primary">{s.name}</RouterLink>
                ) : (
                  s.name
                )}
              </TableCell>
              <TableCell className="font-mono tabular-nums">{s.wins}-{s.losses}-{s.ties}</TableCell>
              <TableCell className="text-right font-mono tabular-nums">{s.points_for}</TableCell>
              <TableCell className="text-right font-mono tabular-nums">{s.points_against}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
