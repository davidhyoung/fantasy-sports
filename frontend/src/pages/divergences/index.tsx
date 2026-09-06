import { useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import { Table, TableBody, TableCell, TableHead, TableHeader } from '@/components/ui/table'
import { FilterChip, SelectControl } from '@/components/ui/filter-chip'
import {
  ClickableRow,
  HeaderRow,
  PlayerCell,
  PlayerAvatar,
  SortableHead,
  useTableSort,
  HeaderTip,
} from '@/components/ui/table-helpers'
import { MobileStatCard, type MobileStatField } from '@/components/ui/mobile-stat-card'
import { MobileSortSheet, type MobileSortOption } from '@/components/ui/mobile-sort-sheet'
import { PROJECTION_SEASON } from '@/lib/constants'
import type { DivergenceItem, RankingsFormat } from '@/api/client'
import { PlayerDetailPanel } from '@/pages/player-detail/PlayerDetailPanel'
import { useDivergences } from './hooks/useDivergences'
import DeltaBadge from './components/DeltaBadge'

const SORT_OPTIONS: MobileSortOption[] = [
  { col: 'delta', label: 'Δ' },
  { col: 'name', label: 'Player' },
  { col: 'pos', label: 'Pos' },
  { col: 'our', label: 'Ours' },
  { col: 'consensus', label: 'Consensus' },
  { col: 'sources', label: 'Sources' },
]

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K', 'Flex', 'Superflex']
const POSITION_FILTER: Record<string, string> = {
  Flex: 'RB,WR,TE',
  Superflex: 'QB,RB,WR,TE',
}

const FORMATS: { label: string; value: RankingsFormat }[] = [
  { label: 'PPR', value: 'ppr' },
  { label: 'Half PPR', value: 'half' },
  { label: 'Standard', value: 'standard' },
]

// Ascending-first: names read A→Z; our rank and the consensus rank are both
// "1 is best." |delta| and source count are magnitudes where bigger is more
// interesting (biggest disagreement / most corroborated first), so they stay
// descending — the default already set below.
const ASC_COLS = ['name', 'pos', 'our', 'consensus']

type SortKey = 'delta' | 'name' | 'pos' | 'our' | 'consensus' | 'sources'

function sortRows(players: DivergenceItem[], col: SortKey, dir: 'asc' | 'desc'): DivergenceItem[] {
  return [...players].sort((a, b) => {
    let cmp = 0
    switch (col) {
      case 'name':
        cmp = a.name.localeCompare(b.name)
        break
      case 'pos':
        cmp = a.position_group.localeCompare(b.position_group)
        break
      case 'our':
        cmp = a.our_rank - b.our_rank
        break
      case 'consensus':
        cmp = a.consensus_rank_median - b.consensus_rank_median
        break
      case 'sources':
        cmp = a.source_count - b.source_count
        break
      // Default view is "biggest disagreement first", which is magnitude —
      // direction is conveyed by the badge, not the ordering.
      default:
        cmp = Math.abs(a.rank_delta) - Math.abs(b.rank_delta)
    }
    return dir === 'desc' ? -cmp : cmp
  })
}

export default function Divergences() {
  const [positionLabel, setPositionLabel] = useState('All')
  const [format, setFormat] = useState<RankingsFormat>('ppr')
  const [viewingPlayer, setViewingPlayer] = useState<string | null>(null)
  const position =
    positionLabel === 'All' ? '' : (POSITION_FILTER[positionLabel] ?? positionLabel)

  const { sortCol, sortDir, handleSort } = useTableSort('delta', 'desc', ASC_COLS)
  const { data, isLoading, isError } = useDivergences({ format, position })

  const players = data ? sortRows(data.players, sortCol as SortKey, sortDir) : []

  return (
    <div className="max-w-6xl mx-auto space-y-4">
      <RouterLink to="/" className="text-xs text-muted-foreground hover:text-foreground">
        ← Back to leagues
      </RouterLink>

      <div className="flex flex-col sm:flex-row sm:items-start sm:justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-bold text-foreground">Consensus Divergence</h1>
          <p className="text-sm text-muted-foreground mt-1 max-w-2xl">
            Where our projections disagree most with external expert rankings and ADP,
            compared within position group. A large gap is a prompt to look — not a
            verdict that either side is wrong. Our model can't see trades, injuries, or
            camp battles; the market can't see our comp analysis.
          </p>
        </div>

        <SelectControl
          label="Scoring"
          value={format}
          onChange={(e) => setFormat(e.target.value as RankingsFormat)}
          className="self-start sm:self-center"
        >
          {FORMATS.map((f) => (
            <option key={f.value} value={f.value}>{f.label}</option>
          ))}
        </SelectControl>
      </div>

      <div className="flex gap-1.5 flex-wrap">
        {POSITIONS.map((pos) => (
          <FilterChip
            key={pos}
            active={pos === positionLabel}
            onClick={() => setPositionLabel(pos)}
          >
            {pos}
          </FilterChip>
        ))}
      </div>

      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading divergences…</p>
      ) : isError ? (
        <p className="text-destructive text-sm">Failed to load divergences.</p>
      ) : data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {data.total} player{data.total !== 1 ? 's' : ''} with consensus data for{' '}
            {PROJECTION_SEASON}
            {positionLabel !== 'All' ? ` · ${positionLabel}` : ''}
          </p>

          {players.length === 0 ? (
            <p className="text-muted-foreground text-sm mt-6">
              No consensus data imported for this scoring format
              {positionLabel !== 'All' ? ` and position filter` : ''}. Import rankings for
              it and re-run the diff step to populate this view.
            </p>
          ) : (
            <>
            {/* Card list below md — face: Δ badge, Player, Ours vs Consensus rank;
                expansion: Position, Source count, situational-note text (a
                hover-title tooltip on desktop, so it must be visible text here). */}
            <div className="space-y-2 md:hidden">
              <div className="flex justify-end">
                <MobileSortSheet options={SORT_OPTIONS} current={sortCol} dir={sortDir} onSort={handleSort} />
              </div>
              {players.map((p) => {
                const expanded: MobileStatField[] = [
                  { label: 'Position', value: p.position_group },
                  {
                    label: 'Sources',
                    value: (
                      <span title={p.source_count < 2 ? 'Uncorroborated' : undefined}>
                        {p.source_count}{p.source_count < 2 ? '*' : ''}
                      </span>
                    ),
                  },
                  {
                    label: 'Context',
                    value: p.notes.length > 0 ? (
                      <ul className="space-y-1">
                        {p.notes.map((n, i) => (
                          <li key={i}>
                            [{n.scope === 'team' ? 'team' : n.category}] {n.summary}
                          </li>
                        ))}
                      </ul>
                    ) : '—',
                  },
                ]
                return (
                  <MobileStatCard
                    key={p.gsis_id}
                    onClick={() => setViewingPlayer(p.gsis_id)}
                    leading={<PlayerAvatar src={p.headshot_url} alt={p.name} size={28} />}
                    title={p.name}
                    subtitle={p.team}
                    face={
                      <div className="flex items-center gap-2">
                        <DeltaBadge delta={p.rank_delta} />
                        <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                          {p.our_rank} / {p.consensus_rank_median.toFixed(1)}
                        </span>
                      </div>
                    }
                    expanded={expanded}
                  />
                )
              })}
            </div>

            <div className="hidden md:block overflow-x-auto rounded-lg bg-card">
              <Table>
                <TableHeader style={{ top: 0 }}>
                  <HeaderRow>
                    <SortableHead
                      col="delta"
                      current={sortCol}
                      dir={sortDir}
                      onSort={handleSort}
                      className="w-20 text-right"
                    >
                      <HeaderTip description="Our rank minus the consensus median rank, within position group. Positive means we rank him lower than the market">
                        Δ
                      </HeaderTip>
                    </SortableHead>
                    <SortableHead col="name" current={sortCol} dir={sortDir} onSort={handleSort}>
                      Player
                    </SortableHead>
                    <SortableHead
                      col="pos"
                      current={sortCol}
                      dir={sortDir}
                      onSort={handleSort}
                      className="w-14 text-center"
                    >
                      <HeaderTip description="Position group">Pos</HeaderTip>
                    </SortableHead>
                    <SortableHead
                      col="our"
                      current={sortCol}
                      dir={sortDir}
                      onSort={handleSort}
                      className="w-20 text-right"
                    >
                      <HeaderTip description="Our projection rank within position group">Ours</HeaderTip>
                    </SortableHead>
                    <SortableHead
                      col="consensus"
                      current={sortCol}
                      dir={sortDir}
                      onSort={handleSort}
                      className="w-24 text-right"
                    >
                      <HeaderTip description="Median rank across external expert/ADP sources, within position group">Consensus</HeaderTip>
                    </SortableHead>
                    <SortableHead
                      col="sources"
                      current={sortCol}
                      dir={sortDir}
                      onSort={handleSort}
                      className="w-20 text-right"
                    >
                      <HeaderTip description="Number of external sources that ranked this player. Fewer than 2 is treated as uncorroborated">Sources</HeaderTip>
                    </SortableHead>
                    <TableHead>Context</TableHead>
                  </HeaderRow>
                </TableHeader>
                <TableBody>
                  {players.map((p) => (
                    <ClickableRow key={p.gsis_id}>
                      <TableCell className="text-right">
                        <DeltaBadge delta={p.rank_delta} />
                      </TableCell>
                      <PlayerCell name={p.name} imageUrl={p.headshot_url} sub={p.team} onClick={() => setViewingPlayer(p.gsis_id)} />
                      <TableCell className="text-center text-xs text-muted-foreground">
                        {p.position_group}
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        {p.our_rank}
                      </TableCell>
                      <TableCell className="text-right tabular-nums font-mono">
                        {p.consensus_rank_median.toFixed(1)}
                      </TableCell>
                      {/* A single-source divergence is noise until corroborated
                          (docs/stats/consensus-ensemble.md), so flag it rather
                          than presenting it with the same weight as a 7-source one. */}
                      <TableCell
                        className={`text-right tabular-nums font-mono ${
                          p.source_count < 2
                            ? 'text-negative-foreground'
                            : 'text-muted-foreground'
                        }`}
                        title={
                          p.source_count < 2
                            ? 'Only one source — treat this divergence as uncorroborated.'
                            : `Median across ${p.source_count} sources.`
                        }
                      >
                        {p.source_count}
                        {p.source_count < 2 ? '*' : ''}
                      </TableCell>
                      <TableCell>
                        {p.notes.length > 0 ? (
                          <span
                            className="text-xs px-1.5 py-0.5 rounded border bg-muted text-muted-foreground border-border whitespace-nowrap"
                            title={p.notes
                              .map(
                                (n) =>
                                  `[${n.scope === 'team' ? 'team' : n.category}] ${n.summary}`
                              )
                              .join('\n\n')}
                          >
                            {p.notes.length} note{p.notes.length !== 1 ? 's' : ''}
                          </span>
                        ) : (
                          <span className="text-muted-foreground/40">—</span>
                        )}
                      </TableCell>
                    </ClickableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            </>
          )}
        </>
      ) : null}

      <PlayerDetailPanel gsisId={viewingPlayer} onClose={() => setViewingPlayer(null)} />
    </div>
  )
}
