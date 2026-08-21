import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { Button } from '@/components/ui/button'
import { updateLeagueRoster, dropLeagueRoster, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { ROSTER_SLOTS, SLOT_DISPLAY_ORDER } from '../lib/nativeSlots'

interface Props {
  leagueId: number
  roster: RosterEntry[]
  /** The league's configured slot counts (e.g. {QB:1,RB:2,...}) — what turns
   *  a list of rostered players into the full lineup shape, empty spots and
   *  all. Omit (or pass {}) to just show whoever's actually rostered. */
  slots?: Record<string, number>
  onEdit: (entry: RosterEntry) => void
}

interface Row {
  slot: string
  entry?: RosterEntry
}

/** One row per configured slot, filled with whichever rostered player (if
 *  any) currently occupies it — padded with empty rows up to the league's
 *  configured count, and never hiding an actual player even if the league's
 *  slot counts changed since they were assigned (a slot with 3 rostered
 *  players against a configured count of 2 still shows all 3). */
function buildRows(roster: RosterEntry[], slots: Record<string, number>): Row[] {
  const bySlot = new Map<string, RosterEntry[]>()
  for (const r of roster) {
    if (!bySlot.has(r.slot)) bySlot.set(r.slot, [])
    bySlot.get(r.slot)!.push(r)
  }

  const rows: Row[] = []
  for (const slot of SLOT_DISPLAY_ORDER) {
    const actual = bySlot.get(slot) ?? []
    const count = Math.max(slots[slot] ?? 0, actual.length)
    for (let i = 0; i < count; i++) {
      rows.push({ slot, entry: actual[i] })
    }
    bySlot.delete(slot)
  }
  // Any slot value not in SLOT_DISPLAY_ORDER (shouldn't happen — safety net
  // so an unrecognized slot on a roster row never silently disappears).
  for (const [slot, entries] of bySlot) {
    for (const entry of entries) rows.push({ slot, entry })
  }
  return rows
}

/**
 * Shared roster table for the Roster tab, for whichever team is selected.
 * The Slot cell is an inline `<select>` — that's the actual lineup-setting
 * mechanism for weekly play: there's no separate "Lineup" screen, moving a
 * player between a starter slot and BN/TAXI/IR here is exactly what
 * ScoreLeagueWeek reads when a week gets scored.
 */
export function NativeRosterTable({ leagueId, roster, slots = {}, onEdit }: Props) {
  const qc = useQueryClient()
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
    qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
  }

  const slotMutation = useMutation({
    mutationFn: ({ gsisId, slot }: { gsisId: string; slot: string }) => updateLeagueRoster(leagueId, gsisId, { slot }),
    onSuccess: invalidate,
  })

  const dropMutation = useMutation({
    mutationFn: (gsisId: string) => dropLeagueRoster(leagueId, gsisId),
    onSuccess: () => {
      invalidate()
      setConfirmDrop(null)
    },
  })

  const rows = useMemo(() => buildRows(roster, slots), [roster, slots])

  if (rows.length === 0) {
    return <p className="text-muted-foreground">No roster spots configured yet.</p>
  }

  return (
    <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead>Player</TableHead>
            <TableHead>Pos</TableHead>
            <TableHead>Slot</TableHead>
            <TableHead className="text-right">Salary</TableHead>
            <TableHead className="text-right">Years</TableHead>
            <TableHead />
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {rows.map((row, i) => {
            const r = row.entry
            if (!r) {
              return (
                <TableRow key={`${row.slot}-${i}`}>
                  <TableCell className="italic text-muted-foreground">Empty</TableCell>
                  <TableCell className="text-muted-foreground">—</TableCell>
                  <TableCell className="text-muted-foreground">{row.slot}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
                  <TableCell className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
                  <TableCell />
                </TableRow>
              )
            }
            return (
              <ClickableRow key={r.gsis_id} href={`/players/${r.gsis_id}`}>
                <PlayerCell name={r.name} imageUrl={r.headshot_url} linked />
                <TableCell className="text-muted-foreground">{r.position}</TableCell>
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <select
                    value={r.slot}
                    onChange={(e) => slotMutation.mutate({ gsisId: r.gsis_id, slot: e.target.value })}
                    disabled={slotMutation.isPending}
                    className="h-7 rounded-md border border-input bg-background px-1.5 font-display text-xs font-semibold text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {ROSTER_SLOTS.map((s) => <option key={s} value={s}>{s}</option>)}
                  </select>
                </TableCell>
                <TableCell className="text-right font-mono tabular-nums">${r.salary}</TableCell>
                <TableCell className="text-right font-mono tabular-nums">
                  {r.years_total != null ? `${r.years_used}/${r.years_total}` : 'Y2Y'}
                </TableCell>
                <TableCell className="text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                  <Button variant="text" size="bare" onClick={() => onEdit(r)}>Edit</Button>
                  {confirmDrop === r.gsis_id ? (
                    <Button
                      variant="destructive" size="sm" className="ml-2"
                      onClick={() => dropMutation.mutate(r.gsis_id)}
                      disabled={dropMutation.isPending}
                    >
                      Confirm drop
                    </Button>
                  ) : (
                    <Button variant="text" size="bare" className="ml-2 text-negative" onClick={() => setConfirmDrop(r.gsis_id)}>
                      Drop
                    </Button>
                  )}
                </TableCell>
              </ClickableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
