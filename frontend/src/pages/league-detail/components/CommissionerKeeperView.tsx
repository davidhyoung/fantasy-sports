import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Loader2, Download, CheckCircle2, Clock } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { HeaderRow } from '@/components/ui/table-helpers'
import { getKeeperSummary, submitKeepers, unsubmitKeepers } from '../../../api/client'
import type { Team } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

interface Props {
  leagueId: number
  myTeam: Team | undefined
  active: boolean
}

function exportToCsv(leagueId: number, rows: ReturnType<typeof buildCsvRows>) {
  const header = 'Team,Player,Position,Draft Cost,Keeper Cost (est.),Years Kept'
  const body = rows.map((r) =>
    [r.team, r.player, r.position, r.draftCost, r.keeperCost, r.yearsKept]
      .map((v) => `"${String(v).replace(/"/g, '""')}"`)
      .join(',')
  ).join('\n')

  const blob = new Blob([header + '\n' + body], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `keepers-league-${leagueId}.csv`
  a.click()
  URL.revokeObjectURL(url)
}

function buildCsvRows(summary: ReturnType<typeof getKeeperSummary> extends Promise<infer T> ? T : never) {
  return summary.flatMap((team) =>
    team.keepers.map((k) => ({
      team: team.team_name,
      player: k.player_name,
      position: k.position ?? '',
      draftCost: k.draft_cost ?? 0,
      keeperCost: '',   // cost calculation lives in the draft results; omit here
      yearsKept: k.years_kept,
    }))
  )
}

export function CommissionerKeeperView({ leagueId, myTeam, active }: Props) {
  const qc = useQueryClient()

  const { data: summary, isLoading } = useQuery({
    queryKey: keys.keeperSummary(leagueId),
    queryFn: () => getKeeperSummary(leagueId),
    enabled: active,
  })

  const submitMutation = useMutation({
    mutationFn: (teamId: number) => submitKeepers(teamId),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.keeperSummary(leagueId) }),
  })
  const unsubmitMutation = useMutation({
    mutationFn: (teamId: number) => unsubmitKeepers(teamId),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.keeperSummary(leagueId) }),
  })

  if (isLoading) return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
  if (!summary) return null

  const submittedCount = summary.filter((t) => t.submitted).length
  const totalCount = summary.length
  const csvRows = buildCsvRows(summary)

  const myEntry = myTeam ? summary.find((t) => t.team_id === myTeam.id) : undefined
  const mySubmitted = myEntry?.submitted ?? false

  return (
    <div className="space-y-4">
      {/* Header row */}
      <div className="flex items-center justify-between flex-wrap gap-3">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-semibold text-foreground">Commissioner View</h3>
          <span className={`text-xs font-medium px-2 py-0.5 rounded-full border ${
            submittedCount === totalCount
              ? 'bg-green-100 text-green-700 border-green-300 dark:bg-green-900/30 dark:text-green-400 dark:border-green-700'
              : 'bg-muted text-muted-foreground border-border'
          }`}>
            {submittedCount} / {totalCount} submitted
          </span>
        </div>
        <div className="flex items-center gap-2">
          {/* Team owner submit/unsubmit their own keepers */}
          {myTeam && (
            mySubmitted ? (
              <Button
                size="sm"
                variant="outline"
                disabled={unsubmitMutation.isPending}
                onClick={() => unsubmitMutation.mutate(myTeam.id)}
              >
                {unsubmitMutation.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Unsubmit My Keepers'}
              </Button>
            ) : (
              <Button
                size="sm"
                disabled={submitMutation.isPending}
                onClick={() => submitMutation.mutate(myTeam.id)}
              >
                {submitMutation.isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Submit My Keepers'}
              </Button>
            )
          )}
          <Button
            size="sm"
            variant="outline"
            onClick={() => exportToCsv(leagueId, csvRows)}
            disabled={csvRows.length === 0}
          >
            <Download className="h-3 w-3 mr-1" />
            Export CSV
          </Button>
        </div>
      </div>

      {/* Per-team keeper summary */}
      <div className="rounded-lg bg-card">
        <Table>
          <TableHeader>
            <HeaderRow>
              <TableHead>Team</TableHead>
              <TableHead className="text-center w-24">Status</TableHead>
              <TableHead>Keepers</TableHead>
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {summary.map((entry) => (
              <TableRow key={entry.team_id} className={entry.team_id === myTeam?.id ? 'bg-muted/20' : ''}>
                {/* Team */}
                <TableCell className="font-medium">
                  <div className="flex items-center gap-2">
                    {entry.logo_url && (
                      <img src={entry.logo_url} alt={entry.team_name} className="h-6 w-6 rounded object-contain shrink-0" />
                    )}
                    <span>{entry.team_name}</span>
                    {entry.team_id === myTeam?.id && (
                      <span className="text-xs text-muted-foreground">(you)</span>
                    )}
                  </div>
                </TableCell>

                {/* Submitted badge */}
                <TableCell className="text-center">
                  {entry.submitted ? (
                    <div className="flex flex-col items-center gap-0.5">
                      <Badge className="bg-green-100 text-green-700 border-green-300 dark:bg-green-900/40 dark:text-green-300 dark:border-green-700 text-xs gap-1">
                        <CheckCircle2 className="h-3 w-3" />
                        Submitted
                      </Badge>
                      {entry.submitted_at && (
                        <span className="text-xs text-muted-foreground">
                          {new Date(entry.submitted_at).toLocaleDateString()}
                        </span>
                      )}
                    </div>
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground gap-1 text-xs">
                      <Clock className="h-3 w-3" />
                      Pending
                    </Badge>
                  )}
                </TableCell>

                {/* Keeper list */}
                <TableCell>
                  {entry.keepers.length === 0 ? (
                    <span className="text-xs text-muted-foreground italic">No keepers selected</span>
                  ) : (
                    <div className="flex flex-wrap gap-2">
                      {entry.keepers.map((k) => (
                        <span
                          key={k.player_key}
                          className="inline-flex items-center gap-1 text-xs bg-muted rounded px-2 py-0.5"
                        >
                          <span className="font-medium">{k.player_name}</span>
                          {k.position && (
                            <span className="text-muted-foreground">{k.position}</span>
                          )}
                          {k.draft_cost != null && (
                            <span className="text-muted-foreground">${k.draft_cost}</span>
                          )}
                        </span>
                      ))}
                    </div>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
