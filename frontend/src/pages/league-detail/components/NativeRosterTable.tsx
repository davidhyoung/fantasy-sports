import { useEffect, useMemo, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { updateLeagueRoster, dropLeagueRoster, type RosterEntry, type PlayerStat } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { contractYearsLabel } from '@/lib/utils'
import { SLOT_DISPLAY_ORDER, BENCH_SLOTS, DISPLAY_HIDDEN_SLOTS, isSlotEligible } from '../lib/nativeSlots'
import { SCORING_STATS, SCORING_LABELS, type ScoringStat } from '../hooks/useDraftSettings'
import { L4Sparkline } from './L4Sparkline'

function statValue(stats: PlayerStat[] | undefined, stat: ScoringStat): number | undefined {
  return stats?.find((s) => s.stat === stat)?.value
}

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

type Group = 'starters' | 'bench' | 'taxi'

const GROUP_LABEL: Record<Group, string> = {
  starters: 'Starters',
  bench: 'Bench',
  taxi: 'Taxi & IR',
}

function groupOf(slot: string): Group {
  if (slot === 'BN') return 'bench'
  if (slot === 'TAXI' || slot === 'IR') return 'taxi'
  return 'starters'
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

const contractLabel = (r: RosterEntry) => contractYearsLabel(r.salary, r.years_total, r.years_used)

/** Row-actions menu — a single "⋯" trigger replacing separate Edit/Drop text
 *  links, since a row doesn't need two always-visible destructive-adjacent
 *  controls competing with the picking/drag affordances in the same row. */
function RowActionsMenu({
  entry, onEdit, onDropRequested, confirming, onConfirmDrop, dropPending,
}: {
  entry: RosterEntry
  onEdit: () => void
  onDropRequested: () => void
  confirming: boolean
  onConfirmDrop: () => void
  dropPending: boolean
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLSpanElement | null>(null)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  if (confirming) {
    return (
      <button
        type="button"
        onClick={onConfirmDrop}
        disabled={dropPending}
        className="rounded-md border border-destructive px-2 py-1 font-display text-[11px] font-semibold text-destructive"
      >
        {dropPending ? 'Dropping…' : 'Confirm drop'}
      </button>
    )
  }

  return (
    <span ref={ref} className="relative inline-block">
      <button
        type="button"
        aria-label={`Actions for ${entry.name}`}
        onClick={() => setOpen((o) => !o)}
        className="flex h-[26px] w-[26px] items-center justify-center rounded-md text-base leading-none text-muted-foreground hover:bg-accent hover:text-accent-foreground"
      >
        ⋯
      </button>
      {open && (
        <div className="absolute right-0 top-full z-20 mt-1 min-w-[110px] rounded-md border border-border bg-card py-1">
          <button
            type="button"
            onClick={() => { setOpen(false); onEdit() }}
            className="block w-full px-3 py-1.5 text-left text-xs text-foreground hover:bg-accent"
          >
            Edit
          </button>
          <button
            type="button"
            onClick={() => { setOpen(false); onDropRequested() }}
            className="block w-full px-3 py-1.5 text-left text-xs text-negative hover:bg-accent"
          >
            Drop
          </button>
        </div>
      )}
    </span>
  )
}

/**
 * Shared roster table for the Roster tab, for whichever team is selected.
 * Rows are grouped into Starters / Bench / Taxi & IR (each with a fill-count
 * + total-salary header) since that's the arithmetic a lineup pass is
 * actually checking, and it gives the drag handle somewhere to live without
 * adding chrome to every row.
 *
 * Slot is changed by dragging a player's row onto another row (desktop
 * only — native HTML5 drag-and-drop has no touch story) or by clicking the
 * Slot button to enter "picking" mode: every row that's a legal destination
 * for that player lights up and becomes clickable — the row itself is the
 * target, not an abstract slot name, which is what lets a league with two
 * RB slots distinguish "the empty one" from "the one Jahmyr's in" (a swap).
 * Picking mode is announced with an in-table banner (aria-live) plus a
 * per-row "place here"/"swap here" label — color and cursor alone don't
 * exist for a screen reader, and barely exist on a glance at a long table
 * (design review open item 4, its highest-priority fix).
 *
 * Either way it's the same lineup-setting mechanism: there's no separate
 * "Lineup" screen, moving a player between a starter slot and BN/TAXI/IR
 * here is exactly what ScoreLeagueWeek reads when a week gets scored.
 * Landing on an empty row just reassigns the player; landing on an occupied
 * one swaps the two players' slots, matching how drag-and-drop lineups
 * behave elsewhere (Yahoo, ESPN) rather than silently doubling up a slot.
 */
export function NativeRosterTable({ leagueId, roster, slots = {}, onEdit }: Props) {
  const qc = useQueryClient()
  const [confirmDrop, setConfirmDrop] = useState<string | null>(null)
  const [dragOverKey, setDragOverKey] = useState<string | null>(null)
  // The player currently being moved, by click (Slot was clicked) or by
  // starting a drag — either way, only one at a time, and while set every
  // legal destination row is highlighted and clickable. Starting a drag sets
  // this immediately (not just the row under the cursor) so a drag shows the
  // exact same "every legal destination lit up" feedback as clicking Slot,
  // rather than a separate, narrower highlight of its own. This also doubles
  // as the eligibility source for onDragOver — browsers don't expose
  // DataTransfer.getData() until the actual drop fires, so eligibility
  // during a drag has to come from component state, not the payload.
  const [pickingFor, setPickingFor] = useState<string | null>(null)
  const tableRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!pickingFor) return
    const clickHandler = (e: MouseEvent) => {
      if (tableRef.current && !tableRef.current.contains(e.target as Node)) {
        setPickingFor(null)
      }
    }
    const keyHandler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPickingFor(null)
    }
    document.addEventListener('mousedown', clickHandler)
    document.addEventListener('keydown', keyHandler)
    return () => {
      document.removeEventListener('mousedown', clickHandler)
      document.removeEventListener('keydown', keyHandler)
    }
  }, [pickingFor])

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
    qc.invalidateQueries({ queryKey: ['league', leagueId, 'free-agents'] })
    qc.invalidateQueries({ queryKey: keys.leagueTransactions(leagueId) })
  }

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
  const groups = useMemo(() => {
    const byGroup: Record<Group, Row[]> = { starters: [], bench: [], taxi: [] }
    for (const row of rows) byGroup[groupOf(row.slot)].push(row)
    return byGroup
  }, [rows])

  // Dynamic, same convention as the Players tab: only the categories this
  // league's own scoring actually weights nonzero come back from the API at
  // all, so the column set adapts league to league rather than assuming a
  // fixed shape.
  const statColumns = useMemo(() => {
    const present = new Set<string>()
    for (const r of roster) for (const s of r.stats ?? []) present.add(s.stat)
    return SCORING_STATS.filter((s) => present.has(s))
  }, [roster])
  // drag handle, slot, player, contract, actions (5 fixed) + one per stat + the L4 wks trend column
  const totalCols = 5 + statColumns.length + 1

  if (rows.length === 0) {
    return <p className="text-muted-foreground">No roster spots configured yet.</p>
  }

  const dragDisabled = moveMutation.isPending

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

  const renderRow = (row: Row, i: number) => {
    const r = row.entry
    const rowKey = r ? r.gsis_id : `${row.slot}-${i}`
    const dropTargetProps = {
      onDragOver: (e: React.DragEvent<HTMLTableRowElement>) => {
        // Not calling preventDefault() here is what makes an ineligible slot
        // refuse the drop (browser default) — the dragged player's gsis_id
        // isn't readable from DataTransfer until the actual drop event, so
        // eligibility here comes from pickedEntry (set at dragstart), same
        // as the click-to-pick path.
        if (pickedEntry && !canDrop(pickedEntry, row.slot, r)) return
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        setDragOverKey(rowKey)
      },
      onDragLeave: () => setDragOverKey((k) => (k === rowKey ? null : k)),
      onDrop: (e: React.DragEvent<HTMLTableRowElement>) => handleDrop(e, row.slot, r),
    }
    // While a player is being moved by click (pickedEntry set), every row
    // that's a legal destination for them lights up and becomes clickable —
    // the row itself is the target, so two same-named slots (two RB rows)
    // are distinguishable: one may be empty, one may hold someone else (a
    // swap).
    const isTarget = !!pickedEntry && canDrop(pickedEntry, row.slot, r)
    // The one row currently under the cursor during an active drag — a
    // stronger treatment than "eligible" so it's unambiguous which row you'd
    // actually land on, not just which rows you could. Only ever set on an
    // eligible row (onDragOver never sets it otherwise), so this is always a
    // subset of isTarget.
    const isDragOverTarget = dragOverKey === rowKey
    const targetWord = r ? 'swap here' : 'place here'
    const targetLabel = pickedEntry
      ? r
        ? `Swap ${pickedEntry.name} with ${r.name}`
        : `Place ${pickedEntry.name} in ${row.slot}`
      : undefined
    const pickTargetProps = isTarget
      ? {
          onClick: () => handlePick(row.slot, r),
          role: 'button' as const,
          tabIndex: 0,
          'aria-label': targetLabel,
          onKeyDown: (e: React.KeyboardEvent) => {
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              handlePick(row.slot, r)
            }
          },
        }
      : {}
    const targetHighlight = isDragOverTarget
      ? 'cursor-pointer border-l-[3px] border-l-primary bg-positive-light ring-1 ring-inset ring-primary'
      : isTarget
      ? 'cursor-pointer border-l-[3px] border-l-primary bg-positive-light/40'
      : ''

    const dragHandle = (
      <TableCell className={`w-8 px-0 pl-3 text-xs ${dragDisabled ? 'text-muted-foreground/40' : 'text-muted-foreground'}`}>
        {r ? '⠿' : ''}
      </TableCell>
    )

    if (!r) {
      return (
        <TableRow key={rowKey} className={targetHighlight} {...dropTargetProps} {...pickTargetProps}>
          {dragHandle}
          <TableCell className="font-mono text-xs font-semibold text-muted-foreground">{row.slot}</TableCell>
          <TableCell className="italic text-muted-foreground">
            Empty
            {isTarget && <span className="ml-2 not-italic font-mono text-[11px] text-primary">{targetWord}</span>}
          </TableCell>
          {statColumns.map((s) => (
            <TableCell key={s} className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
          ))}
          <TableCell className="text-center text-muted-foreground">—</TableCell>
          <TableCell className="text-right font-mono tabular-nums text-muted-foreground">—</TableCell>
          <TableCell />
        </TableRow>
      )
    }
    return (
      <ClickableRow
        key={r.gsis_id}
        href={isTarget ? undefined : `/players/${r.gsis_id}?league=${leagueId}`}
        className={`${targetHighlight} ${dragDisabled ? '' : 'cursor-grab active:cursor-grabbing'}`}
        draggable={!dragDisabled}
        onDragStart={(e) => {
          e.dataTransfer.setData(DRAG_MIME, r.gsis_id)
          e.dataTransfer.effectAllowed = 'move'
          setPickingFor(r.gsis_id)
        }}
        onDragEnd={() => {
          setPickingFor(null)
          setDragOverKey(null)
        }}
        {...dropTargetProps}
        {...pickTargetProps}
      >
        {dragHandle}
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
        <PlayerCell
          name={r.name}
          imageUrl={r.headshot_url}
          linked
          sub={isTarget ? targetWord : SLOT_DISPLAY_ORDER.filter((s) => !BENCH_SLOTS.has(s) && !DISPLAY_HIDDEN_SLOTS.has(s) && isSlotEligible(s, r.position)).join(' · ')}
        />
        {statColumns.map((s) => {
          const v = statValue(r.stats, s)
          return (
            <TableCell key={s} className="text-right font-mono tabular-nums">
              {v != null ? v.toFixed(1) : '—'}
            </TableCell>
          )
        })}
        <TableCell className="text-center"><L4Sparkline trend={r.trend} /></TableCell>
        <TableCell className="text-right font-mono tabular-nums whitespace-nowrap">{contractLabel(r)}</TableCell>
        <TableCell className="text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
          <RowActionsMenu
            entry={r}
            onEdit={() => onEdit(r)}
            onDropRequested={() => setConfirmDrop(r.gsis_id)}
            confirming={confirmDrop === r.gsis_id}
            onConfirmDrop={() => dropMutation.mutate(r.gsis_id)}
            dropPending={dropMutation.isPending}
          />
        </TableCell>
      </ClickableRow>
    )
  }

  const groupHeader = (group: Group, groupRows: Row[]) => {
    if (groupRows.length === 0) return null
    const filled = groupRows.filter((r) => r.entry).length
    const salary = groupRows.reduce((sum, r) => sum + (r.entry?.salary ?? 0), 0)
    return (
      <TableRow key={`header-${group}`} className="bg-background hover:bg-background">
        <TableCell colSpan={totalCols} className="py-1.5 font-display text-[10px] font-bold uppercase tracking-wide text-muted-foreground">
          {GROUP_LABEL[group]}{' '}
          <span className="font-mono text-xs font-normal normal-case tracking-normal">
            {filled} of {groupRows.length} filled · ${salary}
            {group === 'taxi' && ' · does not count against starters'}
          </span>
        </TableCell>
      </TableRow>
    )
  }

  return (
    <div ref={tableRef} className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
      <Table>
        <TableHeader style={{ top: 0 }}>
          <HeaderRow>
            <TableHead className="w-8" />
            <TableHead>Slot</TableHead>
            <TableHead>Player</TableHead>
            {statColumns.map((s) => (
              <TableHead key={s} className="text-right">{SCORING_LABELS[s]}</TableHead>
            ))}
            <TableHead className="text-center">L4 wks</TableHead>
            <TableHead className="text-right">Contract</TableHead>
            <TableHead />
          </HeaderRow>
        </TableHeader>
        <TableBody>
          {pickedEntry && (
            <TableRow className="bg-background hover:bg-background">
              <TableCell colSpan={totalCols} className="py-1.5 text-xs text-muted-foreground" aria-live="polite">
                <span className="font-semibold text-primary">Choosing a slot for {pickedEntry.name}</span>
                {' '}— click a highlighted row to place or swap.
                <span className="float-right font-semibold">Esc to cancel</span>
              </TableCell>
            </TableRow>
          )}
          {(['starters', 'bench', 'taxi'] as Group[]).flatMap((group) => {
            const groupRows = groups[group]
            if (groupRows.length === 0) return []
            return [groupHeader(group, groupRows), ...groupRows.map((row, i) => renderRow(row, i))]
          })}
        </TableBody>
      </Table>
    </div>
  )
}
