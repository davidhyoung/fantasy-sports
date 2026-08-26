import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { TeamAvatar, HeaderRow } from '@/components/ui/table-helpers'
import { SelectControl } from '@/components/ui/filter-chip'
import { Button } from '@/components/ui/button'
import { ResponsiveDialog } from '@/components/ui/responsive-dialog'
import {
  listFAWindows, openFAWindow, resolveFAWindow, listFAOffers, withdrawFAOffer, reorderFAOffers,
  type Team, type FAOffer,
} from '@/api/client'
import { keys } from '@/api/queryKeys'
import { FAOfferForm } from './components/FAOfferForm'

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  myTeam: Team | undefined
}

/**
 * Free agency — no FAAB, the salary cap is the only currency. Teams offer
 * contracts against a ranked priority list; the commissioner opens a
 * window, teams submit/edit/withdraw/reorder offers while it's open, and
 * resolving it (an explicit action, never a cron — matching
 * ScoreLeagueWeek, since nothing in this app's pipeline is genuinely live)
 * signs the best offer on every player with one, best market value first.
 * See free_agency.go / dynasty-transactions.md.
 */
export function NativeFreeAgencyTab({ leagueId, active, teams, myTeam }: Props) {
  const qc = useQueryClient()
  // Derived, not frozen at mount — `teams` loads asynchronously, so a plain
  // useState default here would freeze on 0 before real teams are known
  // (the same bug class NativeRosterTab's team selection was fixed to avoid
  // by deriving from the URL instead of useState).
  const [teamOverride, setTeamOverride] = useState<number | null>(null)
  const teamId = teamOverride != null && teams.some((t) => t.id === teamOverride)
    ? teamOverride
    : myTeam?.id ?? teams[0]?.id ?? 0
  const setTeamId = setTeamOverride
  const [offering, setOffering] = useState(false)
  const [confirmResolve, setConfirmResolve] = useState(false)
  const [openKind, setOpenKind] = useState<'offseason' | 'weekly'>('offseason')

  const { data: windows = [] } = useQuery({
    queryKey: keys.faWindows(leagueId),
    queryFn: () => listFAWindows(leagueId),
    enabled: active,
  })
  const openWindow = windows.find((w) => !w.resolved_at)

  const { data: offers = [] } = useQuery({
    queryKey: keys.faOffers(leagueId, openWindow?.id),
    queryFn: () => listFAOffers(leagueId, openWindow!.id),
    enabled: active && !!openWindow,
  })

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: keys.faWindows(leagueId) })
    qc.invalidateQueries({ queryKey: keys.faOffers(leagueId) })
    qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
    qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'team'] })
  }

  const openMutation = useMutation({
    mutationFn: () => openFAWindow(leagueId, { kind: openKind }),
    onSuccess: invalidate,
  })
  const resolveMutation = useMutation({
    mutationFn: () => resolveFAWindow(leagueId, openWindow!.id),
    // Stay open on success so the signed-players summary below can render —
    // the dialog's own Close button (not this handler) dismisses it.
    onSuccess: invalidate,
  })
  const withdrawMutation = useMutation({
    mutationFn: (offerId: number) => withdrawFAOffer(leagueId, offerId),
    onSuccess: invalidate,
  })
  const reorderMutation = useMutation({
    mutationFn: (offerIds: number[]) => reorderFAOffers(leagueId, teamId, offerIds),
    onSuccess: invalidate,
  })

  const myOffers = offers.filter((o) => o.team_id === teamId).sort((a, b) => a.priority - b.priority)
  const move = (offer: FAOffer, dir: -1 | 1) => {
    const idx = myOffers.findIndex((o) => o.id === offer.id)
    const swapIdx = idx + dir
    if (swapIdx < 0 || swapIdx >= myOffers.length) return
    const ids = myOffers.map((o) => o.id)
    ;[ids[idx], ids[swapIdx]] = [ids[swapIdx], ids[idx]]
    reorderMutation.mutate(ids)
  }

  const teamName = (id: number) => teams.find((t) => t.id === id)?.name ?? `Team ${id}`

  // Everyone's pending offers this window, grouped by player — the full
  // competitive picture, since the single commissioner manages every team
  // and needs to see the whole board before deciding to resolve.
  const byPlayer = new Map<string, FAOffer[]>()
  for (const o of offers.filter((o) => o.status === 'pending')) {
    if (!byPlayer.has(o.gsis_id)) byPlayer.set(o.gsis_id, [])
    byPlayer.get(o.gsis_id)!.push(o)
  }

  if (!active) return null

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-lg bg-card px-4 py-3">
        {openWindow ? (
          <>
            <div>
              <div className="font-display text-sm font-semibold text-foreground">
                Free agency is open <span className="text-muted-foreground font-normal">({openWindow.kind})</span>
              </div>
              <p className="text-xs text-muted-foreground">
                Opened {new Date(openWindow.opened_at).toLocaleDateString()}. Teams can submit, edit, reorder, or
                withdraw offers until you resolve it.
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => setConfirmResolve(true)}>Resolve free agency</Button>
          </>
        ) : (
          <>
            <div>
              <div className="font-display text-sm font-semibold text-foreground">No free agency window is open</div>
              <p className="text-xs text-muted-foreground">Open one to start collecting offers.</p>
            </div>
            <div className="flex items-center gap-2">
              <SelectControl value={openKind} onChange={(e) => setOpenKind(e.target.value as 'offseason' | 'weekly')}>
                <option value="offseason">Offseason</option>
                <option value="weekly">Weekly</option>
              </SelectControl>
              <Button size="sm" onClick={() => openMutation.mutate()} disabled={openMutation.isPending}>
                {openMutation.isPending ? 'Opening…' : 'Open free agency'}
              </Button>
            </div>
          </>
        )}
      </div>
      {openMutation.error && <p className="mb-3 text-sm text-destructive">{(openMutation.error as Error).message}</p>}
      {resolveMutation.error && <p className="mb-3 text-sm text-destructive">{(resolveMutation.error as Error).message}</p>}

      {openWindow && (
        <>
          <div className="mb-4 flex items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              {teams.find((t) => t.id === teamId) && <TeamAvatar name={teamName(teamId)} />}
              <SelectControl value={teamId} onChange={(e) => setTeamId(Number(e.target.value))}>
                {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </SelectControl>
            </div>
            <Button size="sm" onClick={() => setOffering(true)}>+ Make offer</Button>
          </div>

          <div className="mb-6 rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead className="w-16">Priority</TableHead>
                  <TableHead>Player</TableHead>
                  <TableHead className="text-right">Salary</TableHead>
                  <TableHead className="text-right">Years</TableHead>
                  <TableHead />
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {myOffers.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-sm text-muted-foreground py-6">
                      No pending offers for {teamName(teamId)}.
                    </TableCell>
                  </TableRow>
                ) : (
                  myOffers.map((o, i) => (
                    <TableRow key={o.id}>
                      <TableCell>
                        <div className="flex items-center gap-1 font-mono text-xs tabular-nums text-muted-foreground">
                          {o.priority}
                          <button
                            onClick={() => move(o, -1)}
                            disabled={i === 0}
                            aria-label="Move up"
                            className="disabled:opacity-30"
                          >▲</button>
                          <button
                            onClick={() => move(o, 1)}
                            disabled={i === myOffers.length - 1}
                            aria-label="Move down"
                            className="disabled:opacity-30"
                          >▼</button>
                        </div>
                      </TableCell>
                      <TableCell className="font-medium text-foreground">{o.player_name}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">${o.salary}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums">{o.years}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="text" size="bare"
                          onClick={() => withdrawMutation.mutate(o.id)}
                          disabled={withdrawMutation.isPending}
                        >
                          Withdraw
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          <div className="mb-2 font-display text-sm font-bold text-foreground">All pending offers this window</div>
          <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
            <Table>
              <TableHeader style={{ top: 0 }}>
                <HeaderRow>
                  <TableHead>Player</TableHead>
                  <TableHead>Team</TableHead>
                  <TableHead className="text-right">Salary</TableHead>
                  <TableHead className="text-right">Years</TableHead>
                  <TableHead className="text-right">Priority</TableHead>
                </HeaderRow>
              </TableHeader>
              <TableBody>
                {byPlayer.size === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-center text-sm text-muted-foreground py-6">
                      No pending offers yet.
                    </TableCell>
                  </TableRow>
                ) : (
                  [...byPlayer.entries()].flatMap(([gsisId, os]) =>
                    os.map((o, i) => (
                      <TableRow key={`${gsisId}-${o.id}`}>
                        {i === 0 ? (
                          <TableCell rowSpan={os.length} className="font-medium text-foreground align-top">
                            {o.player_name}
                          </TableCell>
                        ) : null}
                        <TableCell className="text-muted-foreground">{o.team_name}</TableCell>
                        <TableCell className="text-right font-mono tabular-nums">${o.salary}</TableCell>
                        <TableCell className="text-right font-mono tabular-nums">{o.years}</TableCell>
                        <TableCell className="text-right font-mono tabular-nums text-muted-foreground">{o.priority}</TableCell>
                      </TableRow>
                    ))
                  )
                )}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      <ResponsiveDialog open={offering} onClose={() => setOffering(false)} title="Make an offer">
        {offering && <FAOfferForm leagueId={leagueId} teamId={teamId} onClose={() => setOffering(false)} />}
      </ResponsiveDialog>

      <ResponsiveDialog
        open={confirmResolve}
        onClose={() => { setConfirmResolve(false); resolveMutation.reset() }}
        title="Resolve free agency"
      >
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted-foreground">
            Every pending offer resolves now, best player first — the highest-scoring surviving offer on each
            player signs, everyone else loses. This can't be undone.
          </p>
          {resolveMutation.data && (
            <div className="rounded-md bg-muted p-3 text-sm">
              <div className="font-display font-semibold text-foreground mb-1">Signed:</div>
              {resolveMutation.data.signed.length === 0 ? (
                <p className="text-muted-foreground">Nobody signed.</p>
              ) : (
                resolveMutation.data.signed.map((s) => (
                  <div key={s.gsis_id} className="text-muted-foreground">
                    {s.name} → {teamName(s.team_id)} (${s.salary} · {s.years}yr)
                  </div>
                ))
              )}
            </div>
          )}
          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={() => { setConfirmResolve(false); resolveMutation.reset() }}>
              {resolveMutation.data ? 'Close' : 'Cancel'}
            </Button>
            {!resolveMutation.data && (
              <Button onClick={() => resolveMutation.mutate()} disabled={resolveMutation.isPending}>
                {resolveMutation.isPending ? 'Resolving…' : 'Resolve'}
              </Button>
            )}
          </div>
        </div>
      </ResponsiveDialog>
    </>
  )
}
