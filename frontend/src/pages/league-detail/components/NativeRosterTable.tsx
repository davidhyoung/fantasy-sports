import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { Button } from '@/components/ui/button'
import { updateLeagueRoster, dropLeagueRoster, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { SLOT_DISPLAY_ORDER, BENCH_SLOTS, isSlotEligible } from '../lib/nativeSlots'

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
 * Slot is a plain read-only column — the two ways to actually change it are
 * dragging a player's row onto another row (desktop only — native HTML5
 * drag-and-drop has no touch story) and clicking Pos to enter "picking"
 * mode: every row that's a legal destination for that player lights up
 * (left accent edge + pointer cursor) and becomes clickable, so the row
 * itself is the target — not an abstract slot name — which is what lets a
 * league with two RB slots distinguish "the empty one" from "the one
 * Jahmyr's in" (a swap) instead of just setting a slot label blind to which
 * physical row it lands on. Either way it's the same lineup-setting
 * mechanism: there's no separate "Lineup" screen, moving a player between a
 * starter slot and BN/TAXI/IR here is exactly what ScoreLeagueWeek reads
 * when a week gets scored. Landing on an empty row just reassigns the
 * player; landing on an occupied one swaps the two players' slots, matching
 * how drag-and-drop lineups behave elsewhere (Yahoo, ESPN) rather than
 * silently doubling up a slot.
 */
export function NativeRosterTable({ leagueId, roster, slots = {}, onEdit }: Props) {
  const qc = useQueryClient()
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)
  // The dragged player, kept in component state (not just the DataTransfer
  // payload) so onDragOver can check eligibility — browsers don't expose
  // DataTransfer.getData() until the actual drop fires.
  const [draggingEntry, setDraggingEntry] = useState<RosterEntry | null>(null)
  // The player currently being moved via click (Pos was clicked), if any —
  // only one at a time. While set, every legal destination row is
  // highlighted and clickable.
  const [pickingFor, setPickingFor] = useState<string | null>(null)
  const tableRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!pickingFor) return
    const handler = (e: MouseEvent) => {
      if (tableRef.current && !tableRef.current.contains(e.target as Node)) {
        setPickingFor(null)
      }
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [pickingFor])

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

  // Both directions have to clear eligibility: the dragged player has to be
  // allowed in the target slot, and — for a swap onto an occupied row — the
  // occupant has to be allowed in the slot the dragged player is leaving.
  const canDrop = (dragged: RosterEntry, targetSlot: string, targetEntry: RosterEntry | undefined) => {
    if (targetEntry?.gsis_id === dragged.gsis_id) return false
    if (!isSlotEligible(targetSlot, dragged.position)) return false
    if (targetEntry && !isSlotEligible(dragged.slot, targetEntry.position)) return false
    return true
  }

  const handleDrop = (e: React.DragEvent<HTMLTableRowElement>, targetSlot: string, targetEntry: RosterEntry | undefined) => {
    e.preventDefault()
    setDragOverKey(null)
    const draggedGsisId = e.dataTransfer.getData(DRAG_MIME)
    const dragged = roster.find((r) => r.gsis_id === draggedGsisId)
    if (!dragged || !canDrop(dragged, targetSlot, targetEntry)) return
    moveMutation.mutate({ draggedGsisId, draggedSlot: dragged.slot, targetSlot, targetGsisId: targetEntry?.gsis_id })
  }

  const pickedEntry = pickingFor ? roster.find((r) => r.gsis_id === pickingFor) : undefined

  const handlePick = (targetSlot: string, targetEntry: RosterEntry | undefined) => {
    if (!pickedEntry || !canDrop(pickedEntry, targetSlot, targetEntry)) return
    moveMutation.mutate({
      draggedGsisId: pickedEntry.gsis_id, draggedSlot: pickedEntry.slot, targetSlot, targetGsisId: targetEntry?.gsis_id,
    })
    setPickingFor(null)
  }

  return (
    <div ref={tableRef} className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead>Slot</TableHead>
            <TableHead>Player</TableHead>
            <TableHead>Pos</TableHead>
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
                // Not calling preventDefault() here is what makes an
                // ineligible slot refuse the drop (browser default) — the
                // dragged player's gsis_id isn't readable from DataTransfer
                // until the actual drop event, so eligibility here has to
                // come from draggingEntry (component state set at dragstart).
                if (draggingEntry && !canDrop(draggingEntry, row.slot, r)) return
                e.preventDefault()
                e.dataTransfer.dropEffect = 'move'
                setDragOverKey(rowKey)
              },
              onDragLeave: () => setDragOverKey((k) => (k === rowKey ? null : k)),
              onDrop: (e: React.DragEvent<HTMLTableRowElement>) => handleDrop(e, row.slot, r),
            }
            const highlight = dragOverKey === rowKey ? 'bg-muted' : ''
            // While a player is being moved by click (pickedEntry set), every
            // row that's a legal destination for them lights up and becomes
            // clickable — the row itself is the target, so two same-named
            // slots (two RB rows) are distinguishable: one may be empty, one
            // may hold someone else (a swap).
            const isTarget = !!pickedEntry && canDrop(pickedEntry, row.slot, r)
            const pickTargetProps = isTarget ? { onClick: () => handlePick(row.slot, r) } : {}
            const targetHighlight = isTarget ? 'cursor-pointer border-l-[3px] border-l-primary' : ''

            if (!r) {
              return (
                <TableRow key={rowKey} className={`${highlight} ${targetHighlight}`} {...dropTargetProps} {...pickTargetProps}>
                  <TableCell className="font-mono text-xs font-semibold text-muted-foreground">{row.slot}</TableCell>
                  <TableCell className="italic text-muted-foreground">Empty</TableCell>
                  <TableCell className="text-muted-foreground">—</TableCell>
                  <TableCell className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
                  <TableCell className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
                  <TableCell />
                </TableRow>
              )
            }
            return (
              <ClickableRow
                key={r.gsis_id}
                href={isTarget ? undefined : `/players/${r.gsis_id}`}
                className={`${highlight} ${targetHighlight} ${dragDisabled ? '' : 'cursor-grab active:cursor-grabbing'}`}
                draggable={!dragDisabled}
                onDragStart={(e) => {
                  e.dataTransfer.setData(DRAG_MIME, r.gsis_id)
                  e.dataTransfer.effectAllowed = 'move'
                  setDraggingEntry(r)
                }}
                onDragEnd={() => {
                  setDraggingEntry(null)
                  setDragOverKey(null)
                }}
                {...dropTargetProps}
                {...pickTargetProps}
              >
                <TableCell onClick={(e) => e.stopPropagation()}>
                  <button
                    type="button"
                    onClick={() => setPickingFor((k) => (k === r.gsis_id ? null : r.gsis_id))}
                    className={`rounded-md border px-2 py-1 font-mono text-xs font-semibold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${
                      pickingFor === r.gsis_id
                        ? 'border-positive-border bg-positive-light text-primary'
                        : 'border-input bg-background text-foreground hover:bg-accent hover:text-accent-foreground'
                    }`}
                  >
                    {r.slot}
                  </button>
                </TableCell>
                <PlayerCell name={r.name} imageUrl={r.headshot_url} linked />
                <TableCell className="text-muted-foreground">
                  {SLOT_DISPLAY_ORDER.filter((s) => !BENCH_SLOTS.has(s) && isSlotEligible(s, r.position)).join(' · ')}
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
