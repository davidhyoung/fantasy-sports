import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { updateLeagueRoster, type RosterEntry } from '@/api/client'
import { keys } from '@/api/queryKeys'

/** Shared by the commissioner-wide Roster tab, the personal My Team tab, and
 *  the player-detail panel's fixed action bar — the last of those only has
 *  a player's contract (from `GET /nfl/players/:gsisId`), not a full roster
 *  row, so this only asks for the three fields it actually reads. */
export function EditContractForm({
  leagueId, entry, onClose,
}: { leagueId: number; entry: Pick<RosterEntry, 'gsis_id' | 'salary' | 'years_total'>; onClose: () => void }) {
  const qc = useQueryClient()
  const [salary, setSalary] = useState(entry.salary)
  const [yearToYear, setYearToYear] = useState(entry.years_total == null)
  const [yearsTotal, setYearsTotal] = useState(entry.years_total ?? 1)

  const mutation = useMutation({
    mutationFn: () =>
      updateLeagueRoster(leagueId, entry.gsis_id, {
        salary,
        years_total: yearToYear ? null : yearsTotal,
        clear_years_total: yearToYear,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.leagueRosters(leagueId) })
      qc.invalidateQueries({ queryKey: ['league', leagueId, 'team'] })
      onClose()
    },
  })

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex flex-col gap-1">
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Salary $</span>
          <input
            type="number" min={0} value={salary}
            onChange={(e) => setSalary(Math.max(0, parseInt(e.target.value, 10) || 0))}
            className="h-8 w-24 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
        </label>
        <label className="flex items-center gap-1.5 text-xs text-muted-foreground mt-4">
          <input type="checkbox" checked={yearToYear} onChange={(e) => setYearToYear(e.target.checked)} />
          Year-to-year
        </label>
        {!yearToYear && (
          <label className="flex flex-col gap-1">
            <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Years total</span>
            <input
              type="number" min={1} max={10} value={yearsTotal}
              onChange={(e) => setYearsTotal(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="h-8 w-16 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
          </label>
        )}
      </div>
      {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}
      <div className="flex justify-end gap-2 border-t border-border pt-4">
        <Button variant="outline" onClick={onClose}>Cancel</Button>
        <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
          {mutation.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
