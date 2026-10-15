import { Fragment, useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, ChevronRight, ChevronDown } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell, TableRow } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort, type SortDir } from '@/components/ui/table-helpers'
import { FilterChip } from '@/components/ui/filter-chip'
import { Input } from '@/components/ui/input'
import { getNBADraftValues, NBA_CATEGORY_LABELS, type NBADraftPlayer } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { loadPersistedFilters, savePersistedFilters } from '@/lib/persistedFilters'

const POSITIONS = ['G', 'F', 'C']

interface Props {
  leagueId: number
  season: number
  onSelectPlayer: (espnId: string) => void
}

interface SavedFilters {
  positions: string[]
  search: string
  sortCol: string
  sortDir: SortDir
}

const filtersStorageKey = (leagueId: number, season: number) => `fs.nba-auction.${leagueId}.${season}.filters`

function edgeOf(p: NBADraftPlayer) {
  return p.consensus_auction_value == null ? null : p.auction_value - p.consensus_auction_value
}

const isPct = (cat: string) => cat === 'fg_pct' || cat === 'ft_pct'
const fmtStat = (cat: string, v: number) => (isPct(cat) ? v.toFixed(3).replace(/^0/, '') : v.toFixed(1))
const fmtZ = (z: number) => `${z > 0 ? '+' : ''}${z.toFixed(1)}`

function sourceLabel(source: string) {
  return source
    .split('_')
    .map((w) => w[0].toUpperCase() + w.slice(1))
    .join(' ')
}

/**
 * League-specific NBA auction dollar values: our own price (a z-score sum
 * over only the categories this league scores, priced by position-scarcity
 * replacement level off this league's own roster slots) alongside a
 * consensus price derived from external rankings. League-scoped (unlike
 * NBAConsensusRankings, which is global) since the dollar figure only means
 * something relative to one league's actual budget/teams/slots/scoring. See
 * docs/stats/nba-auction-values.md.
 */
export function NBAAuctionValues({ leagueId, season, onSelectPlayer }: Props) {
  const saved = useMemo(() => loadPersistedFilters<SavedFilters>(filtersStorageKey(leagueId, season)), [leagueId, season])
  const [positions, setPositions] = useState<Set<string>>(new Set(saved.positions ?? []))
  const [search, setSearch] = useState(saved.search ?? '')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const { sortCol, sortDir, handleSort } = useTableSort(saved.sortCol ?? 'auction', saved.sortDir ?? 'desc', ['auction', 'rank', 'player', 'team', 'pos', 'consensus', 'edge', 'comps', 'vscomps'])

  useEffect(() => {
    savePersistedFilters<SavedFilters>(filtersStorageKey(leagueId, season), { positions: [...positions], search, sortCol, sortDir })
  }, [leagueId, season, positions, search, sortCol, sortDir])

  const { data, isFetching: darkoFetching } = useQuery({
    queryKey: keys.nbaDraftValues(leagueId, season, 'darko'),
    queryFn: () => getNBADraftValues(leagueId, season, 'darko'),
  })
  const { data: compsData, isFetching: compsFetching } = useQuery({
    queryKey: keys.nbaDraftValues(leagueId, season, 'comps'),
    queryFn: () => getNBADraftValues(leagueId, season, 'comps'),
  })
  const isFetching = darkoFetching || compsFetching
  const compsPrice = useMemo(
    () => new Map((compsData?.players ?? []).map((p) => [p.espn_id, p.auction_value])),
    [compsData],
  )
  // DARKO minus comps: positive = DARKO likes the player more than our comp model.
  const vsComps = (p: NBADraftPlayer) => {
    const c = compsPrice.get(p.espn_id)
    return c == null ? null : p.auction_value - c
  }

  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }
  const toggleExpanded = (espnId: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(espnId)) next.delete(espnId); else next.add(espnId)
      return next
    })
  }

  const players = data?.players ?? []
  const search_ = search.trim().toLowerCase()
  const filtered = players.filter((p) => {
    if (positions.size > 0 && !positions.has(p.position)) return false
    if (search_ && !`${p.name} ${p.team} ${p.position}`.toLowerCase().includes(search_)) return false
    return true
  })

  const rows = useMemo(() => {
    const dirMul = sortDir === 'asc' ? 1 : -1
    return [...filtered].sort((a, b) => {
      let cmp: number
      switch (sortCol) {
        case 'auction': cmp = a.auction_value - b.auction_value; break
        case 'rank': cmp = a.overall_rank - b.overall_rank; break
        case 'player': cmp = a.name.localeCompare(b.name); break
        case 'team': cmp = (a.team || '').localeCompare(b.team || ''); break
        case 'pos': cmp = (a.position || '').localeCompare(b.position || ''); break
        case 'consensus': cmp = (a.consensus_auction_value ?? -1) - (b.consensus_auction_value ?? -1); break
        case 'edge': cmp = (edgeOf(a) ?? -Infinity) - (edgeOf(b) ?? -Infinity); break
        case 'comps': cmp = (compsPrice.get(a.espn_id) ?? -1) - (compsPrice.get(b.espn_id) ?? -1); break
        case 'vscomps': cmp = (vsComps(a) ?? -Infinity) - (vsComps(b) ?? -Infinity); break
        default: cmp = 0
      }
      return cmp * dirMul
    })
  }, [filtered, sortCol, sortDir, compsPrice])

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
        {!isFetching && data && (
          <span className="text-xs text-muted-foreground ml-1">
            {rows.length} players · {data.num_teams} teams · ${data.budget_per_team} budget
          </span>
        )}
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
                <TableHead className="w-6" />
                <SortableHead col="rank" current={sortCol} dir={sortDir} onSort={handleSort}>Rank</SortableHead>
                <SortableHead col="player" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>Team</SortableHead>
                <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort}>Pos</SortableHead>
                <SortableHead col="auction" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">DARKO $</SortableHead>
                <SortableHead col="comps" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Comps $</SortableHead>
                <SortableHead col="vscomps" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Δ Comps</SortableHead>
                <SortableHead col="consensus" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Cons $</SortableHead>
                <SortableHead col="edge" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Edge</SortableHead>
                <TableHead className="text-right">Tier</TableHead>
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p) => {
                const edge = edgeOf(p)
                const comps = compsPrice.get(p.espn_id)
                const dComps = vsComps(p)
                const hasSources = (p.source_ranks?.length ?? 0) > 0
                const zByCat = new Map(p.z_scores.map((z) => [z.category, z.value]))
                const isOpen = expanded.has(p.espn_id)
                return (
                  <Fragment key={p.espn_id}>
                    <ClickableRow>
                      <TableCell>
                        <button
                            onClick={() => toggleExpanded(p.espn_id)}
                            aria-label={isOpen ? 'Collapse player detail' : 'Expand player detail'}
                            aria-expanded={isOpen}
                            className="flex h-5 w-5 items-center justify-center text-muted-foreground hover:text-foreground"
                          >
                            {isOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                          </button>
                      </TableCell>
                      <TableCell className="font-mono tabular-nums text-muted-foreground">{p.overall_rank}</TableCell>
                      <PlayerCell name={p.name} imageUrl={p.headshot_url} onClick={() => onSelectPlayer(p.espn_id)} />
                      <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                      <TableCell className="text-muted-foreground">{p.position}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums text-foreground">${p.auction_value}</TableCell>
                      <TableCell className="text-right font-mono tabular-nums text-muted-foreground">
                        {comps != null ? `$${comps}` : '—'}
                      </TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {dComps == null ? (
                          <span className="text-muted-foreground/40">—</span>
                        ) : (
                          <span className={Math.abs(dComps) < 5 ? 'text-muted-foreground' : dComps > 0 ? 'text-positive' : 'text-secondary'}>
                            {dComps > 0 ? `+$${dComps}` : dComps < 0 ? `-$${Math.abs(dComps)}` : '$0'}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-mono tabular-nums text-muted-foreground">
                        {p.consensus_auction_value != null
                          ? `$${p.consensus_auction_value}${p.consensus_sources === 1 ? '*' : ''}`
                          : '—'}
                      </TableCell>
                      <TableCell className="text-right font-mono tabular-nums">
                        {edge == null ? (
                          <span className="text-muted-foreground/40">—</span>
                        ) : (
                          <span className={Math.abs(edge) < 5 ? 'text-muted-foreground' : edge > 0 ? 'text-positive' : 'text-secondary'}>
                            {edge > 0 ? `+$${edge}` : edge < 0 ? `-$${Math.abs(edge)}` : '$0'}
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-right font-mono tabular-nums text-muted-foreground">{p.tier || '—'}</TableCell>
                    </ClickableRow>
                    {isOpen && (
                      <TableRow className="bg-muted/30">
                        <TableCell colSpan={11} className="py-3">
                          <div className="pl-7 mb-3">
                            <div className="text-xs text-muted-foreground mb-1.5">
                              DARKO projection, per game
                              {p.minutes > 0 && <> · <span className="font-mono text-foreground">{p.minutes.toFixed(1)}</span> min</>}
                              {' '}· z-score vs. the draftable pool
                              <span className="ml-2">
                                value <span className="font-mono text-foreground">{fmtZ(p.fantasy_value)}</span>
                                {' '}· VOR <span className="font-mono text-foreground">{p.vor.toFixed(1)}</span>
                              </span>
                            </div>
                            <div className="flex flex-wrap gap-x-5 gap-y-2">
                              {p.stats.map((s) => {
                                const z = zByCat.get(s.category) ?? 0
                                const counts = (data?.settings.scoring[s.category] ?? 0) !== 0
                                return (
                                  <div key={s.category} className={`min-w-[3.25rem] ${counts ? '' : 'opacity-40'}`} title={counts ? undefined : 'Not a scoring category in this league'}>
                                    <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{NBA_CATEGORY_LABELS[s.category]}</div>
                                    <div className="font-mono tabular-nums text-sm text-foreground">{fmtStat(s.category, s.value)}</div>
                                    <div className={`font-mono tabular-nums text-xs ${Math.abs(z) < 0.5 ? 'text-muted-foreground' : z > 0 ? 'text-positive' : 'text-secondary'}`}>{fmtZ(z)}</div>
                                  </div>
                                )
                              })}
                            </div>
                          </div>
                          {hasSources && (<>
                          <div className="flex flex-wrap items-center gap-x-6 gap-y-1 pl-7 text-xs">
                            <span className="text-muted-foreground">
                              Each outlet's own <strong className="text-foreground">{p.position}</strong> position rank
                              (median → <span className="font-mono">${p.consensus_auction_value ?? '—'}</span> on our
                              curve). No outlet publishes real auction prices, so this is rank, not a second column of
                              dollar figures.
                            </span>
                          </div>
                          <div className="flex flex-wrap gap-x-6 gap-y-1 pl-7 mt-1.5">
                            {p.source_ranks!.map((s) => (
                              <span key={s.source} className="text-xs font-mono tabular-nums text-foreground">
                                {sourceLabel(s.source)} <span className="text-muted-foreground">#{s.rank}</span>
                              </span>
                            ))}
                          </div>
                          </>)}
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
