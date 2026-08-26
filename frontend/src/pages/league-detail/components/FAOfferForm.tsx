import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { createFAOffer, getLeagueFreeAgents, getFAValuations, type FreeAgent } from '@/api/client'
import { keys } from '@/api/queryKeys'

interface Props {
  leagueId: number
  teamId: number
  onClose: () => void
}

/**
 * Make (or edit) a free-agent offer for one team. Search step mirrors
 * PlayerAssignForm's; the confirm step is different in kind, not just
 * fields — this shows the player's real market value, the reservation
 * floor below it, and his own preferred contract length (L*) *before* you
 * set salary/years, since those three numbers are exactly what the offer
 * gets scored against at resolution. A hidden formula would turn this into
 * a lottery; showing it is what makes free agency a solvable problem.
 */
export function FAOfferForm({ leagueId, teamId, onClose }: Props) {
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [position, setPosition] = useState('')
  const [selected, setSelected] = useState<FreeAgent | null>(null)
  const [salary, setSalary] = useState(0)
  const [years, setYears] = useState(1)

  const { data: candidates = [], isFetching } = useQuery({
    queryKey: keys.freeAgents(leagueId, position, search),
    queryFn: () => getLeagueFreeAgents(leagueId, position, 25, 0, search),
    enabled: search.trim().length > 0 || position !== '',
  })

  const { data: valuations } = useQuery({
    queryKey: keys.faValuations(leagueId, selected ? [selected.gsis_id] : []),
    queryFn: () => getFAValuations(leagueId, [selected!.gsis_id]),
    enabled: !!selected,
  })
  const valuation = valuations?.[0]

  const mutation = useMutation({
    mutationFn: () => createFAOffer(leagueId, { gsis_id: selected!.gsis_id, team_id: teamId, salary, years }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.faOffers(leagueId) })
      onClose()
    },
  })

  return (
    <div className="flex flex-col gap-4">
      {!selected ? (
        <>
          <Input
            autoFocus
            placeholder="Search free agents by name…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          <div className="flex flex-wrap gap-1.5">
            {['', 'QB', 'RB', 'WR', 'TE', 'K'].map((p) => (
              <button
                key={p || 'all'}
                onClick={() => setPosition(p)}
                className={`rounded-pill border px-2.5 py-1 font-display text-[11px] font-semibold ${
                  position === p ? 'border-positive-border bg-positive-light text-primary' : 'border-border text-muted-foreground'
                }`}
              >
                {p || 'All'}
              </button>
            ))}
          </div>
          <div className="max-h-64 overflow-y-auto rounded-md border border-border">
            {isFetching ? (
              <p className="p-3 text-xs text-muted-foreground">Searching…</p>
            ) : candidates.length === 0 ? (
              <p className="p-3 text-xs text-muted-foreground">
                {search.trim() || position ? 'No free agents match.' : 'Type a name or pick a position to search.'}
              </p>
            ) : (
              candidates.map((c) => (
                <button
                  key={c.gsis_id}
                  onClick={() => setSelected(c)}
                  className="flex w-full items-center gap-2.5 border-b border-border px-3 py-2 text-left last:border-0 hover:bg-muted"
                >
                  <PlayerAvatar src={c.headshot_url} alt={c.name} size={28} />
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-display text-[13px] font-semibold text-foreground">{c.name}</div>
                    <div className="text-xs text-muted-foreground">{c.position} · {c.team || 'FA'}</div>
                  </div>
                  {c.proj_fpts_ppr != null && (
                    <span className="font-mono text-xs tabular-nums text-muted-foreground">{c.proj_fpts_ppr.toFixed(1)}</span>
                  )}
                </button>
              ))
            )}
          </div>
        </>
      ) : (
        <>
          <div className="flex items-center gap-2.5 rounded-md border border-border px-3 py-2">
            <PlayerAvatar src={selected.headshot_url} alt={selected.name} size={32} />
            <div className="min-w-0 flex-1">
              <div className="font-display text-sm font-semibold text-foreground">{selected.name}</div>
              <div className="text-xs text-muted-foreground">{selected.position} · {selected.team || 'FA'}</div>
            </div>
            <Button variant="text" size="bare" onClick={() => setSelected(null)}>Change</Button>
          </div>

          {valuation && (
            <div className="flex flex-wrap gap-4 rounded-md bg-muted px-3 py-2 text-xs">
              <div>
                <div className="font-display font-semibold uppercase tracking-wide text-muted-foreground">Market value</div>
                <div className="font-mono tabular-nums text-foreground">${valuation.auction_value}</div>
              </div>
              <div>
                <div className="font-display font-semibold uppercase tracking-wide text-muted-foreground">Won't sign below</div>
                <div className="font-mono tabular-nums text-foreground">${valuation.reservation_value}</div>
              </div>
              <div>
                <div className="font-display font-semibold uppercase tracking-wide text-muted-foreground">Prefers</div>
                <div className="font-mono tabular-nums text-foreground">{valuation.preferred_years}yr deal</div>
              </div>
            </div>
          )}

          <div className="grid grid-cols-2 gap-3">
            <label className="flex flex-col gap-1">
              <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Salary $</span>
              <input
                type="number"
                min={0}
                value={salary}
                onChange={(e) => setSalary(Math.max(0, parseInt(e.target.value, 10) || 0))}
                className="h-8 w-full rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </label>
            <label className="flex flex-col gap-1">
              <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Years</span>
              <input
                type="number"
                min={1}
                max={6}
                value={years}
                onChange={(e) => setYears(Math.max(1, parseInt(e.target.value, 10) || 1))}
                className="h-8 w-full rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
            </label>
          </div>

          <p className="text-xs text-muted-foreground">
            Offers are ranked by salary × years, discounted the further the length misses his preferred
            deal — the best-scoring offer wins when the window resolves, below reservation never signs.
          </p>

          {mutation.error && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}

          <div className="flex justify-end gap-2 border-t border-border pt-4">
            <Button variant="outline" onClick={onClose}>Cancel</Button>
            <Button onClick={() => mutation.mutate()} disabled={mutation.isPending}>
              {mutation.isPending ? 'Submitting…' : 'Submit offer'}
            </Button>
          </div>
        </>
      )}
    </div>
  )
}
