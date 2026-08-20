import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { HeaderRow } from '@/components/ui/table-helpers'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { getLeagueDraftPicks, generateLeagueDraftPicks, type Team, type LeagueDraftPick } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { PlayerAssignForm } from './components/PlayerAssignForm'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  season: string
}

/**
 * Future draft picks — the dynasty-specific tradable asset. Rounds-by-season
 * grouping mirrors how a commissioner actually thinks about picks ("what do
 * I have in 2027"), not a flat list.
 */
export function NativeDraftPicksTab({ leagueId, active, teams, season }: Props) {
  const qc = useQueryClient()
  const [using, setUsing] = useState<LeagueDraftPick | null>(null)

  const { data: picks, isFetching } = useQuery({
    queryKey: keys.leaguePicks(leagueId),
    queryFn: () => getLeagueDraftPicks(leagueId),
    enabled: active,
  })

  const nextSeason = (parseInt(season, 10) || new Date().getFullYear()) + 1

  const generateMutation = useMutation({
    mutationFn: () => generateLeagueDraftPicks(leagueId, nextSeason),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.leaguePicks(leagueId) }),
  })

  const bySeason = useMemo(() => {
    const groups = new Map<number, LeagueDraftPick[]>()
    for (const p of picks ?? []) {
      if (!groups.has(p.season)) groups.set(p.season, [])
      groups.get(p.season)!.push(p)
    }
    return [...groups.entries()].sort((a, b) => a[0] - b[0])
  }, [picks])

  const hasUpcomingClass = (picks ?? []).some((p) => p.season === nextSeason)

  if (!active) return null

  return (
    <>
      <div className="flex items-center justify-between mb-4">
        <p className="text-sm text-muted-foreground">
          Future draft picks, tradable like players. Draft order is unset until standings exist — picks are unordered within a round.
        </p>
        {!hasUpcomingClass && (
          <Button size="sm" onClick={() => generateMutation.mutate()} disabled={generateMutation.isPending}>
            {generateMutation.isPending ? 'Generating…' : `Generate ${nextSeason} class`}
          </Button>
        )}
      </div>
      {generateMutation.error && (
        <p className="mb-3 text-sm text-destructive">{(generateMutation.error as Error).message}</p>
      )}

      {isFetching ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : bySeason.length === 0 ? (
        <p className="text-muted-foreground">No draft picks generated yet.</p>
      ) : (
        <div className="flex flex-col gap-6">
          {bySeason.map(([s, rows]) => (
            <div key={s}>
              <div className="mb-2 font-display text-sm font-bold text-foreground">{s}</div>
              <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
                <Table>
                  <TableHeader style={{ top: 0 }}>
                    <HeaderRow>
                      <TableHead>Round</TableHead>
                      <TableHead>Owned by</TableHead>
                      <TableHead>Original owner</TableHead>
                      <TableHead>Used on</TableHead>
                      <TableHead />
                    </HeaderRow>
                  </TableHeader>
                  <TableBody>
                    {rows
                      .sort((a, b) => a.round - b.round || a.current_team_name.localeCompare(b.current_team_name))
                      .map((p) => (
                        <TableRow key={p.id}>
                          <TableCell className="font-mono tabular-nums">{p.round}</TableCell>
                          <TableCell className="text-muted-foreground">{p.current_team_name}</TableCell>
                          <TableCell className="text-muted-foreground">
                            {p.original_team_id !== p.current_team_id ? p.original_team_name : '—'}
                          </TableCell>
                          <TableCell className="text-muted-foreground">{p.used_on_name ?? '—'}</TableCell>
                          <TableCell className="text-right">
                            {!p.used_on_gsis_id && (
                              <Button variant="text" size="bare" onClick={() => setUsing(p)}>Use pick</Button>
                            )}
                          </TableCell>
                        </TableRow>
                      ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          ))}
        </div>
      )}

      <Dialog open={using != null} onClose={() => setUsing(null)} title="Use draft pick">
        {using && (
          <PlayerAssignForm
            leagueId={leagueId}
            teams={teams}
            mode="pick"
            pickId={using.id}
            teamId={using.current_team_id}
            onClose={() => setUsing(null)}
          />
        )}
      </Dialog>
    </>
  )
}
