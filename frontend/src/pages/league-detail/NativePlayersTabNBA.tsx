import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import { getLeagueRostersNBA, getLeagueFreeAgentsNBA, NBA_CATEGORY_LABELS, type Team } from '@/api/client'
import { keys } from '@/api/queryKeys'

const POSITIONS = ['G', 'F', 'C']

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
}

/**
 * Players tab for a native NBA league — Free Agents/Rostered toggle, same
 * "filled tray, not chips" convention as the NFL version. Category columns
 * are fixed (all 9 roto categories — no per-league "relevant" filtering,
 * unlike the NFL side's scored-stats-only columns, since a native NBA
 * league's scoring.json controls week-to-week category inclusion, not
 * whether a category has any meaning to show at all).
 */
export function NativePlayersTabNBA({ leagueId, active, teams }: Props) {
  const [view, setView] = useState<'free-agents' | 'rostered'>('free-agents')
  const [positions, setPositions] = useState<Set<string>>(new Set())

  const { data: freeAgents, isFetching: loadingFA } = useQuery({
    queryKey: keys.freeAgentsNBA(leagueId, ''),
    queryFn: () => getLeagueFreeAgentsNBA(leagueId, '', 200),
    enabled: active && view === 'free-agents',
  })
  const { data: roster, isFetching: loadingRoster } = useQuery({
    queryKey: keys.leagueRostersNBA(leagueId),
    queryFn: () => getLeagueRostersNBA(leagueId),
    enabled: active && view === 'rostered',
  })

  if (!active) return null

  const teamName = (id: number) => teams.find((t) => t.id === id)?.name ?? `Team ${id}`
  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }
  const matchesPosition = (position: string) => positions.size === 0 || positions.has(position)

  const loading = view === 'free-agents' ? loadingFA : loadingRoster
  const rows = view === 'free-agents' ? (freeAgents ?? []).filter((p) => matchesPosition(p.position)) : (roster ?? []).filter((p) => matchesPosition(p.position))

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <div className="flex w-fit flex-none rounded-lg bg-muted overflow-hidden">
          {([{ value: 'free-agents' as const, label: 'Free Agents' }, { value: 'rostered' as const, label: 'Rostered' }]).map((s) => (
            <button
              key={s.value}
              onClick={() => setView(s.value)}
              className={`px-3 py-1.5 font-display text-xs font-semibold whitespace-nowrap ${
                view === s.value ? 'bg-foreground text-background' : 'bg-card text-muted-foreground hover:bg-muted'
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
        <div className="flex flex-none gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>{p}</FilterChip>
          ))}
        </div>
        {!loading && <span className="text-xs text-muted-foreground ml-1">{rows.length} players</span>}
      </div>

      {loading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">{view === 'free-agents' ? 'No free agents match this filter.' : 'No players rostered yet.'}</p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <TableHead>Player</TableHead>
                <TableHead>Team</TableHead>
                <TableHead>Pos</TableHead>
                {Object.values(NBA_CATEGORY_LABELS).map((label) => (
                  <TableHead key={label} className="text-right">{label}</TableHead>
                ))}
                {view === 'rostered' && <TableHead>Rostered By</TableHead>}
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <ClickableRow key={p.espn_id}>
                  <PlayerCell name={p.name} imageUrl={p.headshot_url} />
                  <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                  <TableCell className="text-muted-foreground">{p.position}</TableCell>
                  {Object.keys(NBA_CATEGORY_LABELS).map((cat) => {
                    const v = p.stats?.find((s) => s.category === cat)?.value
                    return (
                      <TableCell key={cat} className="text-right font-mono tabular-nums">
                        {v != null ? (cat === 'fg_pct' || cat === 'ft_pct' ? v.toFixed(3) : v) : '—'}
                      </TableCell>
                    )
                  })}
                  {view === 'rostered' && 'team_id' in p && (
                    <TableCell className="text-muted-foreground">{teamName((p as { team_id: number }).team_id)}</TableCell>
                  )}
                </ClickableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
