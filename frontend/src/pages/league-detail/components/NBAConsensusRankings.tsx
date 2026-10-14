import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort, type SortDir } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import { Input } from '@/components/ui/input'
import { getNBARankings, type NBARankingItem } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { loadPersistedFilters, savePersistedFilters } from '@/lib/persistedFilters'

const POSITIONS = ['G', 'F', 'C']

interface Props {
  season: number
  onSelectPlayer: (espnId: string) => void
}

interface SavedFilters {
  positions: string[]
  search: string
  sortCol: string
  sortDir: SortDir
}

const filtersStorageKey = (season: number) => `fs.nba-consensus.${season}.filters`

function sourceLabel(source: string) {
  return source
    .split('_')
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(' ')
}

/**
 * Our comp-based projection rank for every batch-projected NBA player
 * (cmd/project-nba -project), alongside the external consensus median AND
 * each individual outlet's own rank — the piece a pure median hides. Global,
 * not league-scoped (same reasoning as NFL's standalone /divergences page),
 * so it's rendered as a sibling view to the personal draft board rather than
 * filtered by this league's roster/period state. Source columns are driven
 * entirely by whichever sources the API actually returns for this season, so
 * adding or dropping an import source never requires a frontend change.
 */
export function NBAConsensusRankings({ season, onSelectPlayer }: Props) {
  const saved = useMemo(() => loadPersistedFilters<SavedFilters>(filtersStorageKey(season)), [season])
  const [positions, setPositions] = useState<Set<string>>(new Set(saved.positions ?? []))
  const [search, setSearch] = useState(saved.search ?? '')
  const { sortCol, sortDir, handleSort } = useTableSort(saved.sortCol ?? 'our_rank', saved.sortDir ?? 'asc', ['our_rank', 'player', 'team', 'pos', 'consensus'])

  useEffect(() => {
    savePersistedFilters<SavedFilters>(filtersStorageKey(season), { positions: [...positions], search, sortCol, sortDir })
  }, [season, positions, search, sortCol, sortDir])

  const { data, isFetching } = useQuery({
    queryKey: keys.nbaRankings(season),
    queryFn: () => getNBARankings(season),
  })

  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }

  const players = data?.players ?? []
  const sources = useMemo(() => {
    const set = new Set<string>()
    for (const p of players) for (const s of p.sources ?? []) set.add(s.source)
    return [...set].sort()
  }, [players])

  const search_ = search.trim().toLowerCase()
  const filtered = players.filter((p) => {
    if (positions.size > 0 && !positions.has(p.position)) return false
    if (search_ && !`${p.name} ${p.team} ${p.position}`.toLowerCase().includes(search_)) return false
    return true
  })

  const rankOf = (p: NBARankingItem, source: string) => p.sources?.find((s) => s.source === source)?.rank

  const rows = useMemo(() => {
    const dirMul = sortDir === 'asc' ? 1 : -1
    return [...filtered].sort((a, b) => {
      let cmp: number
      switch (sortCol) {
        case 'our_rank': cmp = a.our_rank - b.our_rank; break
        case 'player': cmp = a.name.localeCompare(b.name); break
        case 'team': cmp = (a.team || '').localeCompare(b.team || ''); break
        case 'pos': cmp = (a.position || '').localeCompare(b.position || ''); break
        case 'consensus': cmp = (a.consensus_rank_median || Infinity) - (b.consensus_rank_median || Infinity); break
        default: {
          const av = rankOf(a, sortCol) ?? Infinity
          const bv = rankOf(b, sortCol) ?? Infinity
          cmp = av - bv
        }
      }
      return cmp * dirMul
    })
  }, [filtered, sortCol, sortDir])

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <div className="flex flex-none gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>{p}</FilterChip>
          ))}
        </div>
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search players…"
          className="w-48"
        />
        {!isFetching && <span className="text-xs text-muted-foreground ml-1">{rows.length} players</span>}
      </div>

      {isFetching ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">
          {players.length === 0
            ? 'No projections available yet — run cmd/project-nba -project for this season.'
            : 'No players match this filter.'}
        </p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <SortableHead col="our_rank" current={sortCol} dir={sortDir} onSort={handleSort}>Our Rank</SortableHead>
                <SortableHead col="player" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>Team</SortableHead>
                <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort}>Pos</SortableHead>
                <SortableHead col="consensus" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Consensus</SortableHead>
                {sources.map((s) => (
                  <SortableHead key={s} col={s} current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">
                    {sourceLabel(s)}
                  </SortableHead>
                ))}
                <TableHead className="text-right">Sources</TableHead>
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => (
                <ClickableRow key={p.espn_id}>
                  <TableCell className="font-mono tabular-nums text-muted-foreground">{p.our_rank}</TableCell>
                  <PlayerCell name={p.name} imageUrl={p.headshot_url} onClick={() => onSelectPlayer(p.espn_id)} />
                  <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                  <TableCell className="text-muted-foreground">{p.position}</TableCell>
                  <TableCell className="text-right font-mono tabular-nums text-foreground">
                    {p.source_count > 0 ? p.consensus_rank_median.toFixed(1) : '—'}
                  </TableCell>
                  {sources.map((s) => {
                    const v = rankOf(p, s)
                    return (
                      <TableCell key={s} className="text-right font-mono tabular-nums text-muted-foreground">
                        {v != null ? v : '—'}
                      </TableCell>
                    )
                  })}
                  <TableCell className="text-right font-mono tabular-nums text-muted-foreground">{p.source_count}</TableCell>
                </ClickableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
