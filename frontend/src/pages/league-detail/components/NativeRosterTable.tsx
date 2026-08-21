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

// A drag payload is just the gsis_id being moved — text/plain is enough and
// keeps this readable from a browser's native drag inspector if anything
// ever goes wrong.
const DRAG_MIME = 'text/plain'

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
 * The Slot cell has two ways to change it: an inline `<select>` (works
 * everywhere, including touch and keyboard) and dragging a player's row onto
 * another row (desktop only — native HTML5 drag-and-drop has no touch
 * story, so the dropdown stays as the one mechanism that always works).
 * Either way it's the same lineup-setting mechanism: there's no separate
 * "Lineup" screen, moving a player between a starter slot and BN/TAXI/IR
 * here is exactly what ScoreLeagueWeek reads when a week gets scored.
 * Dropping onto an empty slot just reassigns the dragged player; dropping
 * onto an occupied one swaps the two players' slots, matching how drag-and-
 * drop lineups behave elsewhere (Yahoo, ESPN) rather than silently doubling
 * up a slot.
 */
export function NativeRosterTable({ leagueId, roster, slots = {}, onEdit }: Props) {
  const qc = useQueryClient()
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)

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

  const moveMutation = useMutation({
    mutationFn: async ({
      draggedGsisId, draggedSlot, targetSlot, targetGsisId,
    }: { draggedGsisId: string; draggedSlot: string; targetSlot: string; targetGsisId?: string }) => {
      if (targetGsisId) {
        // Swap: the player who was in the target slot takes the dragged
        // player's old slot, rather than leaving a duplicate at the target.
        await updateLeagueRoster(leagueId, targetGsisId, { slot: draggedSlot })
      }
      await updateLeagueRoster(leagueId, draggedGsisId, { slot: targetSlot })
    },
    onSuccess: invalidate,
  })

  const rows = useMemo(() => buildRows(roster, slots), [roster, slots])

  if (rows.length === 0) {
    return <p className="text-muted-foreground">No roster spots configured yet.</p>
  }

  const dragDisabled = moveMutation.isPending || slotMutation.isPending

  const handleDrop = (e: React.DragEvent<HTMLTableRowElement>, targetSlot: string, targetGsisId: string | undefined) => {
    e.preventDefault()
    setDragOverKey(null)
    const draggedGsisId = e.dataTransfer.getData(DRAG_MIME)
    if (!draggedGsisId || draggedGsisId === targetGsisId) return
    const dragged = roster.find((r) => r.gsis_id === draggedGsisId)
    if (!dragged) return
    moveMutation.mutate({ draggedGsisId, draggedSlot: dragged.slot, targetSlot, targetGsisId })
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
            const rowKey = r ? r.gsis_id : `${row.slot}-${i}`
            const dropTargetProps = {
              onDragOver: (e: React.DragEvent<HTMLTableRowElement>) => {
                e.preventDefault()
                e.dataTransfer.dropEffect = 'move'
                setDragOverKey(rowKey)
              },
              onDragLeave: () => setDragOverKey((k) => (k === rowKey ? null : k)),
              onDrop: (e: React.DragEvent<HTMLTableRowElement>) => handleDrop(e, row.slot, r?.gsis_id),
            }
            const highlight = dragOverKey === rowKey ? 'bg-muted' : ''

            if (!r) {
              return (
                <TableRow key={rowKey} className={highlight} {...dropTargetProps}>
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
              <ClickableRow
                key={r.gsis_id}
                href={`/players/${r.gsis_id}`}
                className={`${highlight} ${dragDisabled ? '' : 'cursor-grab active:cursor-grabbing'}`}
                draggable={!dragDisabled}
                onDragStart={(e) => {
                  e.dataTransfer.setData(DRAG_MIME, r.gsis_id)
                  e.dataTransfer.effectAllowed = 'move'
                }}
                {...dropTargetProps}
              >
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
